package flowable

import (
	"testing"
)

// TestHistoricCommentBinaryMessage 验证真实数据库 BLOB 和 bytea 评论完整内容可恢复。
// @param t 测试上下文
// @return 无
func TestHistoricCommentBinaryMessage(t *testing.T) {
	for _, value := range []any{[]byte("完整中文评论"), "完整中文评论", nil, int64(1), []byte{0xff}} {
		row := historicCommentRow(t, "comment")
		row.Values["FULL_MSG_"] = value
		comment, err := decodeHistoricComment(row)
		_, isNumber := value.(int64)
		bytesValue, isBytes := value.([]byte)
		invalid := isNumber || (isBytes && len(bytesValue) == 1 && bytesValue[0] == 0xff)
		if (err != nil) != invalid {
			t.Fatalf("完整评论类型 %T: %+v %v", value, comment, err)
		}
		if !invalid && value != nil && comment.FullMessage != "完整中文评论" {
			t.Fatalf("完整评论丢失: %+v", comment)
		}
	}
}
