package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestNewStateStore 验证默认存储兼容性、互斥参数与非法配置。
// @param t 测试上下文
// @return 无
func TestNewStateStore(t *testing.T) {
	t.Setenv("db_mysql_user", "")
	// 默认路径保留原有行为，显式数据库路径严格拒绝冲突配置。
	path := filepath.Join(t.TempDir(), ".env")
	store, closeDB, err := newStateStore(context.Background(), "", "state.json", path)
	if err != nil || store != nil {
		t.Fatalf("默认配置错误: %v", err)
	}
	if err := closeDB(); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct{ storage, path string }{{"unknown", ""}, {"mysql", "state.json"}, {"mysql", ""}} {
		if _, _, err := newStateStore(context.Background(), scenario.storage, scenario.path, path); err == nil {
			t.Fatal("错误配置应失败")
		}
	}
	if err := os.WriteFile(path, []byte("db_mysql_host=127.0.0.1\ndb_mysql_port=1\ndb_mysql_user=test\ndb_mysql_db_name=test\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for key, value := range map[string]string{"db_mysql_host": "127.0.0.1", "db_mysql_port": "1", "db_mysql_user": "test", "db_mysql_pwd": "", "db_mysql_db_name": "test"} {
		t.Setenv(key, value)
	}
	if _, _, err := newStateStore(ctx, "mysql", "", path); err == nil {
		t.Fatal("连接取消应失败")
	}
}
