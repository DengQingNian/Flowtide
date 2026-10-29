package flowable

import (
	"testing"
)

// TestAsyncJobPayloadReference 验证缺失或损坏的外置引用不能被恢复成空变量作业。
// @param t 测试上下文
// @return 无
func TestAsyncJobPayloadReference(t *testing.T) {
	row := runtimeJobRow(t, "async")
	row.Values["HANDLER_TYPE_"] = "variable-update"
	if _, err := decodeJob(row); err == nil {
		t.Fatal("缺失异步载荷引用未拒绝")
	}
	row.Values["CUSTOM_VALUES_ID_"] = int64(1)
	if _, err := decodeJob(row); err == nil {
		t.Fatal("无效异步载荷引用未拒绝")
	}
	row.Values["CUSTOM_VALUES_ID_"] = "payload"
	if _, err := decodeJob(row, map[string][]byte{}); err == nil {
		t.Fatal("外置载荷缺失未拒绝")
	}
	bytes := []byte(`{"version":1}`)
	job, err := decodeJob(row, map[string][]byte{"payload": bytes})
	if err != nil || string(job.VariablePayload) != string(bytes) {
		t.Fatalf("异步载荷恢复: %v", err)
	}
	job.VariablePayload[0] = 'x'
	if bytes[0] != '{' {
		t.Fatal("恢复作业共享输入字节")
	}
}
