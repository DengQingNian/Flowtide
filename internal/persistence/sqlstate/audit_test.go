package sqlstate

import (
	"context"
	"strings"
	"testing"
)

// TestAuditPostgresAtomicSave 验证 PostgreSQL 单条原子保存及失败后旧快照可读。
// @param t 测试上下文
// @return 无
func TestAuditPostgresAtomicSave(t *testing.T) {
	db := &fakeDB{}
	store, _ := New(db, "postgres")
	if err := store.Save(context.Background(), []byte("旧快照")); err != nil {
		t.Fatal(err)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0], "ON CONFLICT (ID_) DO UPDATE") {
		t.Fatalf("快照必须原子写入: %v", db.execs)
	}
	db.failInsert = true
	if err := store.Save(context.Background(), []byte("新快照")); err == nil {
		t.Fatal("故障应返回错误")
	}
	data, err := store.Load(context.Background())
	if err != nil || string(data) != "旧快照" {
		t.Fatalf("失败写入丢失旧快照: %q %v", data, err)
	}
}
