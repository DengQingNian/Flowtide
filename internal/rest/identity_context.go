package rest

import (
	"crypto/subtle"
	"net/http"
)

// authIdentityKey 标识认证中间件建立的可信身份。
type authIdentityKey struct{}

// WithTenantBindings 为 Basic 用户和静态 bearer 服务账号绑定固定租户。
// @param bindings 用户到租户的映射，静态服务账号名为 bearer
// @return 服务选项
func WithTenantBindings(bindings map[string]string) Option {
	return func(server *Server) {
		server.tenantBindings = make(map[string]string, len(bindings))
		for user, tenantID := range bindings {
			server.tenantBindings[user] = tenantID
		}
	}
}

// WithTenantAdministrators 显式授权指定身份选择单个其他租户。
// @param subjects 允许跨租户的身份列表
// @return 服务选项
func WithTenantAdministrators(subjects ...string) Option {
	return func(server *Server) {
		server.tenantAdministrators = make(map[string]bool, len(subjects))
		for _, subject := range subjects {
			server.tenantAdministrators[subject] = true
		}
	}
}

// authenticateIdentity 校验凭据并生成仅由服务端决定的租户身份。
// @param r HTTP 请求
// @return 身份
// @return 是否认证成功
func (s *Server) authenticateIdentity(r *http.Request) (jwtIdentity, bool) {
	// 按静态服务账号、JWT、Basic 的顺序认证，与现有认证优先级一致。
	if token, ok := bearerTokenFromHeader(r.Header.Get("Authorization")); ok && s.bearerToken != "" && subtle.ConstantTimeCompare([]byte(token), []byte(s.bearerToken)) == 1 {
		return jwtIdentity{Subject: "bearer", Tenant: s.tenantBindings["bearer"]}, true
	}
	if s.jwt != nil {
		if identity, ok := s.jwt.verifyRequest(r); ok {
			return identity, true
		}
	}
	if user, password, ok := r.BasicAuth(); ok {
		expected, exists := s.basicUsers[user]
		if (exists && subtle.ConstantTimeCompare([]byte(password), []byte(expected)) == 1) || (s.identityAuth && s.engine.IdentityService().CheckPassword(r.Context(), user, password) == nil) {
			return jwtIdentity{Subject: user, Tenant: s.tenantBindings[user]}, true
		}
	}
	return jwtIdentity{}, false
}
