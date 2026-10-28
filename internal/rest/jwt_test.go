package rest

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/flowable-go/flowable/internal/engine"
)

// 测试密钥只生成一次，避免重复生成 2048 位 RSA 密钥拖慢测试。
var (
	jwtTestKeyOnce      sync.Once // 主密钥初始化
	jwtTestKeyRSA       *rsa.PrivateKey
	jwtTestWrongKeyOnce sync.Once // 错误密钥初始化
	jwtTestWrongKeyRSA  *rsa.PrivateKey
)

// jwtJWKSFixture 是测试用 JWKS HTTP 服务。
type jwtJWKSFixture struct {
	server *httptest.Server // HTTP 服务
	hits   atomic.Int32     // 请求次数
	mu     sync.Mutex       // 保护响应配置
	keys   []map[string]any // 公钥集合
	status int              // HTTP 状态码
	raw    string           // 自定义响应体
}

// testJWTKey 返回测试用 RSA 私钥。
// @param t 测试上下文
// @return 私钥
func testJWTKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	jwtTestKeyOnce.Do(func() {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("生成 RSA 私钥失败: %v", err)
		}
		jwtTestKeyRSA = key
	})
	return jwtTestKeyRSA
}

// testJWTWrongKey 返回另一个 RSA 私钥，用于错误签名场景。
// @param t 测试上下文
// @return 私钥
func testJWTWrongKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	jwtTestWrongKeyOnce.Do(func() {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("生成错误 RSA 私钥失败: %v", err)
		}
		jwtTestWrongKeyRSA = key
	})
	return jwtTestWrongKeyRSA
}

// testJWK 将 RSA 公钥转换为 JWK。
// @param kid 密钥标识
// @param key 公钥
// @return JWK
func testJWK(kid string, key *rsa.PublicKey) map[string]any {
	return map[string]any{
		"kty": "RSA",
		"kid": kid,
		"use": "sig",
		"alg": "RS256",
		"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
	}
}

// newJWTJWKS 创建测试 JWKS 服务。
// @param t 测试上下文
// @param keys 公钥集合
// @return 测试服务
func newJWTJWKS(t *testing.T, keys ...map[string]any) *jwtJWKSFixture {
	t.Helper()
	fixture := &jwtJWKSFixture{keys: keys, status: http.StatusOK}
	fixture.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fixture.hits.Add(1)
		fixture.mu.Lock()
		status := fixture.status
		raw := fixture.raw
		current := append([]map[string]any(nil), fixture.keys...)
		fixture.mu.Unlock()
		// raw 非空时直接返回自定义响应，便于构造异常场景
		if raw != "" {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(raw))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": current})
	}))
	t.Cleanup(fixture.server.Close)
	return fixture
}

// setResponse 设置自定义响应。
// @param status HTTP 状态码
// @param raw 响应体
func (f *jwtJWKSFixture) setResponse(status int, raw string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status = status
	f.raw = raw
}

// testSignJWTWithParts 使用指定 header/payload 与签名函数构造 JWT。
// @param t 测试上下文
// @param header 头部 JSON
// @param payload 载荷 JSON
// @param sign 签名函数
// @return 令牌
func testSignJWTWithParts(t *testing.T, header, payload []byte, sign func([]byte) []byte) string {
	t.Helper()
	headerPart := base64.RawURLEncoding.EncodeToString(header)
	payloadPart := base64.RawURLEncoding.EncodeToString(payload)
	signingInput := headerPart + "." + payloadPart
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sign([]byte(signingInput)))
}

// testSignJWT 使用 RSA 私钥签发 RS256 JWT。
// @param t 测试上下文
// @param key 私钥
// @param kid 密钥标识
// @param claims 声明
// @return 令牌
func testSignJWT(t *testing.T, key *rsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()
	header := map[string]any{"alg": "RS256", "typ": "JWT"}
	if kid != "" {
		header["kid"] = kid
	}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		t.Fatalf("序列化 JWT 头部失败: %v", err)
	}
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("序列化 JWT 载荷失败: %v", err)
	}
	return testSignJWTWithParts(t, headerJSON, payloadJSON, func(input []byte) []byte {
		digest := sha256.Sum256(input)
		signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
		if err != nil {
			t.Fatalf("RSA 签名失败: %v", err)
		}
		return signature
	})
}

// testSignHS256 使用共享密钥签发 HS256 JWT。
// @param t 测试上下文
// @param secret 共享密钥
// @param claims 声明
// @return 令牌
func testSignHS256(t *testing.T, secret string, claims map[string]any) string {
	t.Helper()
	headerJSON, err := json.Marshal(map[string]any{"alg": "HS256", "typ": "JWT"})
	if err != nil {
		t.Fatalf("序列化 JWT 头部失败: %v", err)
	}
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("序列化 JWT 载荷失败: %v", err)
	}
	return testSignJWTWithParts(t, headerJSON, payloadJSON, func(input []byte) []byte {
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write(input)
		return mac.Sum(nil)
	})
}

