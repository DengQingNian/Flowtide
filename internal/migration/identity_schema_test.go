package migration

import (
	"testing"

	"github.com/DengQingNian/Flowtide/internal/persistence/schema"
)

// TestIdentitySchemaMetadata 验证官方身份版本标记可预检，但未知标记、缺失行和版本冲突仍阻断。
// @param t 测试上下文
// @return 无
func TestIdentitySchemaMetadata(t *testing.T) {
	for _, scenario := range []string{"标准标记", "版本冲突", "未知属性", "只计数无行"} {
		source := analysisFixture()
		source.Tables = append(source.Tables, TableSummary{Name: "ACT_ID_PROPERTY", Count: 1})
		if scenario != "只计数无行" {
			name, value := "schema.version", schema.Version
			if scenario == "版本冲突" {
				value = "7.0"
			}
			if scenario == "未知属性" {
				name = "custom"
			}
			source.Rows = append(source.Rows, analysisRow("ACT_ID_PROPERTY", "", map[string]any{"NAME_": name, "VALUE_": value, "REV_": int64(1)}))
		}
		if report := Analyze(source, Options{}); report.Blocked != (scenario != "标准标记") {
			t.Fatalf("%s: %+v", scenario, report.Issues)
		}
	}
}
