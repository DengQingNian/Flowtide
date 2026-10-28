package flowable_test

import (
	"context"
	"testing"

	"github.com/flowable-go/flowable"
)

// TestNew 验证公共构造器的有效配置和错误传播。
// @param t 测试上下文
// @return 无
func TestNew(t *testing.T) {
	app, err := flowable.New(flowable.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := flowable.New(flowable.Config{HistoryLevel: "无效"}); err == nil {
		t.Fatal("构造错误必须透传")
	}
}
