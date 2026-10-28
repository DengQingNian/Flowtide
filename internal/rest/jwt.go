package rest

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

// JWT 认证的默认参数。
const (
	defaultJWTClaimRoles   = "roles"         // 默认角色声明名
	defaultJWKSCacheTTL    = 5 * time.Minute // JWKS 公钥缓存时间
	defaultJWKSRefreshCool = time.Second     // 未知 kid 触发刷新后的冷却时间
	defaultJWKSHTTPTimeout = 5 * time.Second // JWKS 请求超时
	maxJWKSResponseBytes   = 1 << 20         // JWKS 响应体上限（1 MiB）
)

// JWT 校验内部错误。对外统一返回 401，不暴露具体原因。
var (
	errJWTMalformed      = errors.New("JWT 格式错误")   // 格式不合法
	errJWTUnsupportedAlg = errors.New("JWT 算法不支持")  // alg 不受支持
	errJWTSignature      = errors.New("JWT 签名无效")   // 签名校验失败
	errJWTExpired        = errors.New("JWT 已过期")    // exp 已过
	errJWTNotYetValid    = errors.New("JWT 尚未生效")   // nbf 未到
	errJWTIssuer         = errors.New("JWT 签发者不匹配") // iss 不匹配
	errJWTAudience       = errors.New("JWT 受众不匹配")  // aud 不匹配
	errJWTUnknownKey     = errors.New("JWT 签名密钥未知") // kid 不在 JWKS 中
	errJWKSUnavailable   = errors.New("JWKS 不可用")   // JWKS 拉取或解析失败
)

// JWTConfig 配置 JWT/JWKS Bearer 认证。
// JWKSURL 非空时启用 RS256；HS256Secret 非空时启用 HS256；两者可同时配置。
type JWTConfig struct {
	// JWKSURL 是 JWKS 端点地址，按 kid 获取 RSA 公钥。
	JWKSURL string
	// Issuer 非空时校验令牌的 iss 声明。
	Issuer string
	// Audience 非空时校验令牌的 aud 声明，支持字符串或字符串数组。
	Audience string
	// HS256Secret 是 HS256 共享密钥；为空时不接受 HS256 令牌。
	HS256Secret string
	// RolesClaim 是角色声明名，为空时默认使用 roles。
	// 声明值支持字符串数组或空格分隔字符串。
	RolesClaim string
	// TenantClaim 是租户声明名；当前仅解析，不参与授权判断。
	TenantClaim string
}

// jwtIdentity 是 JWT 校验通过后的身份信息。
type jwtIdentity struct {
	Subject string   // 令牌 sub
	Roles   []string // 令牌角色
	Tenant  string   // 令牌租户（可选）
}

// jwtHeader 是 JWT 头部的必要字段。
type jwtHeader struct {
	Alg string `json:"alg"` // 签名算法
	Kid string `json:"kid"` // 密钥标识
}

// jwksDocument 是 JWKS 文档。
type jwksDocument struct {
	Keys []jsonWebKey `json:"keys"` // 公钥集合
}

// jsonWebKey 是 JWKS 中的单个 RSA 公钥。
type jsonWebKey struct {
	KeyType  string `json:"kty"` // 密钥类型
	Kid      string `json:"kid"` // 密钥标识
	Use      string `json:"use"` // 用途
	Alg      string `json:"alg"` // 算法
	Modulus  string `json:"n"`   // RSA 模数（base64url）
	Exponent string `json:"e"`   // RSA 指数（base64url）
}

// jwtVerifier 负责解析、验签与校验 JWT，并缓存 JWKS 公钥。
type jwtVerifier struct {
	config          JWTConfig                 // 认证配置
	rolesClaim      string                    // 生效的角色声明名
	client          *http.Client              // JWKS HTTP 客户端
	now             func() time.Time          // 当前时间（测试可替换）
	mu              sync.Mutex                // 保护公钥缓存
	keys            map[string]*rsa.PublicKey // kid 到公钥
	fetchedAt       time.Time                 // 最近一次 JWKS 拉取时间
	cacheTTL        time.Duration             // 公钥缓存有效期
	refreshCooldown time.Duration             // 未知 kid 刷新冷却时间
}

// WithJWT 启用 JWT/JWKS Bearer 认证。
// @param config JWT 配置
// @return 选项
func WithJWT(config JWTConfig) Option {
	return func(server *Server) {
		server.jwt = newJWTVerifier(config)
	}
}

