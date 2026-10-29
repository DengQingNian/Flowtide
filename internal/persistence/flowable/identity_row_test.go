package flowable

import (
	"reflect"
	"strings"
	"testing"
)

// TestIdentityMembershipCompositeKey 验证成员行使用真实复合主键，不能因共享用户标识而覆盖其他组关系。
// @param t 测试上下文
// @return 无
func TestIdentityMembershipCompositeKey(t *testing.T) {
	rows := make([]Row, 0, 2)
	for _, pair := range [][2]string{{"alice", "review"}, {"alice", "ops"}} {
		row, err := NewRow("ACT_ID_MEMBERSHIP")
		if err != nil {
			t.Fatal(err)
		}
		if err := row.Set("USER_ID_", pair[0]); err != nil {
			t.Fatal(err)
		}
		if err := row.Set("GROUP_ID_", pair[1]); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, *row)
	}
	first, ok := rowIDValue(rows[0])
	if !ok {
		t.Fatal("复合标识缺失")
	}
	second, ok := rowIDValue(rows[1])
	if !ok || first == second {
		t.Fatal("同用户的组关系标识冲突")
	}
	for _, dialect := range []string{"mysql", "postgres"} {
		// 主键的两列都参与 WHERE，删除单个成员关系不能删除用户的全部组关系。
		query, args, err := rows[0].DeleteSQL(dialect)
		if err != nil || !strings.Contains(query, "USER_ID_ = ") || !strings.Contains(query, "AND GROUP_ID_ = ") || !reflect.DeepEqual(args, []any{"alice", "review"}) {
			t.Fatalf("复合删除SQL错误: %s %+v %v", query, args, err)
		}
		query, args, err = rows[0].InsertSQL(dialect)
		if err != nil || strings.Contains(query, "(ID_,") || !reflect.DeepEqual(args, []any{"alice", "review"}) {
			t.Fatalf("成员插入引入虚构主键: %s %+v %v", query, args, err)
		}
	}
	operations := DiffOperations(rows, rows[1:])
	if len(operations) != 1 || operations[0].Kind != OperationDelete || operations[0].Row.Values["GROUP_ID_"] != "review" {
		t.Fatalf("复合主键差量错误: %+v", operations)
	}
	collision := rows[0]
	collision.Values = map[string]any{"USER_ID_": "a|b", "GROUP_ID_": "c"}
	left, _ := rowIDValue(collision)
	collision.Values = map[string]any{"USER_ID_": "a", "GROUP_ID_": "b|c"}
	right, _ := rowIDValue(collision)
	if left == right {
		t.Fatal("复合标识不能使用可出现在用户输入中的简单分隔符")
	}
}
