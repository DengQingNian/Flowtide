// Package clock 提供可注入的时钟，便于测试定时器与历史时间。
package clock

import (
	"sync"
	"time"
)

// Clock 是时间源接口。
type Clock interface {
	// Now 返回当前时间。
	// @return 当前时间
	Now() time.Time
}

// Real 是真实时钟。
type Real struct{}

// Now 返回当前时间。
// @return 当前时间
func (Real) Now() time.Time { return time.Now() }

// Fake 是可控时钟，并发安全。
type Fake struct {
	mu  sync.RWMutex // 保护 now
	now time.Time    // 当前时间
}

// NewFake 创建假时钟。
// @param now 初始时间
// @return 假时钟
func NewFake(now time.Time) *Fake {
	return &Fake{now: now}
}

// Now 返回当前时间。
// @return 当前时间
func (f *Fake) Now() time.Time {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.now
}

// Advance 推进时间。
// @param d 推进时长
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

// Set 设置时间。
// @param t 新时间
func (f *Fake) Set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = t
}
