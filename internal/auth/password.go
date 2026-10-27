// Package auth 提供密码哈希与校验。
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/flowable-go/flowable/api"
)

const iterations = 120000

// randomRead 生成随机字节，测试可替换。
var randomRead = rand.Read

// Hash 生成密码哈希。
// @param password 明文密码
// @return 哈希
// @return 错误
func Hash(password string) (string, error) {
	if password == "" {
		return "", api.NewError("AUTH-0001", "密码不能为空", api.ErrBadRequest)
	}
	salt := make([]byte, 16)
	if _, err := randomRead(salt); err != nil {
		return "", api.NewError("AUTH-0002", "生成盐失败", err)
	}
	hash := derive(password, salt, iterations)
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", iterations, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}

// Verify 校验密码。
// @param password 明文密码
// @param encoded 哈希
// @return 是否匹配
func Verify(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	count, err := strconv.Atoi(parts[1])
	if err != nil || count <= 0 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	actual := derive(password, salt, count)
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

// derive 执行迭代哈希。
// @param password 密码
// @param salt 盐
// @param count 迭代次数
// @return 哈希
func derive(password string, salt []byte, count int) []byte {
	sum := sha256.Sum256(append([]byte(password), salt...))
	hash := sum[:]
	for i := 1; i < count; i++ {
		next := sha256.Sum256(append(hash, salt...))
		hash = next[:]
	}
	return hash
}
