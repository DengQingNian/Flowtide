package rest

import (
	"net/http"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/tenant"
)

// TestWorkflowRuntimeLastPerson 验证最后一人判断入口的必填校验、业务错误与普通任务结果。
// @param t 测试上下文
// @return 无
func TestWorkflowRuntimeLastPerson(t *testing.T) {
	env := newWorkflowFacadeEnv(t)
	_, taskID := workflowApply(t, env, "runtime-last-person")
	url := env.server.URL + "/workflow/process/isMultiInstanceLastPerson?tenantId=bizA"

	// 缺少任务标识走参数错误，未知任务返回 false，跨租户访问走业务错误。
	requireWorkflowParamFail(t, doWorkflowJSON(t, http.MethodPost, url, `{}`))
	missing := requireWorkflowOK(t, doWorkflowJSON(t, http.MethodPost, url, `{"taskId":"missing"}`))
	if missing.Data != false {
		t.Fatalf("未知任务应返回 false，实际 %+v", missing)
	}
	requireWorkflowBizFail(t, doWorkflowJSON(t, http.MethodPost,
		env.server.URL+"/workflow/process/isMultiInstanceLastPerson?tenantId=bizB", `{"taskId":"`+taskID+`"}`))
	body := requireWorkflowOK(t, doWorkflowJSON(t, http.MethodPost, url, `{"taskId":"`+taskID+`"}`))
	if body.Data != true {
		t.Fatalf("普通用户任务应为最后一人，实际 %+v", body)
	}
}

// TestWorkflowRuntimeDelegateScan 验证手动委托扫描入口返回统计并在无规则时保留原办理人。
// @param t 测试上下文
// @return 无
func TestWorkflowRuntimeDelegateScan(t *testing.T) {
	env := newWorkflowFacadeEnv(t)
	_, taskID := workflowApply(t, env, "runtime-delegate-scan")
	url := env.server.URL + "/workflow/delegate/scheduled/trigger?tenantId=bizA"
	body := requireWorkflowOK(t, doWorkflowJSON(t, http.MethodPost, url, ""))
	summary := rDataMap(t, body)
	// 同时核对扫描、改派和失败数量，避免只校验成功信封而漏掉统计错误。
	if summary["Scanned"] != float64(1) || summary["Adjusted"] != float64(0) || summary["Failed"] != float64(0) {
		t.Fatalf("无委托规则应扫描一条且无改派或失败：%+v", summary)
	}
	task, err := env.engine.TaskService().GetTask(tenant.With(t.Context(), "bizA"), taskID)
	if err != nil || task.Assignee != "u1" {
		t.Fatalf("无委托规则应保留原办理人：%+v，错误 %v", task, err)
	}
}

// TestWorkflowRuntimeAddComment 验证审批意见入口透传类型、作者、任务与内容，并保留业务错误协议。
// @param t 测试上下文
// @return 无
func TestWorkflowRuntimeAddComment(t *testing.T) {
	env := newWorkflowFacadeEnv(t)
	instanceID, taskID := workflowApply(t, env, "runtime-comment")
	url := env.server.URL + "/workflow/process/comment/add?tenantId=bizA"
	ctx := tenant.With(t.Context(), "bizA")
	before, err := env.engine.TaskService().ListComments(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	body := requireWorkflowOK(t, doWorkflowJSON(t, http.MethodPost, url,
		`{"taskId":"`+taskID+`","processInstanceId":"`+instanceID+`","userId":"u2","type":"ZP","message":"已  核实"}`))
	data := rDataMap(t, body)
	comments, err := env.engine.TaskService().ListComments(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != len(before)+1 {
		t.Fatalf("应新增一条意见，原数量 %d，实际 %+v", len(before), comments)
	}
	var saved *api.Comment
	// 通过响应主键定位新增意见，避免依赖评论列表排序。
	for i := range comments {
		if comments[i].ID == data["id"] {
			saved = &comments[i]
		}
	}
	if saved == nil || saved.TaskID != taskID || saved.ProcessInstanceID != instanceID ||
		saved.UserID != "u2" || saved.Type != "ZP" || saved.Message != "已 核实" || saved.FullMessage != "已  核实" {
		t.Fatalf("意见字段未完整写入：响应 %+v，存储 %+v", data, saved)
	}
	requireWorkflowBizFail(t, doWorkflowJSON(t, http.MethodPost, url,
		`{"taskId":"`+taskID+`","userId":"u2","message":"  "}`))
	after, err := env.engine.TaskService().ListComments(ctx, taskID)
	if err != nil || len(after) != len(comments) {
		t.Fatalf("失败请求不应新增意见：%+v，错误 %v", after, err)
	}
}

// TestWorkflowRuntimeMove 验证两类节点跳转入口的参数透传、任务迁移与失败响应。
// @param t 测试上下文
// @return 无
func TestWorkflowRuntimeMove(t *testing.T) {
	for _, action := range []string{"activity", "execution"} {
		t.Run(action, func(t *testing.T) {
			env := newWorkflowFacadeEnv(t)
			instanceID, taskID := workflowApply(t, env, "runtime-move-"+action)
			ctx := tenant.With(t.Context(), "bizA")
			url := env.server.URL + "/workflow/process/move/" + action + "?tenantId=bizA"
			// 缺少实例与无效目标均应返回业务失败，且保留原待办。
			requireWorkflowBizFail(t, doWorkflowJSON(t, http.MethodPost, url, `{}`))
			requireWorkflowBizFail(t, doWorkflowJSON(t, http.MethodPost, url,
				`{"processInstanceId":"`+instanceID+`","nodeId":"apply","toNodeId":"missing"}`))
			original, err := env.engine.TaskService().GetTask(ctx, taskID)
			if err != nil || original.TaskDefinitionKey != "apply" {
				t.Fatalf("失败跳转应保留原待办：%+v，错误 %v", original, err)
			}
			requireWorkflowOK(t, doWorkflowJSON(t, http.MethodPost, url,
				`{"processInstanceId":"`+instanceID+`","nodeId":"apply","toNodeId":"approve"}`))
			page, err := env.engine.TaskService().ListTasks(ctx, api.TaskFilter{ProcessInstanceID: instanceID, Size: 10})
			if err != nil || len(page.Data) != 1 || page.Data[0].ID == taskID ||
				page.Data[0].TaskDefinitionKey != "approve" || page.Data[0].Assignee != "u2" {
				t.Fatalf("跳转后应仅保留目标节点待办：%+v，错误 %v", page, err)
			}
		})
	}
}
