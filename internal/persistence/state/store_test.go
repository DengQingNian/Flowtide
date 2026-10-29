package state

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestMemoryStore 验证内存存储。
// @param t 测试上下文
func TestMemoryStore(t *testing.T) {
	store := NewMemory()
	data, err := store.Load(context.Background())
	if err != nil || data != nil {
		t.Fatalf("空存储应返回 nil: %v %v", data, err)
	}
	original := []byte("hello")
	if err := store.Save(context.Background(), original); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	original[0] = 'X'
	loaded, err := store.Load(context.Background())
	if err != nil || string(loaded) != "hello" {
		t.Fatalf("读取或拷贝错误: %v %s", err, loaded)
	}
	loaded[0] = 'Y'
	again, _ := store.Load(context.Background())
	if string(again) != "hello" {
		t.Fatalf("读取应返回副本")
	}
}

// TestFileStore 验证文件存储。
// @param t 测试上下文
func TestFileStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "state.json")
	store := NewFile(path)
	data, err := store.Load(context.Background())
	if err != nil || data != nil {
		t.Fatalf("缺失文件应返回 nil: %v %v", data, err)
	}
	if err := store.Save(context.Background(), []byte("state")); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	loaded, err := store.Load(context.Background())
	if err != nil || string(loaded) != "state" {
		t.Fatalf("读取失败: %v %s", err, loaded)
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		t.Fatalf("状态文件错误: %v", err)
	}
}

// TestFileStoreErrors 验证文件存储错误。
// @param t 测试上下文
func TestFileStoreErrors(t *testing.T) {
	empty := NewFile("")
	if data, err := empty.Load(context.Background()); err != nil || data != nil {
		t.Fatalf("空路径读取应成功: %v", err)
	}
	if err := empty.Save(context.Background(), []byte("x")); err != nil {
		t.Fatalf("空路径保存应成功: %v", err)
	}
	directory := t.TempDir()
	if err := NewFile(directory).Save(context.Background(), []byte("x")); err == nil {
		t.Fatalf("目录作为文件应失败")
	}
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("创建文件失败: %v", err)
	}
	// Windows 将文件子路径的打开按不存在处理，无法表达 Unix 的 ENOTDIR 语义。
	if runtime.GOOS != "windows" {
		if _, err := NewFile(file + "/child").Load(context.Background()); err == nil {
			t.Fatalf("非法路径读取应失败")
		}
	}
	if err := NewFile(file+"/child").Save(context.Background(), []byte("x")); err == nil {
		t.Fatalf("父路径为文件时保存应失败")
	}
	if _, err := NewFile(directory).Load(context.Background()); err == nil {
		t.Fatalf("目录读取应失败")
	}
	// Unix 依赖只读的 /proc 目录触发临时文件创建失败；Windows 会把根路径解析为
	// 当前盘符并真实创建目录，父路径为文件的断言已覆盖同一错误分支。
	if runtime.GOOS != "windows" {
		if err := NewFile("/proc/flowable-go-state-test.json").Save(context.Background(), []byte("x")); err == nil {
			t.Fatalf("不可写目录保存应失败")
		}
	}
}

// TestFileStoreCreateTempErrors 验证临时文件错误。
// @param t 测试上下文
func TestFileStoreCreateTempErrors(t *testing.T) {
	original := createTempFile
	defer func() { createTempFile = original }()
	createTempFile = func(dir, pattern string) (*os.File, error) {
		return nil, errors.New("create")
	}
	if err := NewFile(filepath.Join(t.TempDir(), "state.json")).Save(context.Background(), []byte("x")); err == nil {
		t.Fatalf("创建临时文件失败应返回")
	}
	createTempFile = func(dir, pattern string) (*os.File, error) {
		file, err := os.CreateTemp(dir, pattern)
		if err != nil {
			return nil, err
		}
		_ = file.Close()
		return file, nil
	}
	if err := NewFile(filepath.Join(t.TempDir(), "state.json")).Save(context.Background(), []byte("x")); err == nil {
		t.Fatalf("写入失败应返回")
	}
}
