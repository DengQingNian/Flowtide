package mysql

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	driver "github.com/go-sql-driver/mysql"
)

// isolateConfigEnv 临时移除外部开发配置，保证单元测试不依赖调用方凭据。
// @param t 测试上下文
// @return 无
func isolateConfigEnv(t *testing.T) {
	t.Helper()
	// 先注册原值恢复，再删除变量，使文件配置能够参与优先级测试。
	for _, key := range []string{"db_mysql_host", "db_mysql_port", "db_mysql_user", "db_mysql_pwd", "db_mysql_db_name"} {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
}

// TestLoadConfig 验证配置优先级、默认值和敏感信息保护。
// @param t 测试上下文
// @return 无
func TestLoadConfig(t *testing.T) {
	isolateConfigEnv(t)
	// 使用临时凭据验证文件解析，并逐项覆盖环境变量与非法输入。
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("db_mysql_host=localhost\ndb_mysql_user=开发用户\ndb_mysql_pwd='p@ss:#word'\ndb_mysql_db_name=flowable\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 3306 || cfg.Password != "p@ss:#word" || cfg.Database != "flowable" {
		t.Fatal("配置解析错误")
	}
	t.Setenv("db_mysql_host", "::1")
	t.Setenv("db_mysql_port", "3307")
	cfg, err = LoadConfig(path)
	if err != nil || cfg.Host != "::1" || cfg.Port != 3307 {
		t.Fatalf("环境覆盖失败: %v", err)
	}
	dc := cfg.driverConfig()
	parsed, err := driver.ParseDSN(dc.FormatDSN())
	if err != nil || parsed.Passwd != cfg.Password || parsed.Addr != "[::1]:3307" || !parsed.ParseTime || parsed.Loc != time.UTC || !strings.Contains(dc.FormatDSN(), "charset=utf8mb4") {
		t.Fatalf("驱动配置错误: %v", err)
	}
	if dc.MultiStatements || dc.Timeout == 0 || dc.ReadTimeout == 0 || dc.WriteTimeout == 0 {
		t.Fatal("驱动边界配置错误")
	}
	for _, port := range []string{"abc", "0", "65536", "-1"} {
		t.Setenv("db_mysql_port", port)
		if _, err := LoadConfig(path); err == nil {
			t.Fatalf("非法端口 %s 应失败", port)
		}
	}
	t.Setenv("db_mysql_port", "3306")
	t.Setenv("db_mysql_user", "")
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("空用户应失败")
	}
	if err := os.WriteFile(path, []byte("bad key=p@ss-secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal("格式错误应失败且不能暴露内容")
	}
	if _, err := LoadConfig(t.TempDir()); err == nil {
		t.Fatal("读取目录应失败")
	}
}

// TestConfigValidation 验证缺失配置、非法连接参数与连接取消。
// @param t 测试上下文
// @return 无
func TestConfigValidation(t *testing.T) {
	isolateConfigEnv(t)
	// 分别覆盖缺失文件、必填字段、端口范围以及已取消的连接。
	if _, err := LoadConfig(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("缺少配置应失败")
	}
	for _, cfg := range []Config{{}, {Host: "localhost", Port: 3306, User: "u"}, {Host: "localhost", Port: 0, User: "u", Database: "d"}} {
		if _, err := Open(context.Background(), cfg); err == nil {
			t.Fatal("非法配置应失败")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if db, err := Open(ctx, Config{Host: "127.0.0.1", Port: 1, User: "u", Password: "private-secret", Database: "d"}); err == nil || db != nil || strings.Contains(err.Error(), "private-secret") {
		t.Fatal("连接失败处理错误")
	}
}
