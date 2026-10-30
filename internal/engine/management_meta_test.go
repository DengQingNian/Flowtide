package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
)

// TestManagementProperties 验证引擎属性。
// @param t 测试上下文
func TestManagementProperties(t *testing.T) {
	engine := newTestEngine(t)
	properties, err := engine.Properties(context.Background())
	if err != nil {
		t.Fatalf("查询属性失败: %v", err)
	}
	if properties["schema.version"] != "8.1.0.3" {
		t.Fatalf("Schema 版本错误: %s", properties["schema.version"])
	}
	if properties["historyLevel"] != HistoryAudit {
		t.Fatalf("历史级别错误: %s", properties["historyLevel"])
	}
	if properties["engine.version"] != Version {
		t.Fatalf("引擎版本错误: %s", properties["engine.version"])
	}
	_ = engine.Close(context.Background())
	if _, err := engine.Properties(context.Background()); err == nil {
		t.Fatalf("关闭后查询属性应失败")
	}
}

// TestManagementTableCount 验证表行数统计。
// @param t 测试上下文
func TestManagementTableCount(t *testing.T) {
	engine := newTestEngine(t)
	counts, err := engine.TableCount(context.Background())
	if err != nil {
		t.Fatalf("统计表行数失败: %v", err)
	}
	if counts["ACT_RE_DEPLOYMENT"] != 0 {
		t.Fatalf("初始部署表应为空")
	}
	deployXML(t, engine, simpleProcess)
	counts, err = engine.TableCount(context.Background())
	if err != nil {
		t.Fatalf("统计表行数失败: %v", err)
	}
	if counts["ACT_RE_DEPLOYMENT"] != 1 {
		t.Fatalf("部署表行数错误: %d", counts["ACT_RE_DEPLOYMENT"])
	}
	if counts["ACT_RE_PROCDEF"] != 1 {
		t.Fatalf("流程定义表行数错误: %d", counts["ACT_RE_PROCDEF"])
	}
	if _, ok := counts["ACT_HI_PROCINST"]; !ok {
		t.Fatalf("应包含全部 Schema 表")
	}
}

// TestManagementTableMetaData 验证表列元数据。
// @param t 测试上下文
func TestManagementTableMetaData(t *testing.T) {
	engine := newTestEngine(t)
	meta, err := engine.GetTableMetaData(context.Background(), "act_ru_task")
	if err != nil {
		t.Fatalf("查询列元数据失败: %v", err)
	}
	if meta.TableName != "ACT_RU_TASK" {
		t.Fatalf("表名未规范化: %s", meta.TableName)
	}
	if !contains(meta.ColumnNames, "ASSIGNEE_") || len(meta.ColumnTypes) != len(meta.ColumnNames) {
		t.Fatalf("列元数据错误: %v %v", meta.ColumnNames, meta.ColumnTypes)
	}
	if _, err := engine.GetTableMetaData(context.Background(), "ACT_UNKNOWN"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("未知表应返回未找到: %v", err)
	}
}

// TestManagementTableData 验证表数据分页与排序。
// @param t 测试上下文
func TestManagementTableData(t *testing.T) {
	engine := newTestEngine(t)
	page, err := engine.QueryTableData(context.Background(), "ACT_RE_DEPLOYMENT", 0, 10, "", "")
	if err != nil {
		t.Fatalf("查询表数据失败: %v", err)
	}
	if page.Total != 0 || len(page.Data) != 0 {
		t.Fatalf("初始表数据应为空: %+v", page)
	}
	deployXML(t, engine, simpleProcess)
	page, err = engine.QueryTableData(context.Background(), "ACT_RE_PROCDEF", 0, 10, "key_", "desc")
	if err != nil {
		t.Fatalf("查询表数据失败: %v", err)
	}
	if page.Total != 1 || len(page.Data) != 1 {
		t.Fatalf("表数据数量错误: %+v", page)
	}
	if page.Sort != "KEY_" || page.Order != "desc" {
		t.Fatalf("排序元数据错误: %+v", page)
	}
	if page.Data[0]["KEY_"] != "simple" {
		t.Fatalf("行数据错误: %+v", page.Data[0])
	}
	if _, ok := page.Data[0]["TENANT_ID_"]; !ok {
		t.Fatalf("未写入列应补 null: %+v", page.Data[0])
	}
}

