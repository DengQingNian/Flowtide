package sqlstate

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestMySQLAtomicSave 验证保存使用单条原子语句且失败后保留旧状态。
// @param t 测试上下文
// @return 无
func TestMySQLAtomicSave(t *testing.T) {
	db := &fakeDB{stored: []byte("旧状态"), has: true, failInsert: true}
	store, err := New(db, "mysql")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), []byte("新状态")); err == nil {
		t.Fatal("写入失败应返回错误")
	}
	if !db.has || string(db.stored) != "旧状态" || len(db.execs) != 1 || !strings.Contains(db.execs[0], "ON DUPLICATE KEY UPDATE") {
		t.Fatal("失败写入破坏了旧状态或不是原子更新")
	}
	db.failInsert = false
	if err := store.Save(context.Background(), []byte("新状态")); err != nil {
		t.Fatal(err)
	}
	if string(db.stored) != "新状态" {
		t.Fatal(errors.New("新状态未保存"))
	}
}
