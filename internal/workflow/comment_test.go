package workflow

import (
	"context"
	"strings"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
)

// TestCommentAdd 验证意见添加：任务级/流程级路由、空白折叠、超长截断、缺参校验。
// @param t 测试上下文
func TestCommentAdd(t *testing.T) {
	tasks := &fakeCommentTasks{}
	service := NewCommentService(tasks, &fakeTaskHistory{}, &fakeUsers{})
	ctx := context.Background()
	// 任务级意见走 AddComment
	comment, err := service.Add(ctx, CommentRequest{TaskID: "task-1", ProcessInstanceID: "pi-1", UserID: "u1", Message: " 同意  本单。 ", Type: CommentTypeSP})
	if err != nil {
		t.Fatalf("添加任务级意见失败: %v", err)
	}
	if len(tasks.addComments) != 1 || tasks.addComments[0].TaskID != "task-1" {
		t.Fatalf("任务级意见未走 AddComment: %+v", tasks.addComments)
	}
	// 空白折叠为单空格
	if comment.Message != " 同意 本单。 " {
		t.Fatalf("空白折叠语义错误: %q", comment.Message)
	}
	// 流程级意见走 AddProcessComment
	if _, err := service.Add(ctx, CommentRequest{ProcessInstanceID: "pi-1", UserID: "u1", Message: "流程级意见"}); err != nil {
		t.Fatalf("添加流程级意见失败: %v", err)
	}
	if len(tasks.addProcess) != 1 || tasks.addProcess[0].TaskID != "" {
		t.Fatalf("流程级意见未走 AddProcessComment: %+v", tasks.addProcess)
	}
	// 超长截断：3901 个字符 → 3900 + "..."
	long := strings.Repeat("好", commentMaxRunes+1)
	truncated, err := service.Add(ctx, CommentRequest{TaskID: "task-1", ProcessInstanceID: "pi-1", UserID: "u1", Message: long})
	if err != nil {
		t.Fatal(err)
	}
	runes := []rune(truncated.Message)
	// 截断后 = 3900 字符 + 省略号
	if len(runes) != commentMaxRunes+3 || !strings.HasSuffix(truncated.Message, "...") {
		t.Fatalf("截断语义错误: 长度 %d", len(runes))
	}
	if !strings.HasPrefix(truncated.Message, strings.Repeat("好", commentMaxRunes)) {
		t.Fatal("截断应保留前 3900 字符")
	}
	// 缺操作人/实例报错
	if _, err := service.Add(ctx, CommentRequest{TaskID: "task-1", Message: "x"}); err == nil {
		t.Fatal("缺操作人应报错")
	}
	// 空白意见报错
	if _, err := service.Add(ctx, CommentRequest{ProcessInstanceID: "pi-1", UserID: "u1", Message: "   "}); err == nil {
		t.Fatal("空白意见应报错")
	}
}

// TestCommentListByProcessInstance 验证意见查询的展示字段补全与查不到给空。
// @param t 测试上下文
func TestCommentListByProcessInstance(t *testing.T) {
	tasks := &fakeCommentTasks{
		comments: []api.Comment{
			{ID: "c1", TaskID: "task-1", Type: CommentTypeSP, UserID: "u1", Message: "同意"},
			{ID: "c2", TaskID: "", Type: "comment", UserID: "u-ghost", Message: "流程级"},
		},
	}
	history := &fakeTaskHistory{tasks: map[string]api.HistoricTaskInstance{
		"task-1": {ID: "task-1", Name: "审批"},
	}, miss: map[string]bool{"task-ghost": true}}
	users := &fakeUsers{users: map[string]api.User{
		"u1": {ID: "u1", DisplayName: "张三"},
	}}
	service := NewCommentService(tasks, history, users)
	ctx := context.Background()
	views, err := service.ListByProcessInstance(ctx, "pi-1")
	if err != nil {
		t.Fatalf("查询意见失败: %v", err)
	}
	if len(views) != 2 {
		t.Fatalf("应有 2 条意见，实际 %d", len(views))
	}
	// 任务名/类型中文名/用户姓名补全
	if views[0].TaskName != "审批" || views[0].TypeName != "审批" || views[0].UserName != "张三" {
		t.Fatalf("展示字段补全错误: %+v", views[0])
	}
	// 非枚举类型与查不到的用户给空
	if views[1].TypeName != "" || views[1].UserName != "" {
		t.Fatalf("未知类型/用户应给空: %+v", views[1])
	}
	// 姓+名回退
	if got := userDisplayName(api.User{LastName: "李", FirstName: "四"}); got != "李四" {
		t.Fatalf("姓名回退错误: %q", got)
	}
	// 查询失败透传
	tasks.failListInstance = true
	if _, err := service.ListByProcessInstance(ctx, "pi-1"); err == nil {
		t.Fatal("查询失败应透传错误")
	}
}
