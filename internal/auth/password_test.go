package auth

import (
	"errors"
	"testing"
)

// TestHashVerify 验证哈希与校验。
// @param t 测试上下文
func TestHashVerify(t *testing.T) {
	encoded, err := Hash("secret")
	if err != nil || encoded == "" {
		t.Fatalf("哈希失败: %v", err)
	}
	if !Verify("secret", encoded) {
		t.Fatalf("正确密码应通过")
	}
	if Verify("wrong", encoded) {
		t.Fatalf("错误密码应失败")
	}
	if Verify("secret", "bad") {
		t.Fatalf("非法哈希应失败")
	}
	second, _ := Hash("secret")
	if encoded == second {
		t.Fatalf("盐应随机")
	}
	if _, err := Hash(""); err == nil {
		t.Fatalf("空密码应失败")
	}
	for _, invalid := range []string{
		"a$b$c", "md5$1$c2FsdA$aGFzaA", "pbkdf2-sha256$x$c2FsdA$aGFzaA",
		"pbkdf2-sha256$1$bad$aGFzaA", "pbkdf2-sha256$1$c2FsdA$bad",
	} {
		if Verify("secret", invalid) {
			t.Fatalf("非法哈希应失败: %s", invalid)
		}
	}
}

// TestHashRandomError 验证随机源错误。
// @param t 测试上下文
func TestHashRandomError(t *testing.T) {
	original := randomRead
	defer func() { randomRead = original }()
	randomRead = func(data []byte) (int, error) { return 0, errors.New("rand") }
	if _, err := Hash("secret"); err == nil {
		t.Fatalf("随机源错误应返回")
	}
}
