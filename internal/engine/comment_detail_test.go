package engine

import (
	"strings"
	"testing"
)

// TestAddCommentDetailTaskScoped 验证任务级评论携带自定义类型与完整原文。
// 场景：按 Java AddHisCommentCmd 语义（TYPE_ 显式、MESSAGE_ 归一化、FULL_MSG_ 原文）添加任务评论；
// 预期：类型/原文/任务归属与 JAVA 落库列一一对应。
// @param t 测试上下文
func TestAddCommentDetailTaskScoped(t *testing.T) {
	eng, ctx := newResolveTestEngine(t)
	taskID, _ := startResolveTask(t, eng, ctx)
	comment, err := eng.TaskService().AddCommentDetail(ctx, taskID, "", "22", "ZP", "已 核实", "已  核实")
	if err != nil {
		t.Fatal(err)
	}
	if comment.Type != "ZP" {
		t.Fatalf("类型应为 ZP，实际 %s", comment.Type)
	}
	if comment.Message != "已 核实" || comment.FullMessage != "已  核实" {
		t.Fatalf("MESSAGE_ 应存归一化内容、FULL_MSG_ 存原文，实际 message=%q full=%q", comment.Message, comment.FullMessage)
	}
	if comment.TaskID != taskID || comment.ProcessInstanceID == "" {
		t.Fatalf("任务归属缺失：taskID=%s procInst=%s", comment.TaskID, comment.ProcessInstanceID)
	}
	if comment.Action != "AddComment" {
		t.Fatalf("动作应为 AddComment，实际 %s", comment.Action)
	}
}

// TestAddCommentDetailDefaultType 验证类型缺省时按 Java 默认 SP。
// 场景：typ 传空串；
// 预期：落库类型为 SP。
// @param t 测试上下文
func TestAddCommentDetailDefaultType(t *testing.T) {
	eng, ctx := newResolveTestEngine(t)
	taskID, _ := startResolveTask(t, eng, ctx)
	comment, err := eng.TaskService().AddCommentDetail(ctx, taskID, "", "22", "", "同意", "同意")
	if err != nil {
		t.Fatal(err)
	}
	if comment.Type != "SP" {
		t.Fatalf("类型缺省应为 SP，实际 %s", comment.Type)
	}
}

// TestAddCommentDetailInstanceScoped 验证流程级评论（无任务上下文）。
// 场景：taskID 为空、processInstanceId 显式给出；
// 预期：评论挂实例且 TaskID 为空。
// @param t 测试上下文
func TestAddCommentDetailInstanceScoped(t *testing.T) {
	eng, ctx := newResolveTestEngine(t)
	_, instanceID := startResolveTask(t, eng, ctx)
	comment, err := eng.TaskService().AddCommentDetail(ctx, "", instanceID, "22", "SP", "阶段说明", "阶段说明")
	if err != nil {
		t.Fatal(err)
	}
	if comment.TaskID != "" || comment.ProcessInstanceID != instanceID {
		t.Fatalf("应为流程级评论：taskID=%q procInst=%q", comment.TaskID, comment.ProcessInstanceID)
	}
}

// TestAddCommentDetailInstanceMissing 验证实例不存在时拒绝（对位 Java FlowableObjectNotFoundException）。
// 场景：taskID 为空且实例 Id 无效；
// 预期：返回 ENGINE-0404 流程实例不存在。
// @param t 测试上下文
func TestAddCommentDetailInstanceMissing(t *testing.T) {
	eng, ctx := newResolveTestEngine(t)
	_, err := eng.TaskService().AddCommentDetail(ctx, "", "missing", "22", "SP", "内容", "内容")
	if err == nil || !strings.Contains(err.Error(), "流程实例不存在") {
		t.Fatalf("应拒绝不存在的实例，实际 err=%v", err)
	}
}

// TestAddCommentDetailBlankMessage 验证空白内容拒绝。
// 场景：message 为空白；
// 预期：返回 ENGINE-1101。
// @param t 测试上下文
func TestAddCommentDetailBlankMessage(t *testing.T) {
	eng, ctx := newResolveTestEngine(t)
	taskID, _ := startResolveTask(t, eng, ctx)
	_, err := eng.TaskService().AddCommentDetail(ctx, taskID, "", "22", "SP", "  ", "  ")
	if err == nil || !strings.Contains(err.Error(), "评论内容不能为空") {
		t.Fatalf("应拒绝空白内容，实际 err=%v", err)
	}
}
