package workflow

import (
	"context"
	"testing"
	"time"

	pwf "github.com/DengQingNian/Flowtide/internal/persistence/workflow"
)

// TestRowValueHelpers 验证通用行取值工具的类型归一化。
// @param t 测试上下文
func TestRowValueHelpers(t *testing.T) {
	stamped := time.Date(2026, 1, 1, 8, 0, 0, 0, time.Local)
	row := pwf.Row{
		"s":     "text",
		"bytes": []byte("字节"),
		"i64":   int64(3),
		"i":     4,
		"f64":   1.5,
		"dec":   "12.34",
		"num":   []byte("56.78"),
		"time":  stamped,
		"timeText": "2026-01-01 08:00:00",
		"nil":   nil,
		"bad":   struct{}{},
	}
	// 字符串：string/[]byte/其他
	if got := rowString(row, "s"); got != "text" {
		t.Fatalf("字符串读取错误: %q", got)
	}
	if got := rowString(row, "bytes"); got != "字节" {
		t.Fatalf("字节读取错误: %q", got)
	}
	if got := rowString(row, "missing"); got != "" {
		t.Fatalf("缺失列应为空: %q", got)
	}
	if got := rowString(row, "nil"); got != "" {
		t.Fatalf("NULL 应为空: %q", got)
	}
	// 整型：int64/int/float64/字符串
	if got := rowInt(row, "i64"); got != 3 {
		t.Fatalf("int64 读取错误: %d", got)
	}
	if got := rowInt(row, "i"); got != 4 {
		t.Fatalf("int 读取错误: %d", got)
	}
	if got := rowInt(row, "f64"); got != 1 {
		t.Fatalf("float64 读取错误: %d", got)
	}
	if got := rowInt(row, "bad"); got != 0 {
		t.Fatalf("不可解析应为 0: %d", got)
	}
	if rowInt(row, "missing") != 0 || rowInt(row, "nil") != 0 {
		t.Fatal("缺失/NULL 整型应为 0")
	}
	// 可空指针
	if ptr := rowIntPtr(row, "i64"); ptr == nil || *ptr != 3 {
		t.Fatalf("可空整型读取错误: %v", ptr)
	}
	if rowIntPtr(row, "missing") != nil {
		t.Fatal("缺失列可空整型应为 nil")
	}
	if ptr := rowI64Ptr(row, "i"); ptr == nil || *ptr != 4 {
		t.Fatalf("可空 int64 读取错误: %v", ptr)
	}
	if rowI64Ptr(row, "missing") != nil {
		t.Fatal("缺失列可空 int64 应为 nil")
	}
	// 浮点：float64/字符串/字节
	if ptr := rowFloatPtr(row, "f64"); ptr == nil || *ptr != 1.5 {
		t.Fatalf("浮点读取错误: %v", ptr)
	}
	if ptr := rowFloatPtr(row, "dec"); ptr == nil || *ptr != 12.34 {
		t.Fatalf("DECIMAL 字符串读取错误: %v", ptr)
	}
	if ptr := rowFloatPtr(row, "num"); ptr == nil || *ptr != 56.78 {
		t.Fatalf("DECIMAL 字节读取错误: %v", ptr)
	}
	if rowFloatPtr(row, "bad") != nil || rowFloatPtr(row, "missing") != nil {
		t.Fatal("不可解析浮点应为 nil")
	}
	// 时间：time.Time/文本/缺失（文本无时区按 UTC 解析）
	if ptr := rowTimePtr(row, "time"); ptr == nil || !ptr.Equal(stamped) {
		t.Fatalf("时间读取错误: %v", ptr)
	}
	textParsed := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	if ptr := rowTimePtr(row, "timeText"); ptr == nil || !ptr.Equal(textParsed) {
		t.Fatalf("时间文本解析错误: %v", ptr)
	}
	if rowTimePtr(row, "missing") != nil || rowTimePtr(row, "bad") != nil {
		t.Fatal("缺失/不可解析时间应为 nil")
	}
	if !timeOrZero(row, "time").Equal(stamped) || !timeOrZero(row, "missing").IsZero() {
		t.Fatal("timeOrZero 语义错误")
	}
}