// TestManagementTableDataBounds 验证分页边界与非法参数。
// @param t 测试上下文
func TestManagementTableDataBounds(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	page, err := engine.QueryTableData(context.Background(), "ACT_RE_PROCDEF", 5, 10, "", "")
	if err != nil {
		t.Fatalf("查询表数据失败: %v", err)
	}
	if page.Total != 1 || len(page.Data) != 0 {
		t.Fatalf("越界分页应返回空数据: %+v", page)
	}
	if _, err := engine.QueryTableData(context.Background(), "ACT_RE_PROCDEF", 0, 10, "MISSING_", ""); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("非法排序列应返回 400: %v", err)
	}
	if _, err := engine.QueryTableData(context.Background(), "ACT_RE_PROCDEF", 0, 10, "ID_", "sideways"); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("非法排序方向应返回 400: %v", err)
	}
	if _, err := engine.QueryTableData(context.Background(), "ACT_UNKNOWN", 0, 10, "", ""); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("未知表应返回未找到: %v", err)
	}
	_ = engine.Close(context.Background())
	if _, err := engine.TableCount(context.Background()); err == nil {
		t.Fatalf("关闭后统计应失败")
	}
}

// TestManagementTableDataClosed 验证关闭引擎后的错误。
// @param t 测试上下文
func TestManagementTableDataClosed(t *testing.T) {
	engine := newTestEngine(t)
	_ = engine.Close(context.Background())
	if _, err := engine.QueryTableData(context.Background(), "ACT_RE_PROCDEF", 0, 10, "", ""); err == nil {
		t.Fatalf("关闭后查询表数据应失败")
	}
	if _, err := engine.TableCount(context.Background()); err == nil {
		t.Fatalf("关闭后统计应失败")
	}
}

// TestInstanceCommentsAndLinks 验证实例评论与身份链接查询。
// @param t 测试上下文
func TestInstanceCommentsAndLinks(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		UserID:               "alice",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	tasks := listTasks(t, engine)
	if len(tasks) == 0 {
		t.Fatalf("任务为空")
	}
	if _, err := engine.AddProcessComment(context.Background(), instance.ID, "alice", "proc"); err != nil {
		t.Fatalf("添加流程评论失败: %v", err)
	}
	if _, err := engine.AddComment(context.Background(), tasks[0].ID, "alice", "task"); err != nil {
		t.Fatalf("添加任务评论失败: %v", err)
	}
	comments, err := engine.ListInstanceComments(context.Background(), instance.ID)
	if err != nil || len(comments) != 2 {
		t.Fatalf("实例评论错误: %v %+v", err, comments)
	}
	links, err := engine.ListHistoricProcessIdentityLinks(context.Background(), instance.ID)
	if err != nil || len(links) == 0 {
		t.Fatalf("实例身份链接错误: %v %+v", err, links)
	}
	for _, link := range links {
		if link.ProcessInstanceID != instance.ID {
			t.Fatalf("身份链接实例不匹配: %+v", link)
		}
	}
}

// TestCompareValues 验证排序值比较。
// @param t 测试上下文
func TestCompareValues(t *testing.T) {
	now := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		left  any
		right any
		want  int
	}{
		{nil, nil, 0},
		{nil, 1, -1},
		{1, nil, 1},
		{1, 2, -1},
		{int64(4), int64(3), 1},
		{3.5, 3.5, 0},
		{true, false, 1},
		{false, true, -1},
		{"a", "b", -1},
		{[]byte("b"), []byte("a"), 1},
		{now, now.Add(time.Second), -1},
		{time.Time{}, now, -1},
	}
	for _, tc := range cases {
		if got := compareValues(tc.left, tc.right); sign(got) != tc.want {
			t.Fatalf("比较 %v/%v 期望 %d 实际 %d", tc.left, tc.right, tc.want, got)
		}
	}
	if numericValue("x") != 0 || stringValue(1) != "" {
		t.Fatalf("默认转换错误")
	}
	if stringValue([]byte("x")) != "x" || stringValue(now) == "" || stringValue("y") != "y" {
		t.Fatalf("字符串转换错误")
	}
	if got := compareValues(map[string]any{}, "x"); got == 0 {
		t.Fatalf("非字符串值应可比较")
	}
	if value := sortValue(map[string]any{"ID_": "a", "X_": "b"}, ""); value != "a" {
		t.Fatalf("默认排序键错误: %v", value)
	}
	if value := sortValue(map[string]any{"X_": "b"}, "X_"); value != "b" {
		t.Fatalf("指定排序键错误: %v", value)
	}
	if value := sortValue(map[string]any{}, ""); value != "" {
		t.Fatalf("空行排序键错误: %v", value)
	}
}

// sign 返回整数符号。
// @param value 整数
// @return -1/0/1
func sign(value int) int {
	if value < 0 {
		return -1
	}
	if value > 0 {
		return 1
	}
	return 0
}