// newJWTVerifier 创建 JWT 校验器。
// @param config JWT 配置
// @return 校验器
func newJWTVerifier(config JWTConfig) *jwtVerifier {
	rolesClaim := strings.TrimSpace(config.RolesClaim)
	// 未显式配置角色声明时回退到默认值
	if rolesClaim == "" {
		rolesClaim = defaultJWTClaimRoles
	}
	return &jwtVerifier{
		config:          config,
		rolesClaim:      rolesClaim,
		client:          &http.Client{Timeout: defaultJWKSHTTPTimeout},
		now:             time.Now,
		keys:            make(map[string]*rsa.PublicKey),
		cacheTTL:        defaultJWKSCacheTTL,
		refreshCooldown: defaultJWKSRefreshCool,
	}
}

// bearerTokenFromHeader 从 Authorization 头提取 Bearer 令牌。
// @param header Authorization 头
// @return 令牌
// @return 是否存在
func bearerTokenFromHeader(header string) (string, bool) {
	const prefix = "Bearer "
	// 认证方案大小写不敏感，但必须带空格和令牌
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(header[len(prefix):])
	if token == "" {
		return "", false
	}
	return token, true
}

// verifyRequest 校验请求中的 JWT Bearer 令牌。
// @param r HTTP 请求
// @return 身份
// @return 是否通过
func (v *jwtVerifier) verifyRequest(r *http.Request) (jwtIdentity, bool) {
	token, ok := bearerTokenFromHeader(r.Header.Get("Authorization"))
	if !ok {
		return jwtIdentity{}, false
	}
	identity, err := v.verify(r.Context(), token)
	if err != nil {
		return jwtIdentity{}, false
	}
	return identity, true
}

// verify 校验单个 JWT 令牌。
// @param ctx 上下文
// @param raw 原始令牌
// @return 身份
// @return 错误
func (v *jwtVerifier) verify(ctx context.Context, raw string) (jwtIdentity, error) {
	parts := strings.Split(raw, ".")
	// JWT 必须是 header.payload.signature 三段
	if len(parts) != 3 {
		return jwtIdentity{}, errJWTMalformed
	}
	headerBytes, err := decodeJWTPart(parts[0])
	if err != nil {
		return jwtIdentity{}, err
	}
	var header jwtHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return jwtIdentity{}, errJWTMalformed
	}
	payload, err := decodeJWTPart(parts[1])
	if err != nil {
		return jwtIdentity{}, err
	}
	signature, err := decodeJWTPart(parts[2])
	if err != nil {
		return jwtIdentity{}, err
	}
	signingInput := []byte(parts[0] + "." + parts[1])
	// 只接受需求中约定的 RS256 与 HS256
	switch header.Alg {
	case "RS256":
		if err := v.verifyRS256(ctx, header.Kid, signingInput, signature); err != nil {
			return jwtIdentity{}, err
		}
	case "HS256":
		if err := v.verifyHS256(signingInput, signature); err != nil {
			return jwtIdentity{}, err
		}
	default:
		return jwtIdentity{}, errJWTUnsupportedAlg
	}
	return v.parseClaims(payload)
}

// decodeJWTPart 解码 base64url 片段。
// @param part 片段
// @return 解码结果
// @return 错误
func decodeJWTPart(part string) ([]byte, error) {
	if part == "" {
		return nil, errJWTMalformed
	}
	data, err := base64.RawURLEncoding.DecodeString(part)
	if err != nil {
		return nil, errJWTMalformed
	}
	return data, nil
}

// verifyRS256 使用 JWKS 中的 RSA 公钥校验 RS256 签名。
// @param ctx 上下文
// @param kid 密钥标识
// @param signingInput 待验签内容
// @param signature 签名
// @return 错误
func (v *jwtVerifier) verifyRS256(ctx context.Context, kid string, signingInput, signature []byte) error {
	// RS256 必须提供 kid 才能定位公钥
	if strings.TrimSpace(kid) == "" {
		return errJWTUnknownKey
	}
	key, err := v.publicKey(ctx, kid)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(signingInput)
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature); err != nil {
		return errJWTSignature
	}
	return nil
}

// verifyHS256 使用共享密钥校验 HS256 签名。
// @param signingInput 待验签内容
// @param signature 签名
// @return 错误
func (v *jwtVerifier) verifyHS256(signingInput, signature []byte) error {
	// 未配置共享密钥时拒绝所有 HS256 令牌
	if v.config.HS256Secret == "" {
		return errJWTUnsupportedAlg
	}
	mac := hmac.New(sha256.New, []byte(v.config.HS256Secret))
	_, _ = mac.Write(signingInput)
	if !hmac.Equal(mac.Sum(nil), signature) {
		return errJWTSignature
	}
	return nil
}