// TestBaseRowHelpers 验证公共列/扩展列与行的双向转换。
// @param t 测试上下文
func TestBaseRowHelpers(t *testing.T) {
	stamped := time.Date(2026, 1, 1, 8, 0, 0, 0, time.Local)
	base := BaseColumns{ID: "id-1", Creator: "op", CreatorName: "操作人", CreateTime: stamped, Updater: "op", UpdateTime: stamped, TenantID: "sys"}
	row := pwf.Row{}
	appendBaseRow(row, base)
	// 回读一致
	if got := baseFromRow(row); got.ID != "id-1" || got.TenantID != "sys" || !got.CreateTime.Equal(stamped) {
		t.Fatalf("公共列往返错误: %+v", got)
	}
	// 零值时间省略（交由仓储填充）
	emptyRow := pwf.Row{}
	appendBaseRow(emptyRow, BaseColumns{ID: "id-2"})
	if _, ok := emptyRow["create_time"]; ok {
		t.Fatal("零值 create_time 应省略")
	}
	// 扩展列写入与回读
	ext := ExtendFields{
		ExtendS1: "s1", ExtendI1: int64Ptr(9), ExtendF1: floatPtr(2.5), ExtendD1: &stamped,
	}
	extRow := pwf.Row{}
	appendExtendRow(extRow, ext)
	if got := extendFromRow(extRow); got.ExtendS1 != "s1" || *got.ExtendI1 != 9 || *got.ExtendF1 != 2.5 || !got.ExtendD1.Equal(stamped) {
		t.Fatalf("扩展列往返错误: %+v", got)
	}
	// nil 值省略
	if _, ok := extRow["extend_s2"]; ok {
		t.Fatal("nil 扩展列应省略")
	}
}

// floatPtr 构造浮点指针。
// @param v 值
// @return 指针
func floatPtr(v float64) *float64 { return &v }

