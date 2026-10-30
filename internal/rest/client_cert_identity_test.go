package rest

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/internal/engine"
)

// testPKI 保存测试用 CA、服务端证书与信任池。
type testPKI struct {
	caCert     *x509.Certificate // CA 证书
	caKey      *ecdsa.PrivateKey // CA 私钥
	roots      *x509.CertPool    // 客户端与服务端共用的信任池
	serverCert tls.Certificate   // 服务端证书
}

// newClientCertEngine 创建测试用引擎。
// @param t 测试上下文
// @return 引擎
func newClientCertEngine(t *testing.T) *engine.Engine {
	t.Helper()
	app, err := engine.New(engine.Config{HistoryLevel: engine.HistoryAudit})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	t.Cleanup(func() {
		_ = app.Close(context.Background())
	})
	return app
}

// randomSerial 生成随机证书序列号。
// @param t 测试上下文
// @return 序列号
func randomSerial(t *testing.T) *big.Int {
	t.Helper()
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	if err != nil {
		t.Fatalf("生成证书序列号失败: %v", err)
	}
	return serial
}

// issueCertificate 使用测试 CA 签发证书。
// @param t 测试上下文
// @param template 证书模板
// @return 证书
func (p *testPKI) issueCertificate(t *testing.T, template *x509.Certificate) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("生成私钥失败: %v", err)
	}
	der, err := x509.CreateCertificate(rand.Reader, template, p.caCert, &key.PublicKey, p.caKey)
	if err != nil {
		t.Fatalf("签发证书失败: %v", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// issueClientCertificate 签发客户端证书。
// @param t 测试上下文
// @param cn 通用名
// @param organizations 组织列表
// @param units 组织单元列表
// @return 客户端证书
func (p *testPKI) issueClientCertificate(t *testing.T, cn string, organizations, units []string) tls.Certificate {
	t.Helper()
	return p.issueCertificate(t, &x509.Certificate{
		SerialNumber: randomSerial(t),
		Subject: pkix.Name{
			CommonName:         cn,
			Organization:       organizations,
			OrganizationalUnit: units,
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	})
}

// newSelfSignedClientCertificate 生成不受 CA 信任的自签客户端证书。
// @param t 测试上下文
// @param cn 通用名
// @return 客户端证书
func newSelfSignedClientCertificate(t *testing.T, cn string) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("生成自签私钥失败: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          randomSerial(t),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("生成自签证书失败: %v", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// newTestPKI 生成测试 CA 与带 127.0.0.1 SAN 的服务端证书。
// @param t 测试上下文
// @return 测试证书体系
func newTestPKI(t *testing.T) *testPKI {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("生成 CA 私钥失败: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          randomSerial(t),
		Subject:               pkix.Name{CommonName: "flowtide-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("生成 CA 证书失败: %v", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("解析 CA 证书失败: %v", err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(caCert)

	pki := &testPKI{caCert: caCert, caKey: caKey, roots: roots}
	pki.serverCert = pki.issueCertificate(t, &x509.Certificate{
		SerialNumber: randomSerial(t),
		Subject:      pkix.Name{CommonName: "flowtide-test-server"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		DNSNames:     []string{"localhost"},
	})
	return pki
}

// startMTLSTestServer 启动真实 TLS 测试服务。
// @param t 测试上下文
// @param pki 测试证书体系
// @param handler 下游处理器
// @param clientAuth TLS 客户端认证模式
// @return 测试服务
func startMTLSTestServer(t *testing.T, pki *testPKI, handler http.Handler, clientAuth tls.ClientAuthType) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(handler)
	// 固定 TLS 1.2，保证客户端证书在握手阶段完成校验，行为可预期
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{pki.serverCert},
		ClientAuth:   clientAuth,
		ClientCAs:    pki.roots,
		MinVersion:   tls.VersionTLS12,
		MaxVersion:   tls.VersionTLS12,
	}
	server.StartTLS()
	t.Cleanup(server.Close)
	return server
}

// newMTLSClient 创建信任测试 CA 的 HTTPS 客户端。
// @param pki 测试证书体系
// @param certificates 客户端证书，可为空
// @return HTTP 客户端
func newMTLSClient(pki *testPKI, certificates ...tls.Certificate) *http.Client {
	config := &tls.Config{
		RootCAs:    pki.roots,
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS12,
	}
	// 仅在显式提供证书时启用客户端认证，便于覆盖无证书场景
	if len(certificates) > 0 {
		config.Certificates = certificates
	}
	return &http.Client{
		Transport: &http.Transport{TLSClientConfig: config},
		Timeout:   5 * time.Second,
	}
}

// newForcedCertificateClient 创建强制发送指定客户端证书的 HTTPS 客户端。
// 用于模拟不受 CA 信任但确实随握手发送的自签证书。
// @param pki 测试证书体系
// @param certificate 客户端证书
// @return HTTP 客户端
func newForcedCertificateClient(pki *testPKI, certificate tls.Certificate) *http.Client {
	config := &tls.Config{
		RootCAs:    pki.roots,
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS12,
	}
	// 忽略服务端可接受 CA 列表，确保自签证书一定被发送
	config.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
		return &certificate, nil
	}
	return &http.Client{
		Transport: &http.Transport{TLSClientConfig: config},
		Timeout:   5 * time.Second,
	}
}

// doMTLSRequest 发送可改写的 GET 请求。
// @param client HTTP 客户端
// @param url 地址
// @param mutate 请求改写函数，可为空
// @return 响应
// @return 错误
func doMTLSRequest(client *http.Client, url string, mutate func(*http.Request)) (*http.Response, error) {
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if mutate != nil {
		mutate(request)
	}
	return client.Do(request)
}

// expectStatus 断言响应状态码。
// @param t 测试上下文
// @param response 响应
// @param err 请求错误
// @param want 期望状态码
func expectStatus(t *testing.T, response *http.Response, err error, want int) {
	t.Helper()
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode != want {
		t.Fatalf("期望状态码 %d，实际 %d", want, response.StatusCode)
	}
}

// withTenantHeader 返回设置租户头的请求改写函数。
// @param tenantID 租户
// @return 改写函数
func withTenantHeader(tenantID string) func(*http.Request) {
	return func(request *http.Request) {
		request.Header.Set("X-Tenant-Id", tenantID)
	}
}

// TestClientCertificateIdentityAuthenticationRequired 验证开启选项后无 HTTP 凭据也强制认证。
// @param t 测试上下文
func TestClientCertificateIdentityAuthenticationRequired(t *testing.T) {
	app := newClientCertEngine(t)
	server := httptest.NewServer(New(app, WithClientCertificateIdentity()).Handler())
	t.Cleanup(server.Close)

	response, err := http.Get(server.URL + "/service/management/engine")
	expectStatus(t, response, err, http.StatusUnauthorized)

	response, err = http.Get(server.URL + "/healthz")
	expectStatus(t, response, err, http.StatusOK)
}

// TestClientCertificateIdentityTrusted 验证受信证书认证与 O 租户绑定。
// @param t 测试上下文
func TestClientCertificateIdentityTrusted(t *testing.T) {
	app := newClientCertEngine(t)
	pki := newTestPKI(t)
	server := startMTLSTestServer(t, pki, New(app, WithClientCertificateIdentity()).Handler(), tls.RequireAndVerifyClientCert)
	client := newMTLSClient(pki, pki.issueClientCertificate(t, "alice", []string{"tenant-a"}, []string{"team-a"}))

	response, err := doMTLSRequest(client, server.URL+"/service/management/engine", nil)
	expectStatus(t, response, err, http.StatusOK)

	response, err = doMTLSRequest(client, server.URL+"/service/management/engine", withTenantHeader("tenant-a"))
	expectStatus(t, response, err, http.StatusOK)

	response, err = doMTLSRequest(client, server.URL+"/service/management/engine", withTenantHeader("tenant-b"))
	expectStatus(t, response, err, http.StatusForbidden)

	// 证书租户取 O 第一项，显式选择 OU 同样视为越权
	response, err = doMTLSRequest(client, server.URL+"/service/management/engine", withTenantHeader("team-a"))
	expectStatus(t, response, err, http.StatusForbidden)
}

// TestClientCertificateIdentityTenantFallback 验证 O 为空时回退 OU。
// @param t 测试上下文
func TestClientCertificateIdentityTenantFallback(t *testing.T) {
	app := newClientCertEngine(t)
	pki := newTestPKI(t)
	server := startMTLSTestServer(t, pki, New(app, WithClientCertificateIdentity()).Handler(), tls.RequireAndVerifyClientCert)
	client := newMTLSClient(pki, pki.issueClientCertificate(t, "bob", nil, []string{"tenant-ou"}))

	response, err := doMTLSRequest(client, server.URL+"/service/management/engine", nil)
	expectStatus(t, response, err, http.StatusOK)

	response, err = doMTLSRequest(client, server.URL+"/service/management/engine", withTenantHeader("tenant-ou"))
	expectStatus(t, response, err, http.StatusOK)

	response, err = doMTLSRequest(client, server.URL+"/service/management/engine", withTenantHeader("tenant-a"))
	expectStatus(t, response, err, http.StatusForbidden)
}

// TestClientCertificateIdentityEmptyTenant 验证无 O/OU 时为空租户。
// @param t 测试上下文
func TestClientCertificateIdentityEmptyTenant(t *testing.T) {
	app := newClientCertEngine(t)
	pki := newTestPKI(t)
	server := startMTLSTestServer(t, pki, New(app, WithClientCertificateIdentity()).Handler(), tls.RequireAndVerifyClientCert)
	client := newMTLSClient(pki, pki.issueClientCertificate(t, "carol", nil, nil))

	response, err := doMTLSRequest(client, server.URL+"/service/management/engine", nil)
	expectStatus(t, response, err, http.StatusOK)

	// 空租户不允许客户端显式选择任何其他租户
	response, err = doMTLSRequest(client, server.URL+"/service/management/engine", withTenantHeader("tenant-x"))
	expectStatus(t, response, err, http.StatusForbidden)
}

// TestClientCertificateIdentityEmptyCommonName 验证 CN 为空时认证失败。
// @param t 测试上下文
func TestClientCertificateIdentityEmptyCommonName(t *testing.T) {
	app := newClientCertEngine(t)
	pki := newTestPKI(t)
	server := startMTLSTestServer(t, pki, New(app, WithClientCertificateIdentity()).Handler(), tls.RequireAndVerifyClientCert)
	client := newMTLSClient(pki, pki.issueClientCertificate(t, "", []string{"tenant-a"}, nil))

	response, err := doMTLSRequest(client, server.URL+"/service/management/engine", nil)
	expectStatus(t, response, err, http.StatusUnauthorized)
}

// TestClientCertificateIdentityMissingCertificate 验证无客户端证书的两种拒绝路径。
// @param t 测试上下文
func TestClientCertificateIdentityMissingCertificate(t *testing.T) {
	app := newClientCertEngine(t)
	pki := newTestPKI(t)
	handler := New(app, WithClientCertificateIdentity()).Handler()

	// 强制要求证书时，缺失证书在 TLS 握手阶段即被拒绝，不会进入 HTTP 层
	strict := startMTLSTestServer(t, pki, handler, tls.RequireAndVerifyClientCert)
	strictClient := newMTLSClient(pki)
	if response, err := doMTLSRequest(strictClient, strict.URL+"/service/management/engine", nil); err == nil {
		_ = response.Body.Close()
		t.Fatalf("无客户端证书应被 TLS 拒绝")
	}

	// TLS 允许无证书（如反向代理终止 TLS）时，应用层返回 401
	optional := startMTLSTestServer(t, pki, handler, tls.RequestClientCert)
	optionalClient := newMTLSClient(pki)
	response, err := doMTLSRequest(optionalClient, optional.URL+"/service/management/engine", nil)
	expectStatus(t, response, err, http.StatusUnauthorized)
}

// TestClientCertificateIdentityUntrustedCertificate 验证自签证书不被当作身份。
// @param t 测试上下文
func TestClientCertificateIdentityUntrustedCertificate(t *testing.T) {
	app := newClientCertEngine(t)
	pki := newTestPKI(t)
	handler := New(app, WithClientCertificateIdentity()).Handler()
	selfSigned := newSelfSignedClientCertificate(t, "eve")

	// 要求并校验证书时，自签证书在 TLS 层被拒绝
	strict := startMTLSTestServer(t, pki, handler, tls.RequireAndVerifyClientCert)
	strictClient := newForcedCertificateClient(pki, selfSigned)
	if response, err := doMTLSRequest(strictClient, strict.URL+"/service/management/engine", nil); err == nil {
		_ = response.Body.Close()
		t.Fatalf("自签客户端证书应被 TLS 拒绝")
	}

	// 即使 TLS 不校验，未验证链也不会被应用层接受
	optional := startMTLSTestServer(t, pki, handler, tls.RequestClientCert)
	optionalClient := newForcedCertificateClient(pki, selfSigned)
	response, err := doMTLSRequest(optionalClient, optional.URL+"/service/management/engine", nil)
	expectStatus(t, response, err, http.StatusUnauthorized)
}

// TestClientCertificateIdentityBasicPriority 验证 Basic 凭据优先且失败不回退证书。
// @param t 测试上下文
func TestClientCertificateIdentityBasicPriority(t *testing.T) {
	app := newClientCertEngine(t)
	pki := newTestPKI(t)
	handler := New(app,
		WithClientCertificateIdentity(),
		WithBasicAuth(map[string]string{"alice": "secret"}),
		WithTenantBindings(map[string]string{"alice": "tenant-basic"}),
	).Handler()
	server := startMTLSTestServer(t, pki, handler, tls.RequireAndVerifyClientCert)
	client := newMTLSClient(pki, pki.issueClientCertificate(t, "alice", []string{"tenant-a"}, nil))

	withBasic := func(user, password, tenantID string) func(*http.Request) {
		return func(request *http.Request) {
			request.SetBasicAuth(user, password)
			if tenantID != "" {
				request.Header.Set("X-Tenant-Id", tenantID)
			}
		}
	}

	// 合法 Basic 凭据生效，租户来自 Basic 绑定而非证书
	response, err := doMTLSRequest(client, server.URL+"/service/management/engine", withBasic("alice", "secret", "tenant-basic"))
	expectStatus(t, response, err, http.StatusOK)
	response, err = doMTLSRequest(client, server.URL+"/service/management/engine", withBasic("alice", "secret", "tenant-a"))
	expectStatus(t, response, err, http.StatusForbidden)

	// HTTP 凭据失败不得回退到证书，否则可绕过错误密码
	response, err = doMTLSRequest(client, server.URL+"/service/management/engine", withBasic("alice", "bad", ""))
	expectStatus(t, response, err, http.StatusUnauthorized)
}

// TestClientCertificateIdentityBearerPriority 验证 Bearer 凭据优先且失败不回退证书。
// @param t 测试上下文
func TestClientCertificateIdentityBearerPriority(t *testing.T) {
	app := newClientCertEngine(t)
	pki := newTestPKI(t)
	server := startMTLSTestServer(t, pki, New(app, WithClientCertificateIdentity(), WithBearerToken("token-1")).Handler(), tls.RequireAndVerifyClientCert)
	client := newMTLSClient(pki, pki.issueClientCertificate(t, "alice", []string{"tenant-a"}, nil))

	withBearer := func(token string) func(*http.Request) {
		return func(request *http.Request) {
			request.Header.Set("Authorization", "Bearer "+token)
		}
	}

	response, err := doMTLSRequest(client, server.URL+"/service/management/engine", withBearer("token-1"))
	expectStatus(t, response, err, http.StatusOK)

	response, err = doMTLSRequest(client, server.URL+"/service/management/engine", withBearer("bad"))
	expectStatus(t, response, err, http.StatusUnauthorized)
}

// TestClientCertificateIdentityJWTPriority 验证 JWT 凭据优先且失败不回退证书。
// @param t 测试上下文
func TestClientCertificateIdentityJWTPriority(t *testing.T) {
	app := newClientCertEngine(t)
	pki := newTestPKI(t)
	const secret = "client-cert-jwt-secret"
	server := startMTLSTestServer(t, pki, New(app, WithClientCertificateIdentity(), WithJWT(JWTConfig{HS256Secret: secret})).Handler(), tls.RequireAndVerifyClientCert)
	client := newMTLSClient(pki, pki.issueClientCertificate(t, "alice", []string{"tenant-a"}, nil))

	withBearer := func(token string) func(*http.Request) {
		return func(request *http.Request) {
			request.Header.Set("Authorization", "Bearer "+token)
		}
	}

	token := testSignHS256(t, secret, map[string]any{"sub": "jwt-user", "exp": time.Now().Add(time.Hour).Unix()})
	response, err := doMTLSRequest(client, server.URL+"/service/management/engine", withBearer(token))
	expectStatus(t, response, err, http.StatusOK)

	response, err = doMTLSRequest(client, server.URL+"/service/management/engine", withBearer("not-a-jwt"))
	expectStatus(t, response, err, http.StatusUnauthorized)
}

// TestClientCertificateIdentityRBAC 验证证书身份未映射角色时的授权语义。
// @param t 测试上下文
func TestClientCertificateIdentityRBAC(t *testing.T) {
	app := newClientCertEngine(t)
	pki := newTestPKI(t)
	client := newMTLSClient(pki, pki.issueClientCertificate(t, "alice", []string{"tenant-a"}, nil))

	// 配置 rolePermissions 后证书身份无角色，应 403
	withRBAC := startMTLSTestServer(t, pki, New(app,
		WithClientCertificateIdentity(),
		WithRBAC(nil, map[string][]string{"reader": {"read"}}),
	).Handler(), tls.RequireAndVerifyClientCert)
	response, err := doMTLSRequest(client, withRBAC.URL+"/service/repository/deployments", nil)
	expectStatus(t, response, err, http.StatusForbidden)

	// 显式主体角色映射应放行只读请求，并继续拒绝管理端点。
	mapped := startMTLSTestServer(t, pki, New(app,
		WithClientCertificateIdentity(),
		WithRBAC(map[string][]string{"alice": {"reader"}}, map[string][]string{"reader": {"read"}}),
	).Handler(), tls.RequireAndVerifyClientCert)
	response, err = doMTLSRequest(client, mapped.URL+"/service/repository/deployments", nil)
	expectStatus(t, response, err, http.StatusOK)
	response, err = doMTLSRequest(client, mapped.URL+"/service/management/engine", nil)
	expectStatus(t, response, err, http.StatusForbidden)

	// 未配置 rolePermissions 时放行证书身份
	withoutRBAC := startMTLSTestServer(t, pki, New(app, WithClientCertificateIdentity()).Handler(), tls.RequireAndVerifyClientCert)
	response, err = doMTLSRequest(client, withoutRBAC.URL+"/service/repository/deployments", nil)
	expectStatus(t, response, err, http.StatusOK)

	// 启用身份库但未配置 rolePermissions 时，证书身份同样放行
	withIdentity := startMTLSTestServer(t, pki, New(app, WithClientCertificateIdentity(), WithIdentityAuth()).Handler(), tls.RequireAndVerifyClientCert)
	response, err = doMTLSRequest(client, withIdentity.URL+"/service/management/engine", nil)
	expectStatus(t, response, err, http.StatusOK)
}

// TestClientCertificateTenantFromSubject 验证 O 优先后 OU 的租户推导。
// @param t 测试上下文
func TestClientCertificateTenantFromSubject(t *testing.T) {
	cases := []struct {
		name    string
		subject pkix.Name
		want    string
	}{
		{"仅组织", pkix.Name{Organization: []string{"tenant-a"}}, "tenant-a"},
		{"仅组织单元", pkix.Name{OrganizationalUnit: []string{"tenant-ou"}}, "tenant-ou"},
		{"组织优先于组织单元", pkix.Name{Organization: []string{"tenant-a"}, OrganizationalUnit: []string{"tenant-ou"}}, "tenant-a"},
		{"组织空白回退组织单元", pkix.Name{Organization: []string{"  "}, OrganizationalUnit: []string{"tenant-ou"}}, "tenant-ou"},
		{"组织空白且无组织单元", pkix.Name{Organization: []string{"  "}}, ""},
		{"组织单元空白", pkix.Name{OrganizationalUnit: []string{"  "}}, ""},
		{"全空", pkix.Name{}, ""},
	}
	for _, tc := range cases {
		if got := clientCertificateTenant(tc.subject); got != tc.want {
			t.Fatalf("%s: 期望租户 %q，实际 %q", tc.name, tc.want, got)
		}
	}
}

// TestClientCertificateIdentityFromRequest 直接验证证书身份解析的边界条件。
// @param t 测试上下文
func TestClientCertificateIdentityFromRequest(t *testing.T) {
	cert := &x509.Certificate{Subject: pkix.Name{CommonName: "alice", Organization: []string{"tenant-a"}}}
	verified := func(certificate *x509.Certificate) *tls.ConnectionState {
		return &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{certificate},
			VerifiedChains:   [][]*x509.Certificate{{certificate}},
		}
	}
	cases := []struct {
		name       string
		server     *Server
		path       string
		state      *tls.ConnectionState
		wantOK     bool
		wantSub    string
		wantTenant string
	}{
		{"选项关闭", &Server{}, "/service/management/engine", verified(cert), false, "", ""},
		{"非 service 路径", &Server{clientCertIdentity: true}, "/healthz", verified(cert), false, "", ""},
		{"service 前缀无斜杠", &Server{clientCertIdentity: true}, "/service", verified(cert), false, "", ""},
		{"无 TLS", &Server{clientCertIdentity: true}, "/service/management/engine", nil, false, "", ""},
		{"无客户端证书", &Server{clientCertIdentity: true}, "/service/management/engine", &tls.ConnectionState{}, false, "", ""},
		{"未验证链", &Server{clientCertIdentity: true}, "/service/management/engine", &tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}}, false, "", ""},
		{"验证链为空", &Server{clientCertIdentity: true}, "/service/management/engine", &tls.ConnectionState{VerifiedChains: [][]*x509.Certificate{{}}}, false, "", ""},
		{"空通用名", &Server{clientCertIdentity: true}, "/service/management/engine", verified(&x509.Certificate{Subject: pkix.Name{Organization: []string{"tenant-a"}}}), false, "", ""},
		{"验证通过", &Server{clientCertIdentity: true}, "/service/management/engine", verified(cert), true, "alice", "tenant-a"},
	}
	for _, tc := range cases {
		request := httptest.NewRequest(http.MethodGet, tc.path, nil)
		request.TLS = tc.state
		identity, ok := tc.server.clientCertificateIdentity(request)
		if ok != tc.wantOK {
			t.Fatalf("%s: 期望认证结果 %v，实际 %v", tc.name, tc.wantOK, ok)
		}
		if identity.Subject != tc.wantSub || identity.Tenant != tc.wantTenant {
			t.Fatalf("%s: 身份不符，期望 %q/%q，实际 %q/%q", tc.name, tc.wantSub, tc.wantTenant, identity.Subject, identity.Tenant)
		}
		if len(identity.Roles) != 0 {
			t.Fatalf("%s: 证书身份不应携带角色", tc.name)
		}
	}
}
