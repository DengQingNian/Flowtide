package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"

	"github.com/DengQingNian/Flowtide/internal/rest"
)

// securityConfig 是标准二进制可加载的认证及可观测配置，配置内容可能含密钥，禁止打印。
type securityConfig struct {
	BasicUsers           map[string]string   `json:"basicUsers"`           // 静态用户
	BearerToken          string              `json:"bearerToken"`          // 静态服务令牌
	IdentityAuth         bool                `json:"identityAuth"`         // 启用身份库认证
	JWT                  *rest.JWTConfig     `json:"jwt"`                  // JWT 验签与声明配置
	UserRoles            map[string][]string `json:"userRoles"`            // 主体角色映射
	RolePermissions      map[string][]string `json:"rolePermissions"`      // 角色权限映射
	TenantBindings       map[string]string   `json:"tenantBindings"`       // 固定租户绑定
	TenantAdministrators []string            `json:"tenantAdministrators"` // 可显式选择租户的主体
	AuditCapacity        int                 `json:"auditCapacity"`        // 内存审计保留数量
	Metrics              bool                `json:"metrics"`              // HTTP 指标
}

// loadSecurityOptions 在启动数据库前解析并验证安全配置，返回统一 REST 选项。
// @param path 配置文件路径，空值保留原有命令行行为
// @return REST 选项
// @return 不包含文件内容或密钥的中文错误
func loadSecurityOptions(path string) ([]rest.Option, error) {
	if path == "" {
		return nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("读取安全配置失败，请检查路径与权限")
	}
	defer func() { _ = file.Close() }()
	decoder := json.NewDecoder(io.LimitReader(file, 1<<20))
	decoder.DisallowUnknownFields()
	var cfg *securityConfig
	if err := decoder.Decode(&cfg); err != nil || cfg == nil {
		return nil, errors.New("安全配置必须是合法 JSON 对象且不能包含未知字段")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, errors.New("安全配置只能包含一个 JSON 对象")
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	options := []rest.Option{rest.WithTenantBindings(cfg.TenantBindings), rest.WithTenantAdministrators(cfg.TenantAdministrators...)}
	// 仅为实际配置的能力添加选项，避免关闭原有命令行认证。
	if len(cfg.BasicUsers) > 0 {
		options = append(options, rest.WithBasicAuth(cfg.BasicUsers))
	}
	if cfg.BearerToken != "" {
		options = append(options, rest.WithBearerToken(cfg.BearerToken))
	}
	if cfg.IdentityAuth {
		options = append(options, rest.WithIdentityAuth())
	}
	if cfg.JWT != nil {
		options = append(options, rest.WithJWT(*cfg.JWT))
	}
	if len(cfg.RolePermissions) > 0 {
		options = append(options, rest.WithRBAC(cfg.UserRoles, cfg.RolePermissions))
	}
	if cfg.AuditCapacity > 0 {
		options = append(options, rest.WithAuditLog(cfg.AuditCapacity))
	}
	if cfg.Metrics {
		options = append(options, rest.WithMetrics())
	}
	return options, nil
}

// validate 验证权限白名单、角色引用、静态用户及 JWT 必需参数。
// @return 配置错误
func (c *securityConfig) validate() error {
	if c.AuditCapacity < 0 {
		return errors.New("审计容量不能为负数")
	}
	for user, password := range c.BasicUsers {
		if user == "" || password == "" {
			return errors.New("安全配置的 Basic 用户名及密码不能为空")
		}
	}
	for role, permissions := range c.RolePermissions {
		if role == "" {
			return errors.New("角色名称不能为空")
		}
		for _, permission := range permissions {
			if permission != "read" && permission != "write" && permission != "admin" {
				return errors.New("角色权限仅支持 read、write、admin")
			}
		}
	}
	for _, roles := range c.UserRoles {
		for _, role := range roles {
			if _, ok := c.RolePermissions[role]; !ok {
				return errors.New("主体引用了未定义的角色")
			}
		}
	}
	if c.JWT != nil {
		if c.JWT.HS256Secret == "" && c.JWT.JWKSURL == "" {
			return errors.New("JWT 必须配置共享密钥或 JWKS 地址")
		}
		if c.JWT.JWKSURL != "" {
			endpoint, err := url.Parse(c.JWT.JWKSURL)
			if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.User != nil {
				return errors.New("JWKS 地址必须是无内嵌凭据的 HTTP 或 HTTPS 地址")
			}
		}
	}
	return nil
}
