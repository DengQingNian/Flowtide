package rest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
)

// TestActivityInstanceSortKey 验证活动实例排序字段。
// @param t 测试上下文
func TestActivityInstanceSortKey(t *testing.T) {
	item := api.ActivityInstance{ID: "a", ActivityID: "task", ActivityType: "userTask", ActivityName: "Task", StartTime: nil}
	for _, field := range []string{"id", "activityId", "activityType", "activityName", "startTime"} {
		key, ok := activityInstanceSortKey(field)
		if !ok {
			t.Fatalf("活动排序字段缺失: %s", field)
		}
		_ = key(item)
	}
	if _, ok := activityInstanceSortKey("missing"); ok {
		t.Fatalf("未知活动排序字段应不支持")
	}
}

// TestREST_ActivityInstances 验证活动实例端点。
// @param t 测试上下文
func TestREST_ActivityInstances(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	instanceID := startQueryInstance(t, server.URL, "rest", "bk1", "Order")

	response, _ := http.Get(server.URL + "/service/runtime/activity-instances?processInstanceId=" + instanceID)
	page := decodeQueryPage[map[string]any](t, response)
	if response.StatusCode != http.StatusOK || page.Total == 0 {
		t.Fatalf("活动实例列表错误: %d %+v", response.StatusCode, page)
	}
	found := false
	for _, item := range page.Data {
		if item["activityId"] == "task" && item["activityType"] == "userTask" && item["taskId"] != nil {
			found = true
		}
	}
	if !found {
		t.Fatalf("未找到用户任务活动: %+v", page.Data)
	}
	for _, query := range []string{
		"activityId=task",
		"activityType=userTask",
		"taskAssignee=alice",
		"activityNameLike=%25prove%25",
	} {
		response, _ = http.Get(server.URL + "/service/runtime/activity-instances?processInstanceId=" + instanceID + "&" + query)
		page = decodeQueryPage[map[string]any](t, response)
		if page.Total == 0 {
			t.Fatalf("活动过滤失败 %s: %+v", query, page)
		}
	}
	for _, field := range []string{"id", "activityId", "activityType", "activityName", "startTime"} {
		response, _ = http.Get(server.URL + "/service/runtime/activity-instances?sort=" + field)
		page = decodeQueryPage[map[string]any](t, response)
		if response.StatusCode != http.StatusOK || page.Total == 0 {
			t.Fatalf("活动排序失败 %s: %d %+v", field, response.StatusCode, page)
		}
	}
	response, _ = http.Get(server.URL + "/service/runtime/activity-instances?sort=activityId&order=desc")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total == 0 || page.Order != "desc" {
		t.Fatalf("活动排序失败: %+v", page)
	}
	response, _ = http.Get(server.URL + "/service/runtime/activity-instances?order=sideways")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法排序方向应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/activity-instances?sort=missing")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法排序应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	response = postQuery(t, server.URL, "/service/query/activity-instances", map[string]any{
		"processInstanceId": instanceID,
		"activityId":        "task",
	}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if response.StatusCode != http.StatusOK || page.Total == 0 || page.Data[0]["activityId"] != "task" {
		t.Fatalf("活动查询失败: %d %+v", response.StatusCode, page)
	}
	response = postQuery(t, server.URL, "/service/query/activity-instances", map[string]any{}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total == 0 {
		t.Fatalf("空查询活动失败: %+v", page)
	}
	response = postQuery(t, server.URL, "/service/query/activity-instances", map[string]any{"sort": "missing"}, "")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法排序应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postQuery(t, server.URL, "/service/query/activity-instances", map[string]any{"start": "x"}, "")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法分页应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/service/query/activity-instances", bytes.NewBufferString("{"))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法查询体应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	response, _ = http.Get(server.URL + "/service/runtime/executions?processInstanceId=" + instanceID)
	var executions struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&executions)
	_ = response.Body.Close()
	if len(executions.Data) == 0 {
		t.Fatalf("执行为空")
	}
	response, _ = http.Get(server.URL + "/service/runtime/executions/" + executions.Data[0].ID + "/activities")
	var activities []string
	_ = json.NewDecoder(response.Body).Decode(&activities)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || len(activities) == 0 {
		t.Fatalf("执行活动错误: %d %+v", response.StatusCode, activities)
	}
	response, _ = http.Get(server.URL + "/service/runtime/executions/missing/activities")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失执行活动应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}
