package rest

import (
	"crypto/x509/pkix"
	"net/http"
	"strings"
)

// WithClientCertificateIdentity 启用 mTLS 客户端证书身份映射。
// 开启后，仅 /service/ 路径在未携带 Authorization 头时使用 TLS 已验证的客户端证书认证。
// @return 选项
func WithClientCertificateIdentity() Option {
	return func(server *Server) {
		server.clientCertIdentity = true
	}
}

// clientCertificateIdentity 从 TLS 已验证的客户端证书构造身份。
// 仅在选项开启、路径为 /service/、未携带 Authorization 头且客户端证书链已通过 TLS 校验时生效。
// @param r HTTP 请求
// @return 身份
// @return 是否认证成功
func (s *Server) clientCertificateIdentity(r *http.Request) (jwtIdentity, bool) {
	// 选项未开启时完全不读取客户端证书
	if !s.clientCertIdentity {
		return jwtIdentity{}, false
	}
	// 认证仅对 /service/ 业务接口生效
	if !strings.HasPrefix(r.URL.Path, "/service/") {
		return jwtIdentity{}, false
	}
	// 未启用 TLS 或握手未完成时没有可信证书
	if r.TLS == nil {
		return jwtIdentity{}, false
	}
	// 只信任 TLS 层已验证的证书链，未验证的自签证书不生效
	if len(r.TLS.VerifiedChains) == 0 || len(r.TLS.VerifiedChains[0]) == 0 || r.TLS.VerifiedChains[0][0] == nil {
		return jwtIdentity{}, false
	}
	subject := strings.TrimSpace(r.TLS.VerifiedChains[0][0].Subject.CommonName)
	// 空 CN 无法映射稳定身份，认证失败
	if subject == "" {
		return jwtIdentity{}, false
	}
	return jwtIdentity{Subject: subject, Tenant: clientCertificateTenant(r.TLS.VerifiedChains[0][0].Subject)}, true
}

// clientCertificateTenant 按 O 优先、OU 兜底的顺序推导证书租户。
// @param subject 证书主题
// @return 租户标识，O 与 OU 均为空时返回空租户
func clientCertificateTenant(subject pkix.Name) string {
	// 优先使用 Organization 第一项
	if len(subject.Organization) > 0 {
		if tenantID := strings.TrimSpace(subject.Organization[0]); tenantID != "" {
			return tenantID
		}
	}
	// O 缺失或为空白时回退到 OrganizationalUnit 第一项
	if len(subject.OrganizationalUnit) > 0 {
		return strings.TrimSpace(subject.OrganizationalUnit[0])
	}
	return ""
}
