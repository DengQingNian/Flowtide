package rest

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/tenant"
)

// TestWorkflowInjectAfter 验证兼容HTTP入口接受原拼写并通过引擎在原节点之后生成任务。
// @param t 测试上下文
// @return 无
func TestWorkflowInjectAfter(t *testing.T) {
	env := newWorkflowFacadeEnv(t)
	ctx := tenant.With(t.Context(), "bizA")
	instance, err := env.engine.RuntimeService().StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "facadeProc", Variables: []api.Variable{{Name: "initiator", Value: "u1"}}})
	if err != nil {
		t.Fatal(err)
	}
	url := env.server.URL + "/workflow/process/inject/node"
	for _, tc := range []struct {
		name, definition string
		success          bool
	}{{"无定义", "", false}, {"定义错误", "missing", false}, {"原始字段", instance.ProcessDefinitionID, true}} {
		t.Run(tc.name, func(t *testing.T) {
			payload, _ := json.Marshal(map[string]string{"processInstaceId": instance.ID, "processDefinitionId": tc.definition, "userTaskId": "apply", "name": "临时复核", "assginee": "u2"})
			request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(payload)))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-Tenant-Id", "bizA")
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if tc.success {
				body, err := io.ReadAll(response.Body)
				if err != nil || response.StatusCode != http.StatusOK || len(body) != 0 {
					t.Fatalf("成功应为空响应：%d %q %v", response.StatusCode, body, err)
				}
				return
			}
			var body workflowRBody
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if response.StatusCode == http.StatusNotImplemented || body.Success != tc.success {
				t.Fatalf("响应错误：%d %+v", response.StatusCode, body)
			}
		})
	}
	page, err := env.engine.TaskService().ListTasks(ctx, api.TaskFilter{ProcessInstanceID: instance.ID, Size: 10})
	if err != nil || len(page.Data) != 1 || page.Data[0].TaskDefinitionKey != "apply" {
		t.Fatalf("原任务状态错误：%+v %v", page, err)
	}
	if err := env.engine.TaskService().Complete(ctx, page.Data[0].ID, api.CompleteTaskRequest{UserID: "u1"}); err != nil {
		t.Fatal(err)
	}
	page, err = env.engine.TaskService().ListTasks(ctx, api.TaskFilter{ProcessInstanceID: instance.ID, Size: 10})
	if err != nil || len(page.Data) != 1 || !strings.HasPrefix(page.Data[0].TaskDefinitionKey, "UserTaskAdd") || page.Data[0].Assignee != "u2" {
		t.Fatalf("动态节点未正确执行：%+v %v", page, err)
	}
}
