package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testCA 测试用证书颁发机构。
type testCA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pem  []byte
}

// newTestCA 生成自签名测试 CA，私钥仅保存在内存中。
// @param t 测试上下文
// @return 测试 CA
func newTestCA(t *testing.T) *testCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("生成 CA 私钥失败: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          newTestSerial(t),
		Subject:               pkix.Name{CommonName: "flowable-go-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("签发 CA 证书失败: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("解析 CA 证书失败: %v", err)
	}
	return &testCA{cert: cert, key: key, pem: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}
}

// newTestSerial 生成随机证书序列号。
// @param t 测试上下文
// @return 序列号
func newTestSerial(t *testing.T) *big.Int {
	t.Helper()
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, limit)
	if err != nil {
		t.Fatalf("生成证书序列号失败: %v", err)
	}
	return serial
}

// issueTestCert 由给定 CA 签发叶子证书，可用于服务端或客户端。
// @param t 测试上下文
// @param ca 签发 CA
// @param commonName 证书主体通用名
// @param server 为 true 时生成带 127.0.0.1 SAN 的服务端证书
// @return TLS 证书
// @return 证书 PEM 内容
func issueTestCert(t *testing.T, ca *testCA, commonName string, server bool) (tls.Certificate, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("生成叶子私钥失败: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: newTestSerial(t),
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	// 服务端证书必须覆盖回环地址，客户端证书只校验证书链与用途。
	if server {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		template.IPAddresses = []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
		template.DNSNames = []string{"localhost"}
	} else {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatalf("签发叶子证书失败: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("编码叶子私钥失败: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("构造 TLS 证书失败: %v", err)
	}
	return tlsCert, certPEM
}

// writeTestFile 将内容写入指定目录下的文件。
// @param t 测试上下文
// @param dir 目标目录
// @param name 文件名
// @param content 文件内容
// @return 文件绝对路径
func writeTestFile(t *testing.T, dir, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}
	return path
}

// TestBuildTLSConfig 校验参数组合、CA 解析与认证模式映射。
// @param t 测试上下文
// @return 无
func TestBuildTLSConfig(t *testing.T) {
	dir := t.TempDir()
	ca := newTestCA(t)
	otherCA := newTestCA(t)
	caFile := writeTestFile(t, dir, "ca.pem", ca.pem)
	multiCAFile := writeTestFile(t, dir, "multi-ca.pem", append(append([]byte{}, ca.pem...), otherCA.pem...))
	garbageFile := writeTestFile(t, dir, "garbage.pem", []byte("这不是 PEM 证书"))
	privateKeyFile := writeTestFile(t, dir, "private.pem",
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("SECRET-MATERIAL")}))
	missingFile := filepath.Join(dir, "absent.pem")
	const (
		serverCert = "server.crt"
		serverKey  = "server.key"
	)
	cases := []struct {
		name        string
		cert        string
		key         string
		clientCA    string
		clientAuth  string
		minVersion  string
		wantErr     string
		wantNil     bool
		wantAuth    tls.ClientAuthType
		wantVersion uint16
	}{
		{name: "未启用TLS使用默认值", clientAuth: "none", minVersion: "1.2", wantNil: true},
		{name: "未启用TLS且参数为空", wantNil: true},
		{name: "仅配置证书", cert: serverCert, clientAuth: "none", minVersion: "1.2", wantErr: "必须同时配置"},
		{name: "仅配置私钥", key: serverKey, clientAuth: "none", minVersion: "1.2", wantErr: "必须同时配置"},
		{name: "未启用TLS却配置CA", clientCA: caFile, clientAuth: "none", minVersion: "1.2", wantErr: "-tls-client-ca"},
		{name: "未启用TLS却启用客户端认证", clientAuth: "require-verify", minVersion: "1.2", wantErr: "-tls-client-auth=require-verify"},
		{name: "启用TLS默认单向", cert: serverCert, key: serverKey, clientAuth: "none", minVersion: "1.2", wantAuth: tls.NoClientCert, wantVersion: tls.VersionTLS12},
		{name: "启用TLS且未指定版本", cert: serverCert, key: serverKey, wantAuth: tls.NoClientCert, wantVersion: tls.VersionTLS12},
		{name: "配置CA隐式要求校验", cert: serverCert, key: serverKey, clientCA: caFile, clientAuth: "none", minVersion: "1.2", wantAuth: tls.RequireAndVerifyClientCert, wantVersion: tls.VersionTLS12},
		{name: "显式require-verify", cert: serverCert, key: serverKey, clientCA: caFile, clientAuth: "require-verify", minVersion: "1.2", wantAuth: tls.RequireAndVerifyClientCert, wantVersion: tls.VersionTLS12},
		{name: "request模式仅请求", cert: serverCert, key: serverKey, clientCA: caFile, clientAuth: "request", minVersion: "1.2", wantAuth: tls.RequestClientCert, wantVersion: tls.VersionTLS12},
		{name: "require-any不校验证书链", cert: serverCert, key: serverKey, clientCA: caFile, clientAuth: "require-any", minVersion: "1.2", wantAuth: tls.RequireAnyClientCert, wantVersion: tls.VersionTLS12},
		{name: "认证模式忽略大小写与空白", cert: serverCert, key: serverKey, clientCA: caFile, clientAuth: " Require-Verify ", minVersion: "1.2", wantAuth: tls.RequireAndVerifyClientCert, wantVersion: tls.VersionTLS12},
		{name: "TLS1.3最低版本", cert: serverCert, key: serverKey, clientAuth: "none", minVersion: "1.3", wantAuth: tls.NoClientCert, wantVersion: tls.VersionTLS13},
		{name: "TLS1.3别名", cert: serverCert, key: serverKey, clientAuth: "none", minVersion: "tls1.3", wantAuth: tls.NoClientCert, wantVersion: tls.VersionTLS13},
		{name: "启用客户端认证但缺少CA", cert: serverCert, key: serverKey, clientAuth: "require-verify", minVersion: "1.2", wantErr: "需要配置 -tls-client-ca"},
		{name: "非法认证模式", cert: serverCert, key: serverKey, clientCA: caFile, clientAuth: "mutual", minVersion: "1.2", wantErr: "非法的 -tls-client-auth"},
		{name: "非法最低版本", cert: serverCert, key: serverKey, clientAuth: "none", minVersion: "1.1", wantErr: "非法的 -tls-min-version"},
		{name: "CA文件不存在", cert: serverCert, key: serverKey, clientCA: missingFile, clientAuth: "require-verify", minVersion: "1.2", wantErr: "读取客户端 CA 证书失败"},
		{name: "CA文件不是PEM", cert: serverCert, key: serverKey, clientCA: garbageFile, clientAuth: "require-verify", minVersion: "1.2", wantErr: "没有有效的 PEM 证书"},
		{name: "CA文件只有私钥", cert: serverCert, key: serverKey, clientCA: privateKeyFile, clientAuth: "require-verify", minVersion: "1.2", wantErr: "没有有效的 PEM 证书"},
		{name: "CA文件包含多个证书", cert: serverCert, key: serverKey, clientCA: multiCAFile, clientAuth: "require-verify", minVersion: "1.2", wantAuth: tls.RequireAndVerifyClientCert, wantVersion: tls.VersionTLS12},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := buildTLSConfig(tc.cert, tc.key, tc.clientCA, tc.clientAuth, tc.minVersion)
			// 错误分支必须命中且不得回传半成品配置。
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("期望错误包含 %q，实际: %v", tc.wantErr, err)
				}
				if cfg != nil {
					t.Fatal("错误时不应返回配置")
				}
				return
			}
			if err != nil {
				t.Fatalf("未期望错误: %v", err)
			}
			// 未启用 TLS 时必须返回空配置，避免调用方误以为已启用。
			if tc.wantNil {
				if cfg != nil {
					t.Fatal("未启用 TLS 时应返回空配置")
				}
				return
			}
			if cfg == nil {
				t.Fatal("启用 TLS 时配置不应为空")
			}
			if cfg.ClientAuth != tc.wantAuth {
				t.Fatalf("认证模式 = %v，期望 %v", cfg.ClientAuth, tc.wantAuth)
			}
			if cfg.MinVersion != tc.wantVersion {
				t.Fatalf("最低版本 = %#x，期望 %#x", cfg.MinVersion, tc.wantVersion)
			}
			if tc.wantAuth == tls.NoClientCert && cfg.ClientCAs != nil {
				t.Fatal("未要求客户端证书时不应设置 CA 池")
			}
			if tc.wantAuth != tls.NoClientCert && cfg.ClientCAs == nil {
				t.Fatal("要求客户端证书时必须设置 CA 池")
			}
		})
	}
}

