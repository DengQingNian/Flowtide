package flowable

import "testing"

// TestJavaChildNullableStartTime 验证 Java 子执行的可空开始时间不会阻断标准行解码。
// @param t 测试上下文
// @return 无
func TestJavaChildNullableStartTime(t *testing.T) {
	row := runtimeExecution(t, "child")
	row.Values["START_TIME_"] = nil
	row.Values["PARENT_ID_"] = "i1"
	execution, err := decodeExecution(row)
	if err != nil || !execution.StartTime.IsZero() {
		t.Fatalf("Java 子执行的空开始时间应保留为空: %+v %v", execution, err)
	}
	row.Values["START_TIME_"] = "invalid"
	if _, err := decodeExecution(row); err == nil {
		t.Fatal("损坏的非空时间仍须拒绝")
	}
}
