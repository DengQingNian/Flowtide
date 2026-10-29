package flowable

import (
	"testing"
	"time"
)

// TestSQLTimestampUTC 验证无时区 SQL 时间参数保持绝对时刻，不把本地墙上时间写成 UTC。
// @param t 测试上下文
// @return 无
func TestSQLTimestampUTC(t *testing.T) {
	for _, offset := range []int{8 * 3600, -5 * 3600} {
		original := time.Date(2026, 10, 29, 12, 0, 0, 123000000, time.FixedZone("验收时区", offset))
		row, _ := NewRow("ACT_RU_TASK")
		_ = row.SetInsertColumns([]string{"ID_", "REV_", "CREATE_TIME_"})
		_ = row.Set("ID_", "task")
		_ = row.Set("REV_", 1)
		_ = row.Set("CREATE_TIME_", original)
		for _, dialect := range []string{"mysql", "postgres"} {
			_, insert, err := row.InsertSQL(dialect)
			if err != nil {
				t.Fatal(err)
			}
			_, update, err := row.UpdateSQL(dialect, 1)
			if err != nil {
				t.Fatal(err)
			}
			for _, value := range []any{insert[2], update[1]} {
				actual, ok := value.(time.Time)
				if !ok || actual.Location() != time.UTC || !actual.Equal(original) {
					t.Fatalf("时间参数未保留UTC时刻: %s %+v", dialect, value)
				}
			}
		}
		if row.Values["CREATE_TIME_"].(time.Time).Location() != original.Location() {
			t.Fatal("SQL编码修改调用方时间")
		}
	}
}

// TestSQLNullableTimestampUTC 验证可空日期也经过统一编码，避免历史到期时间沿指针路径发生时区漂移。
// @param t 测试上下文
// @return 无
func TestSQLNullableTimestampUTC(t *testing.T) {
	row, _ := NewRow("ACT_RU_TASK")
	_ = row.SetInsertColumns([]string{"DUE_DATE_"})
	original := time.Date(2026, 10, 29, 12, 0, 0, 0, time.FixedZone("验收时区", 8*3600))
	_ = row.Set("DUE_DATE_", &original)
	value, ok := row.Args()[0].(time.Time)
	if !ok || value.Location() != time.UTC || !value.Equal(original) {
		t.Fatalf("可空日期没有转为UTC: %+v", row.Args()[0])
	}
	var empty *time.Time
	_ = row.Set("DUE_DATE_", empty)
	if row.Args()[0] != nil {
		t.Fatal("空日期指针未转为SQL空值")
	}
}
