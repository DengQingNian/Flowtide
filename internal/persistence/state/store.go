// Package state 定义引擎状态存储接口与文件/内存实现。
package state

import (
	"context"
	"os"
	"path/filepath"
	"sync"

	"github.com/DengQingNian/Flowtide/api"
)

// Store 是引擎状态存储。
type Store interface {
	// Load 读取状态字节；无状态时返回 nil, nil。
	// @param ctx 上下文
	// @return 状态字节
	// @return 读取错误
	Load(ctx context.Context) ([]byte, error)
	// Save 保存状态字节。
	// @param ctx 上下文
	// @param data 状态字节
	// @return 保存错误
	Save(ctx context.Context, data []byte) error
}

// MemoryStore 是内存状态存储，主要用于测试。
type MemoryStore struct {
	mu   sync.RWMutex // 保护 data
	data []byte       // 状态字节
	has  bool         // 是否存在状态
}

// NewMemory 创建内存状态存储。
// @return 内存存储
func NewMemory() *MemoryStore { return &MemoryStore{} }

// Load 读取状态。
// @param ctx 上下文
// @return 状态字节
// @return 读取错误
func (s *MemoryStore) Load(ctx context.Context) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.has {
		return nil, nil
	}
	return append([]byte(nil), s.data...), nil
}

// Save 保存状态。
// @param ctx 上下文
// @param data 状态字节
// @return 保存错误
func (s *MemoryStore) Save(ctx context.Context, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = append([]byte(nil), data...)
	s.has = true
	return nil
}

// createTempFile 创建临时文件，测试可替换。
var createTempFile = os.CreateTemp

// FileStore 是文件状态存储，使用原子替换。
type FileStore struct {
	Path string // 文件路径
}

// NewFile 创建文件状态存储。
// @param path 文件路径
// @return 文件存储
func NewFile(path string) *FileStore { return &FileStore{Path: path} }

// Load 读取状态文件；不存在时返回 nil, nil。
// @param ctx 上下文
// @return 状态字节
// @return 读取错误
func (s *FileStore) Load(ctx context.Context) ([]byte, error) {
	if s.Path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(s.Path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, api.NewError("ENGINE-0910", "读取状态文件失败", err)
	}
	return data, nil
}

// Save 原子保存状态文件。
// @param ctx 上下文
// @param data 状态字节
// @return 保存错误
func (s *FileStore) Save(ctx context.Context, data []byte) error {
	if s.Path == "" {
		return nil
	}
	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return api.NewError("ENGINE-0902", "创建状态目录失败", err)
	}
	temp, err := createTempFile(dir, ".flowtide-state-*")
	if err != nil {
		return api.NewError("ENGINE-0903", "创建临时状态文件失败", err)
	}
	name := temp.Name()
	defer func() {
		// 失败路径清理临时文件
		_ = os.Remove(name)
	}()
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return api.NewError("ENGINE-0904", "写入状态失败", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return api.NewError("ENGINE-0905", "同步状态失败", err)
	}
	if err := temp.Close(); err != nil {
		return api.NewError("ENGINE-0906", "关闭状态文件失败", err)
	}
	if err := os.Rename(name, s.Path); err != nil {
		return api.NewError("ENGINE-0907", "替换状态文件失败", err)
	}
	return nil
}