// newJWTRestServer 创建启用后端的测试 REST 服务。
// @param t 测试上下文
// @param options 选项
// @return 测试服务器
func newJWTRestServer(t *testing.T, options ...Option) *httptest.Server {
	t.Helper()
	app, err := engine.New(engine.Config{HistoryLevel: engine.HistoryAudit})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	server := httptest.NewServer(New(app, options...).Handler())
	t.Cleanup(func() {
		server.Close()
		if err := app.Close(context.Background()); err != nil {
			t.Errorf("关闭引擎失败: %v", err)
		}
	})
	return server
}

// jwtHTTP 发起带 Authorization 头的请求并返回状态码。
// @param t 测试上下文
// @param server 测试服务器
// @param method HTTP 方法
// @param path 路径
// @param authorization Authorization 头
// @return 状态码
func jwtHTTP(t *testing.T, server *httptest.Server, method, path, authorization string) int {
	t.Helper()
	request, err := http.NewRequest(method, server.URL+path, nil)
	if err != nil {
		t.Fatalf("创建请求失败: %v", err)
	}
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer response.Body.Close()
	return response.StatusCode
}

// assertJWTStatus 断言 JWT 请求状态码。
// @param t 测试上下文
// @param server 测试服务器
// @param method HTTP 方法
// @param path 路径
// @param authorization Authorization 头
// @param want 期望状态码
func assertJWTStatus(t *testing.T, server *httptest.Server, method, path, authorization string, want int) {
	t.Helper()
	if got := jwtHTTP(t, server, method, path, authorization); got != want {
		t.Fatalf("%s %s 期望 %d，实际 %d", method, path, want, got)
	}
}

// TestJWTValidRS256AndRBAC 验证 RS256 有效令牌与 RBAC。
// @param t 测试上下文
func TestJWTValidRS256AndRBAC(t *testing.T) {
	key := testJWTKey(t)
	fixture := newJWTJWKS(t, testJWK("kid-1", &key.PublicKey))
	config := JWTConfig{
		JWKSURL:     fixture.server.URL,
		Issuer:      "issuer-1",
		Audience:    "flowable",
		TenantClaim: "tenant",
	}
	server := newJWTRestServer(t, WithJWT(config), WithRBAC(nil, map[string][]string{
		"reader": {"read"},
		"writer": {"write"},
		"admin":  {"admin"},
	}))
	now := time.Now()
	claims := map[string]any{
		"sub":    "alice",
		"iss":    "issuer-1",
		"aud":    []string{"other", "flowable"},
		"exp":    now.Add(time.Hour).Unix(),
		"nbf":    now.Add(-time.Minute).Unix(),
		"roles":  []string{"reader"},
		"tenant": "tenant-1",
	}
	token := testSignJWT(t, key, "kid-1", claims)
	assertJWTStatus(t, server, http.MethodGet, "/service/repository/deployments", "Bearer "+token, http.StatusOK)
	if got := fixture.hits.Load(); got != 1 {
		t.Fatalf("首次校验应只拉取一次 JWKS，实际 %d", got)
	}
	assertJWTStatus(t, server, http.MethodGet, "/service/management/engine", "Bearer "+token, http.StatusForbidden)
	assertJWTStatus(t, server, http.MethodPost, "/service/repository/deployments", "Bearer "+token, http.StatusForbidden)

	adminToken := testSignJWT(t, key, "kid-1", map[string]any{
		"sub":   "root",
		"iss":   "issuer-1",
		"aud":   "flowable",
		"exp":   now.Add(time.Hour).Unix(),
		"roles": []string{"admin"},
	})
	assertJWTStatus(t, server, http.MethodGet, "/service/management/engine", "Bearer "+adminToken, http.StatusOK)

	// 空格分隔的角色字符串也应被识别
	spaceToken := testSignJWT(t, key, "kid-1", map[string]any{
		"sub":   "root",
		"iss":   "issuer-1",
		"aud":   "flowable",
		"exp":   now.Add(time.Hour).Unix(),
		"roles": "reader admin",
	})
	assertJWTStatus(t, server, http.MethodGet, "/service/management/engine", "Bearer "+spaceToken, http.StatusOK)

	verifier := newJWTVerifier(config)
	identity, err := verifier.verify(context.Background(), token)
	if err != nil {
		t.Fatalf("直接校验失败: %v", err)
	}
	if identity.Subject != "alice" || identity.Tenant != "tenant-1" || len(identity.Roles) != 1 || identity.Roles[0] != "reader" {
		t.Fatalf("身份解析错误: %+v", identity)
	}
	if got := fixture.hits.Load(); got != 2 {
		t.Fatalf("独立校验器应再拉取一次 JWKS，实际 %d", got)
	}
}