// publicKey 按 kid 获取 RSA 公钥，并维护 JWKS 缓存。
// @param ctx 上下文
// @param kid 密钥标识
// @return 公钥
// @return 错误
func (v *jwtVerifier) publicKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	if v.config.JWKSURL == "" {
		return nil, errJWKSUnavailable
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	now := v.now()
	fresh := !v.fetchedAt.IsZero() && now.Sub(v.fetchedAt) < v.cacheTTL
	// 缓存有效且命中 kid 时直接复用，避免每次请求访问 JWKS
	if fresh {
		if key, ok := v.keys[kid]; ok {
			return key, nil
		}
	}
	// 冷却窗口内不重复拉取，防止未知 kid 放大 JWKS 流量
	if !v.fetchedAt.IsZero() && now.Sub(v.fetchedAt) < v.refreshCooldown {
		return nil, errJWTUnknownKey
	}
	keys, err := v.fetchJWKS(ctx)
	if err != nil {
		// 记录失败时间以限制重试频率
		v.fetchedAt = now
		return nil, err
	}
	v.keys = keys
	v.fetchedAt = now
	// 未知 kid 最多重新拉取一次，之后仍找不到即失败
	if key, ok := keys[kid]; ok {
		return key, nil
	}
	return nil, errJWTUnknownKey
}

// fetchJWKS 拉取并解析 JWKS。
// @param ctx 上下文
// @return kid 到公钥的映射
// @return 错误
func (v *jwtVerifier) fetchJWKS(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, v.config.JWKSURL, nil)
	if err != nil {
		return nil, errJWKSUnavailable
	}
	response, err := v.client.Do(request)
	if err != nil {
		return nil, errJWKSUnavailable
	}
	defer response.Body.Close()
	// 仅接受 200，其他状态统一视为 JWKS 不可用
	if response.StatusCode != http.StatusOK {
		return nil, errJWKSUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxJWKSResponseBytes+1))
	if err != nil {
		return nil, errJWKSUnavailable
	}
	// 响应体超限时拒绝解析，避免内存放大
	if len(body) > maxJWKSResponseBytes {
		return nil, errJWKSUnavailable
	}
	keys, err := parseJWKS(body)
	if err != nil {
		return nil, err
	}
	return keys, nil
}

// parseJWKS 解析 JWKS 文档，忽略非法或不适用于 RS256 的条目。
// @param data JWKS JSON
// @return kid 到公钥的映射
// @return 错误
func parseJWKS(data []byte) (map[string]*rsa.PublicKey, error) {
	var document jwksDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, errJWKSUnavailable
	}
	keys := make(map[string]*rsa.PublicKey, len(document.Keys))
	for _, entry := range document.Keys {
		// 只接受带 kid 的 RSA 签名密钥
		if !strings.EqualFold(entry.KeyType, "RSA") || strings.TrimSpace(entry.Kid) == "" {
			continue
		}
		// use 为空表示未限制，非 sig 用途直接跳过
		if entry.Use != "" && !strings.EqualFold(entry.Use, "sig") {
			continue
		}
		// alg 为空表示未限制，非 RS256 直接跳过
		if entry.Alg != "" && entry.Alg != "RS256" {
			continue
		}
		key, err := parseRSAPublicKey(entry)
		if err != nil {
			continue
		}
		keys[entry.Kid] = key
	}
	return keys, nil
}

// parseRSAPublicKey 将 JWK 的 n/e 转换为 RSA 公钥。
// @param entry JWK 条目
// @return 公钥
// @return 错误
func parseRSAPublicKey(entry jsonWebKey) (*rsa.PublicKey, error) {
	modulusBytes, err := base64.RawURLEncoding.DecodeString(entry.Modulus)
	if err != nil || len(modulusBytes) == 0 {
		return nil, errJWKSUnavailable
	}
	exponentBytes, err := base64.RawURLEncoding.DecodeString(entry.Exponent)
	if err != nil || len(exponentBytes) == 0 || len(exponentBytes) > 4 {
		return nil, errJWKSUnavailable
	}
	exponent := new(big.Int).SetBytes(exponentBytes)
	// 指数必须在合理范围内且为奇数（长度已限制为 4 字节，可安全转 int64）
	value := exponent.Int64()
	if value < 3 || value > math.MaxInt32 || value%2 == 0 {
		return nil, errJWKSUnavailable
	}
	modulus := new(big.Int).SetBytes(modulusBytes)
	if modulus.Sign() <= 0 {
		return nil, errJWKSUnavailable
	}
	return &rsa.PublicKey{N: modulus, E: int(value)}, nil
}