// TestEntityRowRoundtrip 验证五个实体的行往返一致性。
// @param t 测试上下文
func TestEntityRowRoundtrip(t *testing.T) {
	stamped := time.Date(2026, 1, 1, 8, 0, 0, 0, time.Local)
	// 下一步实体
	next := &NextActivity{
		BaseColumns:       BaseColumns{ID: "n1", Creator: "op", CreateTime: stamped, UpdateTime: stamped, TenantID: "sys"},
		NextActivityID:    "act-b",
		GroupID:           "g1",
		IdentityID:        "u1",
		SendIdentityID:    "sender",
		SendUserName:      "发送人",
		ProcessInstanceID: "pi-1",
		AgreeWeight:       floatPtr(3.5),
		RejectWeight:      floatPtr(1.5),
		Used:              1,
		PreTaskID:         "t0",
		TaskID:            "t1",
		Version:           2,
	}
	if got := nextFromRow(nextToRow(next)); got.IdentityID != "u1" || *got.AgreeWeight != 3.5 || got.Version != 2 || got.SendUserName != "发送人" {
		t.Fatalf("下一步往返错误: %+v", got)
	}
	// 待办扩展实体
	message := &MessageExtend{
		BaseColumns: BaseColumns{ID: "m1", Creator: "op", CreateTime: stamped, UpdateTime: stamped, TenantID: "sys"},
		TaskID:      "t1", ProcessInstanceID: "pi-1", Title: "标题", Message: "内容", BusinessID: "biz",
		TargetURL: "/url", TaskStepName: "审批", TaskStepCode: "step", FlowCategoryCode: "leave", FlowCategoryName: "请假",
		OwnerID: "u1", OwnerName: "张三", OwnerGroup: "g", OwnerGroupID: "g1", OwnerGroupPaths: "p", OwnerGroupPathIDs: "g1",
		DocumentOwnerID: "u0", DocumentOwnerName: "发起", DocumentGroupID: "dg", DocumentGroup: "dg", DocumentGroupPaths: "dp", DocumentGroupPathIDs: "dg",
		SenderID: "sender", Sender: "发送人", SenderStepName: "上步", SenderStepCode: "up",
		ConsignerID: "c1", Consigner: "受托", IsRead: 1, ReadTime: &stamped, PlanFinishTime: &stamped,
		TaskReceiptTime: &stamped, TaskStartTime: &stamped, IsDelay: 1, DelayTime: &stamped, MessageGroupID: "mg",
		ExtendFields: ExtendFields{ExtendS1: "x", ExtendI1: int64Ptr(1), ExtendF1: floatPtr(1.5), ExtendD1: &stamped},
	}
	if got := messageFromRow(messageToRow(message)); got.Title != "标题" || got.IsRead != 1 || got.ReadTime == nil || got.ExtendS1 != "x" || got.DocumentGroupPathIDs != "dg" {
		t.Fatalf("待办扩展往返错误: %+v", got)
	}
	// 已办扩展实体
	history := &HistoryExtend{MessageExtend: *message, TaskFinishTime: &stamped}
	if got := historyFromRow(historyToRow(history)); got.TaskFinishTime == nil || got.TaskID != "t1" {
		t.Fatalf("已办扩展往返错误: %+v", got)
	}
	// 操作记录实体
	record := &OperationRecord{
		BaseColumns: BaseColumns{ID: "o1", Creator: "op", CreateTime: stamped, UpdateTime: stamped, TenantID: "sys"},
		BusinessID:  "biz", TaskID: "t1", ProcessInstanceID: "pi-1", Message: "同意", IsAgree: intPtr(1),
		TaskStepCode: "step", TaskStepName: "审批", FlowCategoryCode: "leave", FlowCategoryName: "请假",
		OwnerID: "u1", OwnerName: "张三", OwnerGroup: "g", OwnerGroupID: "g1", OwnerGroupPaths: "p", OwnerGroupPathIDs: "g1",
		DealTime: &stamped, TaskReceiptTime: &stamped, TaskStartTime: &stamped, TaskFinishTime: &stamped, Type: CommentTypeSP,
	}
	if got := operationFromRow(operationToRow(record)); got.IsAgree == nil || *got.IsAgree != 1 || got.Type != CommentTypeSP || got.DealTime == nil {
		t.Fatalf("操作记录往返错误: %+v", got)
	}
	// 单据状态实体
	document := &Document{
		BaseColumns: BaseColumns{ID: "d1", Creator: "op", CreateTime: stamped, UpdateTime: stamped, TenantID: "sys"},
		BusinessID:  "biz", CreateGroupPathIDs: "cp", CreateGroupPaths: "cpaths", CreateGroupID: "cg", CreateGroup: "cg",
		UpdaterName: "更新人", Deleted: 0, DocumentURL: "/url", DocumentID: "biz", DocumentTable: "t_leave",
		FlowTitle: "标题", FlowTaskID: "t1", FlowInstanceID: "pi-1", FlowCategoryID: "leave", FlowCategory: "请假",
		FlowActionID: "step", FlowAction: "审批", FlowStatus: FlowStatusRunning, FlowCurrentStepName: "审批", FlowCurrentPersons: "张三",
		FlowStartTime: &stamped, FlowFinishTime: &stamped, FlowTerminationTime: &stamped,
		ExtendFields: ExtendFields{ExtendS2: "y"},
	}
	if got := documentFromRow(documentToRow(document)); got.FlowStatus != FlowStatusRunning || got.FlowStartTime == nil || got.FlowTerminationTime == nil || got.ExtendS2 != "y" || got.DocumentTable != "t_leave" {
		t.Fatalf("单据往返错误: %+v", got)
	}
	// 批量转换
	if rows := nextsFromRows([]pwf.Row{nextToRow(next)}); len(rows) != 1 {
		t.Fatal("nextsFromRows 错误")
	}
	if rows := messagesFromRows([]pwf.Row{messageToRow(message)}); len(rows) != 1 {
		t.Fatal("messagesFromRows 错误")
	}
	if rows := historiesFromRows([]pwf.Row{historyToRow(history)}); len(rows) != 1 {
		t.Fatal("historiesFromRows 错误")
	}
	if rows := operationsFromRows([]pwf.Row{operationToRow(record)}); len(rows) != 1 {
		t.Fatal("operationsFromRows 错误")
	}
	sortNextRowsByCreateTime(nil)
}

// TestSQLAdapterSkeleton 验证适配器骨架：方言占位符与裸 SQL 缺连接池报错。
// @param t 测试上下文
func TestSQLAdapterSkeleton(t *testing.T) {
	// 无 DB 构造（编译期装配路径）
	set := NewSQLStores(nil)
	if set.Next == nil || set.Message == nil || set.History == nil || set.Operation == nil || set.Document == nil {
		t.Fatal("适配器聚合应完整")
	}
	// 裸 SQL 缺连接池报错
	if _, err := set.Next.ListUsedByInstance(context.Background(), "pi-1"); err == nil {
		t.Fatal("缺连接池应报错")
	}
	if _, err := set.Next.ListHisByTaskID(context.Background(), "task-1"); err == nil {
		t.Fatal("缺连接池应报错")
	}
	if _, _, err := set.History.ListByOwner(context.Background(), "u1", "sys", PageQuery{}); err == nil {
		t.Fatal("缺连接池应报错")
	}
	// 方言占位符
	if got := dialectPlaceholder(pwf.DialectMySQL, 1); got != "?" {
		t.Fatalf("MySQL 占位符错误: %q", got)
	}
	if got := dialectPlaceholder(pwf.DialectPostgres, 2); got != "$2" {
		t.Fatalf("PostgreSQL 占位符错误: %q", got)
	}
}
