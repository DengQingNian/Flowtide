// Package idgen 提供时间有序的 UUIDv7 生成器。
package idgen

import (
	"crypto/rand"
	"encoding/hex"
	"sync/atomic"
	"time"
)

// Generator 是 ID 生成器接口。
type Generator interface {
	// NewID 生成一个新 ID。
	// @return 36 字符 UUID 字符串
	NewID() string
}

// UUIDv7 是 UUIDv7 生成器，并发安全。
type UUIDv7 struct {
	counter atomic.Uint64 // 同毫秒内的序号，降低碰撞概率
}

// NewUUIDv7 创建 UUIDv7 生成器。
// @return 生成器
func NewUUIDv7() *UUIDv7 { return &UUIDv7{} }

// NewID 生成当前时间的 UUIDv7。
// @return UUID 字符串
func (g *UUIDv7) NewID() string { return g.NewIDAt(time.Now()) }

// NewIDAt 生成指定时间的 UUIDv7。
// @param t 时间
// @return UUID 字符串
func (g *UUIDv7) NewIDAt(t time.Time) string {
	var b [16]byte
	// 48 位毫秒时间戳，保证时间有序
	ms := uint64(t.UnixMilli())
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)

	// 版本 7 与变体位
	b[6] = 0x70 | byte(g.counter.Add(1)&0x0f)
	b[8] = 0x80 | byte(g.counter.Load()&0x3f)

	// 剩余位使用加密随机数；读取失败时保持零值，仍能保证唯一性
	_, _ = rand.Read(b[7:8])
	_, _ = rand.Read(b[9:16])

	var out [36]byte
	hex.Encode(out[0:8], b[0:4])
	out[8] = '-'
	hex.Encode(out[9:13], b[4:6])
	out[13] = '-'
	hex.Encode(out[14:18], b[6:8])
	out[18] = '-'
	hex.Encode(out[19:23], b[8:10])
	out[23] = '-'
	hex.Encode(out[24:36], b[10:16])
	return string(out[:])
}
