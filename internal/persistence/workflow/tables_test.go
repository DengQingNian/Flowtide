package workflow

import (
	"strings"
	"testing"

	"github.com/DengQingNian/Flowtide/internal/persistence/schema"
)

// TestTableDefsMatchDDL 验证 Go 表元数据与双库 DDL 的列集合逐一一致，防止两边漂移。
// @param t 测试上下文
func TestTableDefsMatchDDL(t *testing.T) {
	defs := []tableDef{
		tableBusinessSystems, tableModels, tableSteps, tableHisSteps, tableTemplates,
		tableActExtends, tableCountersignWeights, tableNextActivities, tableHisNextActivities,
		tableMessageExtends, tableHistoryExtends, tableDelegateTasks, tableDocuments,
		tableOperationRecords, tableReviewTasks, tableTaskExceptions, tableCompleteToActivities,
		tableTaskDelegates, tableTaskConsigners, tableTaskConsignObjs, tableReminds, tableRemindTargets,
	}
	if len(defs) != 22 {
		t.Fatalf("表定义数量错误: %d", len(defs))
	}
	for _, dialect := range schema.Dialects() {
		for _, def := range defs {
			ddlColumns, _, err := schema.TableColumns(dialect, def.Name)
			if err != nil {
				t.Fatalf("%s 解析 %s 列失败: %v", dialect, def.Name, err)
			}
			if len(ddlColumns) != len(def.Columns) {
				t.Fatalf("%s %s 列数量不一致: DDL %d vs 元数据 %d", dialect, def.Name, len(ddlColumns), len(def.Columns))
			}
			for i, ddlColumn := range ddlColumns {
				// 保留字列在 DDL 中带转义符，解析时已去除，统一小写比较
				if !strings.EqualFold(ddlColumn, def.Columns[i]) {
					t.Fatalf("%s %s 第 %d 列不一致: DDL %s vs 元数据 %s", dialect, def.Name, i+1, ddlColumn, def.Columns[i])
				}
			}
		}
	}
}
