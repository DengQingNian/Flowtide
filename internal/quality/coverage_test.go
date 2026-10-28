package quality

import (
	"strings"
	"testing"
)

// TestCoverageGate 验证权重统计、逐包阈值、排除范围与错误输入。
// @param t 测试上下文
// @return 无
func TestCoverageGate(t *testing.T) {
	for _, test := range []struct {
		name, profile string
		excluded      []string
		minimum       float64
		failed        bool
	}{
		{"通过", "mode: set\nmodule/core/a.go:1.1,2.1 9 1\nmodule/core/b.go:1.1,2.1 1 0\n", nil, .9, false},
		{"总量不足", "mode: set\nmodule/core/a.go:1.1,2.1 8 1\nmodule/core/b.go:1.1,2.1 2 0\n", nil, .9, true},
		{"小包不足", "mode: set\nmodule/core/a.go:1.1,2.1 100 1\nmodule/other/b.go:1.1,2.1 1 0\n", nil, .9, true},
		{"明确排除", "mode: set\nmodule/core/a.go:1.1,2.1 10 1\nmodule/cmd/a.go:1.1,2.1 9 0\n", []string{"module/cmd/"}, .9, false},
		{"空文件", "", nil, .9, true},
		{"缺少模式", "data", nil, .9, true},
		{"记录错误", "mode: set\nwrong\n", nil, .9, true},
		{"语句数量错误", "mode: set\nm/a.go:1.1,2.1 bad 1\n", nil, .9, true},
		{"计数错误", "mode: set\nm/a.go:1.1,2.1 1 -1\n", nil, .9, true},
		{"路径错误", "mode: set\na.go:1.1,2.1 1 1\n", nil, .9, true},
		{"空数据", "mode: set\n", nil, .9, true},
		{"非法阈值", "mode: set\n", nil, 1.1, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := CheckCoverage(strings.NewReader(test.profile), test.excluded, test.minimum)
			if (err != nil) != test.failed {
				t.Fatalf("门禁结果错误: %v", err)
			}
		})
	}
	if (Coverage{}).Ratio() != 1 {
		t.Fatal("无语句覆盖率定义错误")
	}
}