// TestJWTExpiryIssuerAudience 验证 exp/nbf/iss/aud 等声明校验。
// @param t 测试上下文
func TestJWTExpiryIssuerAudience(t *testing.T) {
	const secret = "hs256-secret"
	server := newJWTRestServer(t, WithJWT(JWTConfig{
		HS256Secret: secret,
		Issuer:      "issuer",
		Audience:    "flowable",
	}))
	now := time.Now()
	base := func() map[string]any {
		return map[string]any{
			"sub": "alice",
			"iss": "issuer",
			"aud": "flowable",
			"exp": now.Add(time.Hour).Unix(),
		}
	}
	cases := []struct {
		name   string
		mutate func(map[string]any)
		want   int
	}{
		{"Valid", func(map[string]any) {}, http.StatusOK},
		{"Expired", func(claims map[string]any) {
			claims["exp"] = now.Add(-time.Second).Unix()
		}, http.StatusUnauthorized},
		{"NotYetValid", func(claims map[string]any) {
			claims["nbf"] = now.Add(time.Hour).Unix()
		}, http.StatusUnauthorized},
		{"ValidNbf", func(claims map[string]any) {
			claims["nbf"] = now.Add(-time.Minute).Unix()
		}, http.StatusOK},
		{"MissingExp", func(claims map[string]any) {
			delete(claims, "exp")
		}, http.StatusOK},
		{"NullExp", func(claims map[string]any) {
			claims["exp"] = nil
		}, http.StatusOK},
		{"FractionalExp", func(claims map[string]any) {
			claims["exp"] = float64(now.Add(time.Hour).Unix()) + 0.5
		}, http.StatusOK},
		{"BadExp", func(claims map[string]any) {
			claims["exp"] = "soon"
		}, http.StatusUnauthorized},
		{"BadNbf", func(claims map[string]any) {
			claims["nbf"] = []int{1}
		}, http.StatusUnauthorized},
		{"MissingSub", func(claims map[string]any) {
			delete(claims, "sub")
		}, http.StatusUnauthorized},
		{"BadSub", func(claims map[string]any) {
			claims["sub"] = 7
		}, http.StatusUnauthorized},
		{"BadIssuer", func(claims map[string]any) {
			claims["iss"] = "other"
		}, http.StatusUnauthorized},
		{"BadIssuerType", func(claims map[string]any) {
			claims["iss"] = 7
		}, http.StatusUnauthorized},
		{"BadAudience", func(claims map[string]any) {
			claims["aud"] = "other"
		}, http.StatusUnauthorized},
		{"MissingAudience", func(claims map[string]any) {
			delete(claims, "aud")
		}, http.StatusUnauthorized},
		{"NullAudience", func(claims map[string]any) {
			claims["aud"] = nil
		}, http.StatusUnauthorized},
		{"AudienceArray", func(claims map[string]any) {
			claims["aud"] = []string{"other", "flowable"}
		}, http.StatusOK},
		{"BadAudienceType", func(claims map[string]any) {
			claims["aud"] = 7
		}, http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			claims := base()
			tc.mutate(claims)
			token := testSignHS256(t, secret, claims)
			assertJWTStatus(t, server, http.MethodGet, "/service/repository/deployments", "Bearer "+token, tc.want)
		})
	}
	assertJWTStatus(t, server, http.MethodGet, "/service/repository/deployments", "", http.StatusUnauthorized)
}

// TestJWTHS256RolesAndTenant 验证 HS256、自定义角色声明与租户声明。
// @param t 测试上下文
func TestJWTHS256RolesAndTenant(t *testing.T) {
	config := JWTConfig{
		HS256Secret: "secret",
		RolesClaim:  " authorities ",
		TenantClaim: "tenant_id",
	}
	server := newJWTRestServer(t, WithJWT(config), WithRBAC(nil, map[string][]string{
		"reader": {"read"},
		"writer": {"write"},
		"admin":  {"admin"},
	}))
	now := time.Now()
	claims := map[string]any{
		"sub":         "alice",
		"exp":         now.Add(time.Hour).Unix(),
		"authorities": "reader writer",
		"tenant_id":   "tenant-1",
	}
	token := testSignHS256(t, "secret", claims)
	identity, err := newJWTVerifier(config).verify(context.Background(), token)
	if err != nil {
		t.Fatalf("HS256 校验失败: %v", err)
	}
	if identity.Subject != "alice" || identity.Tenant != "tenant-1" {
		t.Fatalf("身份解析错误: %+v", identity)
	}
	if len(identity.Roles) != 2 || identity.Roles[0] != "reader" || identity.Roles[1] != "writer" {
		t.Fatalf("空格分隔角色解析错误: %+v", identity.Roles)
	}

	assertJWTStatus(t, server, http.MethodGet, "/service/repository/deployments", "Bearer "+token, http.StatusOK)
	readerToken := testSignHS256(t, "secret", map[string]any{
		"sub":         "alice",
		"exp":         now.Add(time.Hour).Unix(),
		"authorities": []string{"reader"},
	})
	assertJWTStatus(t, server, http.MethodPost, "/service/repository/deployments", "Bearer "+readerToken, http.StatusForbidden)
	// writer 角色允许写权限，进入处理器后因缺少请求体返回 400 而非 403
	writerToken := testSignHS256(t, "secret", map[string]any{
		"sub":         "alice",
		"exp":         now.Add(time.Hour).Unix(),
		"authorities": []string{"writer"},
	})
	assertJWTStatus(t, server, http.MethodPost, "/service/repository/deployments", "Bearer "+writerToken, http.StatusBadRequest)

	adminToken := testSignHS256(t, "secret", map[string]any{
		"sub":         "root",
		"exp":         now.Add(time.Hour).Unix(),
		"authorities": "reader admin",
	})
	assertJWTStatus(t, server, http.MethodGet, "/service/management/engine", "Bearer "+adminToken, http.StatusOK)

	trimmedToken := testSignHS256(t, "secret", map[string]any{
		"sub":         "alice",
		"exp":         now.Add(time.Hour).Unix(),
		"authorities": []string{"", " reader "},
	})
	assertJWTStatus(t, server, http.MethodGet, "/service/repository/deployments", "Bearer "+trimmedToken, http.StatusOK)

	badRoles := testSignHS256(t, "secret", map[string]any{
		"sub":         "alice",
		"exp":         now.Add(time.Hour).Unix(),
		"authorities": 7,
	})
	assertJWTStatus(t, server, http.MethodGet, "/service/repository/deployments", "Bearer "+badRoles, http.StatusUnauthorized)

	badRoleArray := testSignHS256(t, "secret", map[string]any{
		"sub":         "alice",
		"exp":         now.Add(time.Hour).Unix(),
		"authorities": []int{1, 2},
	})
	assertJWTStatus(t, server, http.MethodGet, "/service/repository/deployments", "Bearer "+badRoleArray, http.StatusUnauthorized)

	badTenant := testSignHS256(t, "secret", map[string]any{
		"sub":         "alice",
		"exp":         now.Add(time.Hour).Unix(),
		"authorities": []string{"reader"},
		"tenant_id":   7,
	})
	assertJWTStatus(t, server, http.MethodGet, "/service/repository/deployments", "Bearer "+badTenant, http.StatusUnauthorized)
}

