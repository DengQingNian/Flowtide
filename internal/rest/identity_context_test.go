package rest

import "testing"

// TestWithTenantAdministrators 验证跨租户管理员选项复制全部身份。
// @param t 测试上下文
// @return 无
func TestWithTenantAdministrators(t *testing.T) {
	server := &Server{}
	WithTenantAdministrators("alice", "bob")(server)
	if !server.tenantAdministrators["alice"] || !server.tenantAdministrators["bob"] {
		t.Fatalf("租户管理员未完整写入: %#v", server.tenantAdministrators)
	}
}