// parseClaims 校验声明并构造身份。
// @param payload 载荷 JSON
// @return 身份
// @return 错误
func (v *jwtVerifier) parseClaims(payload []byte) (jwtIdentity, error) {
	var claims map[string]json.RawMessage
	// 载荷必须是 JSON 对象
	if err := json.Unmarshal(payload, &claims); err != nil || claims == nil {
		return jwtIdentity{}, errJWTMalformed
	}
	subject, err := claimString(claims, "sub")
	if err != nil || subject == "" {
		return jwtIdentity{}, errJWTMalformed
	}
	issuer, err := claimString(claims, "iss")
	if err != nil {
		return jwtIdentity{}, err
	}
	now := v.now()
	// exp 存在时必须晚于当前时间
	if expiry, present, err := claimTime(claims, "exp"); err != nil {
		return jwtIdentity{}, err
	} else if present && !now.Before(expiry) {
		return jwtIdentity{}, errJWTExpired
	}
	// nbf 存在时不得晚于当前时间
	if notBefore, present, err := claimTime(claims, "nbf"); err != nil {
		return jwtIdentity{}, err
	} else if present && now.Before(notBefore) {
		return jwtIdentity{}, errJWTNotYetValid
	}
	// 配置了签发者时执行 iss 校验
	if v.config.Issuer != "" && issuer != v.config.Issuer {
		return jwtIdentity{}, errJWTIssuer
	}
	// 配置了受众时要求 aud 包含该值
	if v.config.Audience != "" {
		audiences, err := claimAudiences(claims["aud"])
		if err != nil {
			return jwtIdentity{}, err
		}
		if !containsString(audiences, v.config.Audience) {
			return jwtIdentity{}, errJWTAudience
		}
	}
	roles, err := v.claimRoles(claims)
	if err != nil {
		return jwtIdentity{}, err
	}
	tenant := ""
	if v.config.TenantClaim != "" {
		tenant, err = claimString(claims, v.config.TenantClaim)
		if err != nil {
			return jwtIdentity{}, err
		}
	}
	return jwtIdentity{Subject: subject, Roles: roles, Tenant: tenant}, nil
}

// claimString 读取字符串声明。
// @param claims 声明集合
// @param name 声明名
// @return 值
// @return 错误
func claimString(claims map[string]json.RawMessage, name string) (string, error) {
	raw, ok := claims[name]
	// 缺失或 null 视为空值
	if !ok || len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", errJWTMalformed
	}
	return value, nil
}

// claimTime 读取 NumericDate 声明。
// @param claims 声明集合
// @param name 声明名
// @return 时间
// @return 是否存在
// @return 错误
func claimTime(claims map[string]json.RawMessage, name string) (time.Time, bool, error) {
	raw, ok := claims[name]
	// 缺失或 null 表示未设置
	if !ok || len(raw) == 0 || string(raw) == "null" {
		return time.Time{}, false, nil
	}
	var seconds float64
	if err := json.Unmarshal(raw, &seconds); err != nil {
		return time.Time{}, false, errJWTMalformed
	}
	whole, fraction := math.Modf(seconds)
	return time.Unix(int64(whole), int64(fraction*float64(time.Second))), true, nil
}

// claimAudiences 解析 aud 声明，支持字符串与字符串数组。
// @param raw 原始声明
// @return 受众列表
// @return 错误
func claimAudiences(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	// 先按单个字符串解析，失败后再按字符串数组解析
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return []string{single}, nil
	}
	var multiple []string
	if err := json.Unmarshal(raw, &multiple); err != nil {
		return nil, errJWTMalformed
	}
	return multiple, nil
}

// claimRoles 解析角色声明，支持数组或空格分隔字符串。
// @param claims 声明集合
// @return 角色列表
// @return 错误
func (v *jwtVerifier) claimRoles(claims map[string]json.RawMessage) ([]string, error) {
	raw, ok := claims[v.rolesClaim]
	// 缺失或 null 表示无角色
	if !ok || len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	// 先按字符串数组解析，失败后再按空格分隔字符串解析
	var roles []string
	if err := json.Unmarshal(raw, &roles); err == nil {
		return normalizeRoles(roles), nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return nil, errJWTMalformed
	}
	return normalizeRoles(strings.Fields(text)), nil
}

// normalizeRoles 去掉角色两侧空白并过滤空值。
// @param roles 角色列表
// @return 规整后的角色列表
func normalizeRoles(roles []string) []string {
	result := make([]string, 0, len(roles))
	for _, role := range roles {
		role = strings.TrimSpace(role)
		// 跳过空角色，避免映射到空权限
		if role != "" {
			result = append(result, role)
		}
	}
	return result
}