// TestJWTSignatureAndAlgorithm 验证签名、算法与令牌格式错误。
// @param t 测试上下文
func TestJWTSignatureAndAlgorithm(t *testing.T) {
	key := testJWTKey(t)
	wrongKey := testJWTWrongKey(t)
	fixture := newJWTJWKS(t, testJWK("kid-1", &key.PublicKey))
	server := newJWTRestServer(t, WithJWT(JWTConfig{
		JWKSURL:     fixture.server.URL,
		HS256Secret: "secret",
	}))
	now := time.Now()
	claims := map[string]any{"sub": "alice", "exp": now.Add(time.Hour).Unix()}
	validRS := testSignJWT(t, key, "kid-1", claims)
	assertJWTStatus(t, server, http.MethodGet, "/service/repository/deployments", "Bearer "+validRS, http.StatusOK)
	assertJWTStatus(t, server, http.MethodGet, "/service/repository/deployments", "Bearer "+testSignJWT(t, wrongKey, "kid-1", claims), http.StatusUnauthorized)
	assertJWTStatus(t, server, http.MethodGet, "/service/repository/deployments", "Bearer "+testSignJWT(t, key, "", claims), http.StatusUnauthorized)

	validHS := testSignHS256(t, "secret", claims)
	assertJWTStatus(t, server, http.MethodGet, "/service/repository/deployments", "Bearer "+validHS, http.StatusOK)
	assertJWTStatus(t, server, http.MethodGet, "/service/repository/deployments", "Bearer "+testSignHS256(t, "wrong", claims), http.StatusUnauthorized)

	// 篡改载荷但保留原签名
	parts := strings.Split(validRS, ".")
	tamperedPayload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"mallory","exp":9999999999}`))
	tampered := parts[0] + "." + tamperedPayload + "." + parts[2]
	assertJWTStatus(t, server, http.MethodGet, "/service/repository/deployments", "Bearer "+tampered, http.StatusUnauthorized)

	// 不支持的算法
	unsupportedHeader := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS512","kid":"kid-1"}`))
	unsupportedPayload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"alice"}`))
	assertJWTStatus(t, server, http.MethodGet, "/service/repository/deployments",
		"Bearer "+unsupportedHeader+"."+unsupportedPayload+".eA", http.StatusUnauthorized)
	noneHeader := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	assertJWTStatus(t, server, http.MethodGet, "/service/repository/deployments",
		"Bearer "+noneHeader+"."+unsupportedPayload+".eA", http.StatusUnauthorized)

	// 各种格式错误
	malformed := []string{
		"abc",
		"a.b",
		"!!!.e30.eA",
		base64.RawURLEncoding.EncodeToString([]byte("{")) + ".e30.eA",
		"e30.!!!.eA",
		"e30.e30.!!!",
		"e30.e30.",
	}
	for _, token := range malformed {
		assertJWTStatus(t, server, http.MethodGet, "/service/repository/deployments", "Bearer "+token, http.StatusUnauthorized)
	}

	// 签名正确但载荷不是 JSON
	badPayload := testSignJWTWithParts(t,
		[]byte(`{"alg":"RS256","kid":"kid-1"}`),
		[]byte("not-json"),
		func(input []byte) []byte {
			digest := sha256.Sum256(input)
			signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
			if err != nil {
				t.Fatalf("签名失败: %v", err)
			}
			return signature
		})
	assertJWTStatus(t, server, http.MethodGet, "/service/repository/deployments", "Bearer "+badPayload, http.StatusUnauthorized)

	// 仅启用 HS256 时拒绝 RS256
	hsOnly := newJWTRestServer(t, WithJWT(JWTConfig{HS256Secret: "secret"}))
	assertJWTStatus(t, hsOnly, http.MethodGet, "/service/repository/deployments", "Bearer "+validRS, http.StatusUnauthorized)
	// 仅启用 RS256 时拒绝 HS256
	rsOnly := newJWTRestServer(t, WithJWT(JWTConfig{JWKSURL: fixture.server.URL}))
	assertJWTStatus(t, rsOnly, http.MethodGet, "/service/repository/deployments", "Bearer "+validHS, http.StatusUnauthorized)
}

// TestJWKSCacheAndUnknownKid 验证 JWKS 缓存、未知 kid 与冷却窗口。
// @param t 测试上下文
func TestJWKSCacheAndUnknownKid(t *testing.T) {
	key := testJWTKey(t)
	fixture := newJWTJWKS(t, testJWK("kid-1", &key.PublicKey))
	verifier := newJWTVerifier(JWTConfig{JWKSURL: fixture.server.URL})
	fixed := time.Now()
	verifier.now = func() time.Time { return fixed }
	claims := map[string]any{"sub": "alice", "exp": fixed.Add(time.Hour).Unix()}
	unknown := testSignJWT(t, key, "missing", claims)
	if _, err := verifier.verify(context.Background(), unknown); !errors.Is(err, errJWTUnknownKey) {
		t.Fatalf("未知 kid 应返回未知密钥错误: %v", err)
	}
	if got := fixture.hits.Load(); got != 1 {
		t.Fatalf("未知 kid 应拉取一次 JWKS，实际 %d", got)
	}
	// 冷却窗口内重复请求不应再次拉取
	if _, err := verifier.verify(context.Background(), unknown); !errors.Is(err, errJWTUnknownKey) {
		t.Fatalf("未知 kid 应返回未知密钥错误: %v", err)
	}
	if got := fixture.hits.Load(); got != 1 {
		t.Fatalf("冷却窗口内不应重复拉取 JWKS，实际 %d", got)
	}
	valid := testSignJWT(t, key, "kid-1", claims)
	if _, err := verifier.verify(context.Background(), valid); err != nil {
		t.Fatalf("有效令牌校验失败: %v", err)
	}
	if got := fixture.hits.Load(); got != 1 {
		t.Fatalf("有效 kid 应命中缓存，实际拉取 %d 次", got)
	}
	// 超过冷却时间后未知 kid 允许再拉取一次
	fixed = fixed.Add(2 * time.Second)
	verifier.now = func() time.Time { return fixed }
	if _, err := verifier.verify(context.Background(), unknown); !errors.Is(err, errJWTUnknownKey) {
		t.Fatalf("未知 kid 应返回未知密钥错误: %v", err)
	}
	if got := fixture.hits.Load(); got != 2 {
		t.Fatalf("冷却后应再拉取一次 JWKS，实际 %d", got)
	}
	// 缓存过期后有效 kid 也应重新拉取
	fixed = fixed.Add(6 * time.Minute)
	verifier.now = func() time.Time { return fixed }
	if _, err := verifier.verify(context.Background(), valid); err != nil {
		t.Fatalf("缓存过期后有效令牌校验失败: %v", err)
	}
	if got := fixture.hits.Load(); got != 3 {
		t.Fatalf("缓存过期后应重新拉取 JWKS，实际 %d", got)
	}
}

// jwtRoundTripFunc 是测试用 RoundTripper。
type jwtRoundTripFunc func(*http.Request) (*http.Response, error)

// RoundTrip 实现 http.RoundTripper。
// @param request 请求
// @return 响应
// @return 错误
func (f jwtRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

// jwtReadError 是始终返回错误的响应体。
type jwtReadError struct{}

// Read 始终返回读取错误。
// @param p 缓冲区
// @return 读取字节数
// @return 错误
func (jwtReadError) Read([]byte) (int, error) {
	return 0, errors.New("读取失败")
}

// TestJWKSFailures 验证 JWKS 拉取与解析失败场景。
// @param t 测试上下文
func TestJWKSFailures(t *testing.T) {
	key := testJWTKey(t)
	token := testSignJWT(t, key, "kid-1", map[string]any{"sub": "alice", "exp": time.Now().Add(time.Hour).Unix()})

	t.Run("NetworkError", func(t *testing.T) {
		fixture := newJWTJWKS(t, testJWK("kid-1", &key.PublicKey))
		jwksURL := fixture.server.URL
		fixture.server.Close()
		verifier := newJWTVerifier(JWTConfig{JWKSURL: jwksURL})
		if _, err := verifier.verify(context.Background(), token); !errors.Is(err, errJWKSUnavailable) {
			t.Fatalf("网络错误应返回 JWKS 不可用: %v", err)
		}
	})

	t.Run("NonOKStatus", func(t *testing.T) {
		fixture := newJWTJWKS(t)
		fixture.setResponse(http.StatusInternalServerError, "")
		verifier := newJWTVerifier(JWTConfig{JWKSURL: fixture.server.URL})
		if _, err := verifier.verify(context.Background(), token); !errors.Is(err, errJWKSUnavailable) {
			t.Fatalf("非 200 应返回 JWKS 不可用: %v", err)
		}
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		fixture := newJWTJWKS(t)
		fixture.setResponse(http.StatusOK, "{bad")
		verifier := newJWTVerifier(JWTConfig{JWKSURL: fixture.server.URL})
		if _, err := verifier.verify(context.Background(), token); !errors.Is(err, errJWKSUnavailable) {
			t.Fatalf("非法 JSON 应返回 JWKS 不可用: %v", err)
		}
	})

	t.Run("OversizedBody", func(t *testing.T) {
		fixture := newJWTJWKS(t)
		fixture.setResponse(http.StatusOK, strings.Repeat("x", maxJWKSResponseBytes+1))
		verifier := newJWTVerifier(JWTConfig{JWKSURL: fixture.server.URL})
		if _, err := verifier.verify(context.Background(), token); !errors.Is(err, errJWKSUnavailable) {
			t.Fatalf("超大响应应返回 JWKS 不可用: %v", err)
		}
	})

	t.Run("EmptyKeys", func(t *testing.T) {
		fixture := newJWTJWKS(t)
		verifier := newJWTVerifier(JWTConfig{JWKSURL: fixture.server.URL})
		if _, err := verifier.verify(context.Background(), token); !errors.Is(err, errJWTUnknownKey) {
			t.Fatalf("空 JWKS 应返回未知密钥: %v", err)
		}
	})

	t.Run("InvalidURL", func(t *testing.T) {
		verifier := newJWTVerifier(JWTConfig{JWKSURL: "://bad"})
		if _, err := verifier.verify(context.Background(), token); !errors.Is(err, errJWKSUnavailable) {
			t.Fatalf("非法 URL 应返回 JWKS 不可用: %v", err)
		}
	})

	t.Run("ReadError", func(t *testing.T) {
		verifier := newJWTVerifier(JWTConfig{JWKSURL: "http://example.test/jwks"})
		verifier.client = &http.Client{Transport: jwtRoundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(jwtReadError{}),
			}, nil
		})}
		if _, err := verifier.verify(context.Background(), token); !errors.Is(err, errJWKSUnavailable) {
			t.Fatalf("读取失败应返回 JWKS 不可用: %v", err)
		}
	})

	t.Run("CanceledContext", func(t *testing.T) {
		fixture := newJWTJWKS(t, testJWK("kid-1", &key.PublicKey))
		verifier := newJWTVerifier(JWTConfig{JWKSURL: fixture.server.URL})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := verifier.verify(ctx, token); !errors.Is(err, errJWKSUnavailable) {
			t.Fatalf("取消上下文应返回 JWKS 不可用: %v", err)
		}
	})

	t.Run("NoJWKSURL", func(t *testing.T) {
		verifier := newJWTVerifier(JWTConfig{})
		if _, err := verifier.verify(context.Background(), token); !errors.Is(err, errJWKSUnavailable) {
			t.Fatalf("未配置 JWKSURL 应返回 JWKS 不可用: %v", err)
		}
	})
}

// TestJWTJWKSNetworkErrorIs401 验证 JWKS 网络错误对外统一 401。
// @param t 测试上下文
func TestJWTJWKSNetworkErrorIs401(t *testing.T) {
	key := testJWTKey(t)
	fixture := newJWTJWKS(t, testJWK("kid-1", &key.PublicKey))
	jwksURL := fixture.server.URL
	fixture.server.Close()
	server := newJWTRestServer(t, WithJWT(JWTConfig{JWKSURL: jwksURL}))
	token := testSignJWT(t, key, "kid-1", map[string]any{"sub": "alice", "exp": time.Now().Add(time.Hour).Unix()})
	assertJWTStatus(t, server, http.MethodGet, "/service/repository/deployments", "Bearer "+token, http.StatusUnauthorized)
}

// TestJWKSParse 验证 JWKS 解析与非法条目过滤。
// @param t 测试上下文
func TestJWKSParse(t *testing.T) {
	key := testJWTKey(t)
	valid := testJWK("k1", &key.PublicKey)
	valid2 := testJWK("k2", &key.PublicKey)
	valid2["kty"] = "rsa"
	valid2["use"] = "SIG"
	delete(valid2, "alg")
	entry := func(mutate func(map[string]any)) map[string]any {
		item := map[string]any{
			"kty": "RSA",
			"kid": "bad",
			"n":   valid["n"],
			"e":   valid["e"],
		}
		mutate(item)
		return item
	}
	entries := []map[string]any{
		valid,
		valid2,
		{"kty": "EC", "kid": "ec"},
		{"kty": "RSA"},
		entry(func(item map[string]any) { item["use"] = "enc" }),
		entry(func(item map[string]any) { item["alg"] = "HS256" }),
		entry(func(item map[string]any) { item["n"] = "!!!" }),
		entry(func(item map[string]any) { item["e"] = "!!!" }),
		entry(func(item map[string]any) { item["e"] = "" }),
		entry(func(item map[string]any) {
			item["e"] = base64.RawURLEncoding.EncodeToString([]byte{1, 0, 1, 0, 1})
		}),
		entry(func(item map[string]any) {
			item["e"] = base64.RawURLEncoding.EncodeToString([]byte{1})
		}),
		entry(func(item map[string]any) {
			item["e"] = base64.RawURLEncoding.EncodeToString([]byte{0, 0, 0, 4})
		}),
		entry(func(item map[string]any) {
			item["e"] = base64.RawURLEncoding.EncodeToString([]byte{0x80, 0, 0, 1})
		}),
		entry(func(item map[string]any) {
			item["n"] = base64.RawURLEncoding.EncodeToString([]byte{0})
		}),
		entry(func(item map[string]any) { item["n"] = "" }),
	}
	document, err := json.Marshal(map[string]any{"keys": entries})
	if err != nil {
		t.Fatalf("序列化 JWKS 失败: %v", err)
	}
	keys, err := parseJWKS(document)
	if err != nil {
		t.Fatalf("解析 JWKS 失败: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("应保留 2 个有效公钥，实际 %d", len(keys))
	}
	for _, kid := range []string{"k1", "k2"} {
		publicKey := keys[kid]
		if publicKey == nil || publicKey.N.Cmp(key.PublicKey.N) != 0 || publicKey.E != key.PublicKey.E {
			t.Fatalf("%s 公钥解析错误", kid)
		}
	}
	if _, err := parseJWKS([]byte("{bad")); !errors.Is(err, errJWKSUnavailable) {
		t.Fatalf("非法 JSON 应返回 JWKS 不可用: %v", err)
	}
	empty, err := parseJWKS([]byte(`{"keys":null}`))
	if err != nil || len(empty) != 0 {
		t.Fatalf("空 JWKS 应解析为空映射: %v %d", err, len(empty))
	}
}

// TestBearerTokenFromHeader 验证 Authorization 头解析。
// @param t 测试上下文
func TestBearerTokenFromHeader(t *testing.T) {
	cases := []struct {
		header string
		token  string
		ok     bool
	}{
		{"Bearer abc", "abc", true},
		{"bearer abc", "abc", true},
		{"BEARER abc", "abc", true},
		{"Bearer   abc  ", "abc", true},
		{"Bearer ", "", false},
		{"Bearer     ", "", false},
		{"Bearer", "", false},
		{"Basic abc", "", false},
		{"Token abc", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		token, ok := bearerTokenFromHeader(tc.header)
		if token != tc.token || ok != tc.ok {
			t.Fatalf("解析 %q 期望 (%q,%v)，实际 (%q,%v)", tc.header, tc.token, tc.ok, token, ok)
		}
	}

	server := newJWTRestServer(t, WithJWT(JWTConfig{HS256Secret: "secret"}))
	token := testSignHS256(t, "secret", map[string]any{"sub": "alice", "exp": time.Now().Add(time.Hour).Unix()})
	assertJWTStatus(t, server, http.MethodGet, "/service/repository/deployments", "bearer "+token, http.StatusOK)
	assertJWTStatus(t, server, http.MethodGet, "/service/repository/deployments", "Bearer", http.StatusUnauthorized)
	assertJWTStatus(t, server, http.MethodGet, "/service/repository/deployments", "Basic YWxpY2U6c2VjcmV0", http.StatusUnauthorized)
}

// TestWithJWTOption 验证 JWT 选项与默认值。
// @param t 测试上下文
func TestWithJWTOption(t *testing.T) {
	server := &Server{}
	WithJWT(JWTConfig{JWKSURL: "https://example.test/jwks"})(server)
	if server.jwt == nil {
		t.Fatal("WithJWT 应配置校验器")
	}
	if server.jwt.rolesClaim != "roles" {
		t.Fatalf("默认角色声明错误: %s", server.jwt.rolesClaim)
	}
	if server.jwt.client == nil || server.jwt.cacheTTL != defaultJWKSCacheTTL {
		t.Fatalf("默认校验器参数错误: %+v", server.jwt)
	}

	custom := &Server{}
	WithJWT(JWTConfig{RolesClaim: "  authorities  "})(custom)
	if custom.jwt.rolesClaim != "authorities" {
		t.Fatalf("自定义角色声明未去除空白: %s", custom.jwt.rolesClaim)
	}
}

// TestJWTWithoutRolePermissions 验证未配置 RBAC 时的放行语义。
// @param t 测试上下文
func TestJWTWithoutRolePermissions(t *testing.T) {
	token := testSignHS256(t, "secret", map[string]any{"sub": "alice", "exp": time.Now().Add(time.Hour).Unix()})
	server := newJWTRestServer(t, WithJWT(JWTConfig{HS256Secret: "secret"}))
	assertJWTStatus(t, server, http.MethodGet, "/service/repository/deployments", "Bearer "+token, http.StatusOK)
	assertJWTStatus(t, server, http.MethodGet, "/service/repository/deployments", "", http.StatusUnauthorized)

	// 仅有身份库认证但未配置 rolePermissions 时，JWT 仍按放行语义处理
	identityServer := newJWTRestServer(t, WithJWT(JWTConfig{HS256Secret: "secret"}), WithIdentityAuth())
	assertJWTStatus(t, identityServer, http.MethodGet, "/service/repository/deployments", "Bearer "+token, http.StatusOK)
}

// TestJWTWithStaticBearerAndBasic 验证静态 Bearer 与 Basic Auth 兼容性。
// @param t 测试上下文
func TestJWTWithStaticBearerAndBasic(t *testing.T) {
	now := time.Now()
	jwtToken := testSignHS256(t, "secret", map[string]any{
		"sub":   "alice",
		"exp":   now.Add(time.Hour).Unix(),
		"roles": []string{"reader"},
	})
	server := newJWTRestServer(t,
		WithBasicAuth(map[string]string{"root": "root"}),
		WithBearerToken("static-token"),
		WithJWT(JWTConfig{HS256Secret: "secret"}),
		WithRBAC(map[string][]string{"root": {"admin"}}, map[string][]string{
			"reader": {"read"},
			"admin":  {"admin"},
		}),
	)
	// 静态 Bearer 仍视为管理员
	assertJWTStatus(t, server, http.MethodGet, "/service/management/engine", "Bearer static-token", http.StatusOK)
	assertJWTStatus(t, server, http.MethodGet, "/service/management/engine", "bearer static-token", http.StatusOK)
	assertJWTStatus(t, server, http.MethodGet, "/service/management/engine", "Bearer bad-token", http.StatusUnauthorized)
	// JWT 角色仍受 RBAC 约束
	assertJWTStatus(t, server, http.MethodGet, "/service/repository/deployments", "Bearer "+jwtToken, http.StatusOK)
	assertJWTStatus(t, server, http.MethodGet, "/service/management/engine", "Bearer "+jwtToken, http.StatusForbidden)
	// Basic Auth 行为保持不变
	request, err := http.NewRequest(http.MethodGet, server.URL+"/service/management/engine", nil)
	if err != nil {
		t.Fatalf("创建请求失败: %v", err)
	}
	request.SetBasicAuth("root", "root")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("Basic 请求失败: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("Basic Auth 应保持管理员访问，实际 %d", response.StatusCode)
	}
	assertJWTStatus(t, server, http.MethodGet, "/service/repository/deployments", "", http.StatusUnauthorized)
}

// TestJWTClaimHelpers 验证声明辅助函数边界。
// @param t 测试上下文
func TestJWTClaimHelpers(t *testing.T) {
	claims := map[string]json.RawMessage{
		"text":    json.RawMessage(`"value"`),
		"null":    json.RawMessage(`null`),
		"bad":     json.RawMessage(`7`),
		"badTime": json.RawMessage(`"soon"`),
	}
	if value, err := claimString(claims, "text"); err != nil || value != "value" {
		t.Fatalf("字符串声明解析错误: %q %v", value, err)
	}
	if value, err := claimString(claims, "missing"); err != nil || value != "" {
		t.Fatalf("缺失声明应为空: %q %v", value, err)
	}
	if _, err := claimString(claims, "bad"); !errors.Is(err, errJWTMalformed) {
		t.Fatalf("类型错误应返回格式错误: %v", err)
	}
	if _, present, err := claimTime(claims, "null"); err != nil || present {
		t.Fatalf("null 时间不应存在: %v %v", present, err)
	}
	if _, _, err := claimTime(claims, "badTime"); !errors.Is(err, errJWTMalformed) {
		t.Fatalf("类型错误时间应返回格式错误: %v", err)
	}
	if audiences, err := claimAudiences(claims["null"]); err != nil || len(audiences) != 0 {
		t.Fatalf("null 受众应为空: %v %v", audiences, err)
	}
	if _, err := claimAudiences(claims["bad"]); !errors.Is(err, errJWTMalformed) {
		t.Fatalf("类型错误受众应返回格式错误: %v", err)
	}
	roles := normalizeRoles([]string{" a ", "", "b"})
	if len(roles) != 2 || roles[0] != "a" || roles[1] != "b" {
		t.Fatalf("角色规整错误: %+v", roles)
	}
}

// TestJWTPermittedWithoutCredentials 验证配置 JWT 后缺少凭证的权限判断。
// @param t 测试上下文
func TestJWTPermittedWithoutCredentials(t *testing.T) {
	server := &Server{
		rolePermissions: map[string][]string{"reader": {"read"}},
		jwt:             newJWTVerifier(JWTConfig{HS256Secret: "secret"}),
	}
	request := httptest.NewRequest(http.MethodGet, "/service/repository/deployments", nil)
	if server.permitted(request) {
		t.Fatal("缺少凭证应拒绝访问")
	}
	request.Header.Set("Authorization", "Bearer invalid")
	if server.permitted(request) {
		t.Fatal("无效 JWT 应拒绝访问")
	}
}
