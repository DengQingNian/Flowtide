package flowable

import (
	"reflect"
	"strings"
	"testing"
)

// auditRow 创建最小审计行。
// @param t 测试上下文
// @param table 表名
// @param values 列值
// @return 行
func auditRow(t *testing.T, table string, values map[string]any) Row {
	t.Helper()
	row, err := NewRow(table)
	if err != nil {
		t.Fatal(err)
	}
	columns := make([]string, 0)
	for _, column := range row.Columns() {
		if value, ok := values[column]; ok {
			if err := row.Set(column, value); err != nil {
				t.Fatal(err)
			}
			columns = append(columns, column)
		}
	}
	if err := row.SetInsertColumns(columns); err != nil {
		t.Fatal(err)
	}
	return *row
}

// TestAuditPostgresPlaceholders 验证任意主键位置和列子集的参数连续性。
// @param t 测试上下文
// @return 无
func TestAuditPostgresPlaceholders(t *testing.T) {
	row := auditRow(t, "ACT_RE_MODEL", map[string]any{"ID_": "m1", "REV_": 1, "NAME_": "修改"})
	query, args, err := row.UpdateSQL("postgres", 1)
	want := "UPDATE ACT_RE_MODEL SET REV_ = $1, NAME_ = $2 WHERE ID_ = $3 AND REV_ = $4"
	if err != nil || query != want || !reflect.DeepEqual(args, []any{int64(2), "修改", "m1", int64(1)}) {
		t.Fatalf("更新参数错误: %s %v %v", query, args, err)
	}
}

// TestAuditPropertyCRUD 验证 NAME_ 主键的增量插入、更新和带修订号删除。
// @param t 测试上下文
// @return 无
func TestAuditPropertyCRUD(t *testing.T) {
	old := auditRow(t, "ACT_GE_PROPERTY", map[string]any{"NAME_": "业务属性", "VALUE_": "旧值", "REV_": 1})
	changed := auditRow(t, "ACT_GE_PROPERTY", map[string]any{"NAME_": "业务属性", "VALUE_": "新值", "REV_": 1})
	for _, pair := range []struct {
		before, after []Row
		kind          string
	}{{nil, []Row{old}, OperationInsert}, {[]Row{old}, []Row{changed}, OperationUpdate}, {[]Row{old}, nil, OperationDelete}} {
		ops := DiffOperations(pair.before, pair.after)
		if len(ops) != 1 || ops[0].Kind != pair.kind {
			t.Fatalf("属性差量错误: %+v", ops)
		}
	}
	query, _, err := changed.UpdateSQL("postgres", 1)
	if err != nil || !strings.Contains(query, "WHERE NAME_ = $3 AND REV_ = $4") {
		t.Fatalf("属性更新主键错误: %s %v", query, err)
	}
	query, args, err := old.DeleteSQL("postgres")
	if err != nil || query != "DELETE FROM ACT_GE_PROPERTY WHERE NAME_ = $1 AND REV_ = $2" || len(args) != 2 {
		t.Fatalf("属性删除缺少版本约束: %s %v", query, err)
	}
}

// TestAuditForeignKeyOrder 验证乱序输入的插入和删除满足真实父子依赖。
// @param t 测试上下文
// @return 无
func TestAuditForeignKeyOrder(t *testing.T) {
	rows := []Row{
		auditRow(t, "ACT_RU_EXECUTION", map[string]any{"ID_": "a-child", "PARENT_ID_": "z-root", "PROC_INST_ID_": "z-root", "PROC_DEF_ID_": "pd"}),
		auditRow(t, "ACT_RE_MODEL", map[string]any{"ID_": "m", "EDITOR_SOURCE_VALUE_ID_": "b"}),
		auditRow(t, "ACT_GE_BYTEARRAY", map[string]any{"ID_": "b", "DEPLOYMENT_ID_": "d"}),
		auditRow(t, "ACT_RE_PROCDEF", map[string]any{"ID_": "pd", "DEPLOYMENT_ID_": "d"}),
		auditRow(t, "ACT_RU_EXECUTION", map[string]any{"ID_": "z-root", "PROC_INST_ID_": "z-root", "PROC_DEF_ID_": "pd"}),
		auditRow(t, "ACT_RE_DEPLOYMENT", map[string]any{"ID_": "d"}),
	}
	for _, ops := range [][]Operation{DiffOperations(nil, rows), DiffOperations(rows, nil), SnapshotOperations(rows)} {
		positions := map[string]int{}
		for index, op := range ops {
			id, _ := rowIDValue(op.Row)
			positions[op.Kind+id] = index
		}
		for _, edge := range [][2]string{{"d", "b"}, {"d", "pd"}, {"b", "m"}, {"pd", "z-root"}, {"z-root", "a-child"}} {
			if parent, ok := positions[OperationInsert+edge[0]]; ok && parent >= positions[OperationInsert+edge[1]] {
				t.Fatalf("父行未先插入: %v %+v", edge, ops)
			}
			if parent, ok := positions[OperationDelete+edge[0]]; ok && parent <= positions[OperationDelete+edge[1]] {
				t.Fatalf("子行未先删除: %v %+v", edge, ops)
			}
		}
	}
}

