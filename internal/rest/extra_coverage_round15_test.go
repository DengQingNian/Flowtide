package rest

import (
	"bytes"
	"net/http"
	"testing"
)

// TestREST_Round15ErrorPaths 补充若干错误路径覆盖。
// @param t 测试上下文
func TestREST_Round15ErrorPaths(t *testing.T) {
	server, _ := newTestServer(t)
	task := createRESTTask(t, server.URL, map[string]any{"name": "Task"}, "")
	taskID, _ := task["id"].(string)

	getCases := []struct {
		path string
		want int
	}{
		{"/service/runtime/process-instances/missing/identitylinks", http.StatusNotFound},
		{"/service/repository/process-definitions/missing/identitylinks/users/x", http.StatusNotFound},
		{"/service/runtime/tasks/" + taskID + "/events/999999", http.StatusNotFound},
	}
	for _, tc := range getCases {
		response, _ := http.Get(server.URL + tc.path)
		if response.StatusCode != tc.want {
			t.Fatalf("%s 期望 %d 实际 %d", tc.path, tc.want, response.StatusCode)
		}
		_ = response.Body.Close()
	}

	request, _ := http.NewRequest(http.MethodDelete, server.URL+"/service/runtime/tasks/"+taskID+"/events/abc", nil)
	response, _ := http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法事件 ID 删除应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	request, _ = http.NewRequest(http.MethodPost, server.URL+"/service/query/activity-instances", bytes.NewBufferString("{"))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法活动查询体应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	request, _ = http.NewRequest(http.MethodPut, server.URL+"/service/identity/users/missing/info/title", bytes.NewBufferString(`{"value":"x"}`))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失用户信息更新应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}