// TestBuildTLSConfigErrorDoesNotLeakSecrets 确认错误信息不包含证书或私钥内容。
// @param t 测试上下文
// @return 无
func TestBuildTLSConfigErrorDoesNotLeakSecrets(t *testing.T) {
	dir := t.TempDir()
	secret := "SECRET-MATERIAL"
	badCA := writeTestFile(t, dir, "bad-ca.pem", []byte("-----BEGIN PRIVATE KEY-----\n"+secret+"\n-----END PRIVATE KEY-----\n"))
	cfg, err := buildTLSConfig("server.crt", "server.key", badCA, "require-verify", "1.2")
	if err == nil {
		t.Fatal("非法 CA 文件应失败")
	}
	if cfg != nil {
		t.Fatal("错误时不应返回配置")
	}
	for _, forbidden := range []string{secret, "BEGIN", "PRIVATE KEY", "-----"} {
		if strings.Contains(err.Error(), forbidden) {
			t.Fatalf("错误信息泄漏敏感内容 %q: %v", forbidden, err)
		}
	}
}

// TestMTLSHandshake 使用真实 TLS 握手验证客户端证书校验。
// @param t 测试上下文
// @return 无
func TestMTLSHandshake(t *testing.T) {
	dir := t.TempDir()
	ca := newTestCA(t)
	caFile := writeTestFile(t, dir, "ca.pem", ca.pem)
	serverCert, _ := issueTestCert(t, ca, "flowable-server", true)
	clientCert, _ := issueTestCert(t, ca, "flowable-client", false)
	untrustedCA := newTestCA(t)
	untrustedCert, _ := issueTestCert(t, untrustedCA, "untrusted-client", false)

	cfg, err := buildTLSConfig("server.crt", "server.key", caFile, "require-verify", "1.2")
	if err != nil {
		t.Fatalf("构造 TLS 配置失败: %v", err)
	}
	cfg.Certificates = []tls.Certificate{serverCert}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	server.TLS = cfg
	server.StartTLS()
	defer server.Close()

	rootCAs := x509.NewCertPool()
	rootCAs.AddCert(ca.cert)
	cases := []struct {
		name         string
		certificates []tls.Certificate
		wantErr      bool
	}{
		{name: "无客户端证书被拒", wantErr: true},
		{name: "受信客户端证书通过", certificates: []tls.Certificate{clientCert}},
		{name: "未受信CA签发的客户端证书被拒", certificates: []tls.Certificate{untrustedCert}, wantErr: true},
	}
	for _, version := range []uint16{tls.VersionTLS12, tls.VersionTLS13} {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("%s-%s", tc.name, tls.VersionName(version)), func(t *testing.T) {
				client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{
					RootCAs:      rootCAs,
					Certificates: tc.certificates,
					MinVersion:   tls.VersionTLS12,
					MaxVersion:   version,
				}}}
				resp, err := client.Get(server.URL)
				// 被拒绝的握手在 TLS 1.3 下可能在读取响应时才报错，统一按请求失败处理。
				if tc.wantErr {
					if err == nil {
						_ = resp.Body.Close()
						t.Fatal("期望客户端证书校验失败")
					}
					return
				}
				if err != nil {
					t.Fatalf("受信客户端证书应通过: %v", err)
				}
				defer func() { _ = resp.Body.Close() }()
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatalf("读取响应失败: %v", err)
				}
				if resp.StatusCode != http.StatusOK || string(body) != "ok" {
					t.Fatalf("响应异常: %d %q", resp.StatusCode, body)
				}
			})
		}
	}
}

// TestValidateClientCertIdentity 验证证书身份映射必须启用本地 TLS、客户端 CA 与 require-verify。
// @param t 测试上下文
// @return 无
func TestValidateClientCertIdentity(t *testing.T) {
	cases := []struct {
		name       string // 场景
		cert       string // 服务端证书
		clientCA   string // 客户端 CA
		clientAuth string // 认证模式
		wantErr    bool   // 是否期望错误
	}{
		{"合法配置", "server.crt", "ca.pem", "require-verify", false},
		{"大小写与空白", "server.crt", "ca.pem", " Require-Verify ", false},
		{"未启用本地TLS", "", "ca.pem", "require-verify", true},
		{"缺少客户端CA", "server.crt", "", "require-verify", true},
		{"未要求校验证书", "server.crt", "ca.pem", "require-any", true},
		{"单向TLS", "server.crt", "ca.pem", "none", true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := validateClientCertIdentity(testCase.cert, testCase.clientCA, testCase.clientAuth)
			if (err != nil) != testCase.wantErr {
				t.Fatalf("错误=%v 期望错误=%v", err, testCase.wantErr)
			}
		})
	}
}