// TestAuditCommittedRevision 验证提交基线推进、不变行稳定以及系统属性保留。
// @param t 测试上下文
// @return 无
func TestAuditCommittedRevision(t *testing.T) {
	old := auditRow(t, "ACT_GE_PROPERTY", map[string]any{"NAME_": "business", "VALUE_": "a", "REV_": int64(9)})
	changed := auditRow(t, "ACT_GE_PROPERTY", map[string]any{"NAME_": "business", "VALUE_": "b", "REV_": 1})
	committed := CommittedRows([]Row{old}, []Row{changed})
	if len(committed) != 1 || committed[0].Values["REV_"] != int64(10) {
		t.Fatalf("提交基线未推进: %+v", committed)
	}
	if changed.Values["REV_"] != 1 || old.Values["REV_"] != int64(9) {
		t.Fatal("差量生成修改了原始行")
	}
	if ops := DiffOperations(committed, []Row{changed}); len(ops) != 0 {
		t.Fatalf("无业务变化不应推进版本: %+v", ops)
	}
	system := auditRow(t, "ACT_GE_PROPERTY", map[string]any{"NAME_": "flowable-go.schema.checksum", "VALUE_": "hash", "REV_": 1})
	preserved := PreserveSchemaRows([]Row{system, old}, []Row{changed, system})
	if len(preserved) != 2 || len(DiffOperations([]Row{system, old}, preserved)) != 1 {
		t.Fatalf("系统属性不应删除: %+v", preserved)
	}
}

// TestAuditTenantRelatedRows 验证模型源码、变量字节、身份链接及历史关联不跨租户。
// @param t 测试上下文
// @return 无
func TestAuditTenantRelatedRows(t *testing.T) {
	rows := []Row{
		{Table: "ACT_RE_DEPLOYMENT", Values: map[string]any{"ID_": "d", "TENANT_ID_": "a"}},
		{Table: "ACT_RE_MODEL", Values: map[string]any{"ID_": "m", "TENANT_ID_": "a", "EDITOR_SOURCE_VALUE_ID_": "source"}},
		{Table: "ACT_RU_EXECUTION", Values: map[string]any{"ID_": "i", "PROC_INST_ID_": "i", "TENANT_ID_": "a"}},
		{Table: "ACT_RU_TASK", Values: map[string]any{"ID_": "t", "TENANT_ID_": "a"}},
		{Table: "ACT_HI_TASKINST", Values: map[string]any{"ID_": "ht", "TENANT_ID_": "a"}},
		{Table: "ACT_RU_VARIABLE", Values: map[string]any{"ID_": "v", "EXECUTION_ID_": "i", "BYTEARRAY_ID_": "data"}},
		{Table: "ACT_RU_IDENTITYLINK", Values: map[string]any{"ID_": "l", "TASK_ID_": "t"}},
		{Table: "ACT_HI_IDENTITYLINK", Values: map[string]any{"ID_": "hl", "TASK_ID_": "ht"}},
		{Table: "ACT_GE_BYTEARRAY", Values: map[string]any{"ID_": "source"}},
		{Table: "ACT_GE_BYTEARRAY", Values: map[string]any{"ID_": "data"}},
		{Table: "ACT_GE_BYTEARRAY", Values: map[string]any{"ID_": "resource", "DEPLOYMENT_ID_": "d"}},
		{Table: "ACT_GE_BYTEARRAY", Values: map[string]any{"ID_": "control", "NAME_": "flowable-go-control/i"}},
		{Table: "ACT_GE_PROPERTY", Values: map[string]any{"NAME_": "global"}},
	}
	foreign := Row{Table: "ACT_RE_MODEL", Values: map[string]any{"ID_": "foreign", "TENANT_ID_": "b"}}
	rows = append(rows, foreign)
	if got := FilterRowsByTenant(rows, "a"); len(got) != len(rows)-1 {
		t.Fatalf("关联行过滤错误: %+v", got)
	}
	if got := FilterRowsByTenant(rows, ""); len(got) != 1 || got[0].Table != "ACT_GE_PROPERTY" {
		t.Fatalf("默认租户越权: %+v", got)
	}
}

// TestTransactionCapability 验证普通单条仓储不会被误认作命令事务仓储。
// @param t 测试上下文
// @return 无
func TestTransactionCapability(t *testing.T) {
	repository, _ := NewRepository(&fakeDB{}, "mysql")
	if repository.SupportsTransactions() {
		t.Fatal("无事务数据库不得标记支持事务")
	}
}
