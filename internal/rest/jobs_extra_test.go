package rest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

const restTimerJobXML = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="timerJob" isExecutable="true"><startEvent id="s"><timerEventDefinition><timeDuration>PT1H</timeDuration></timerEventDefinition></startEvent><sequenceFlow id="f1" sourceRef="s" targetRef="e"/><endEvent id="e"/></process></definitions>`

const restBoundaryJobXML = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn"><process id="boundaryJob" isExecutable="true"><startEvent id="s"/><sequenceFlow id="f1" sourceRef="s" targetRef="task"/><userTask id="task" name="Task" flowable:assignee="alice"/><boundaryEvent id="boundary" attachedToRef="task"><timerEventDefinition><timeDuration>PT1H</timeDuration></timerEventDefinition></boundaryEvent><sequenceFlow id="f2" sourceRef="boundary" targetRef="end2"/><endEvent id="end2"/><sequenceFlow id="f3" sourceRef="task" targetRef="e"/><endEvent id="e"/></process></definitions>`

// TestREST_TimerJobs 验证定时作业端点与删除。
// @param t 测试上下文
func TestREST_TimerJobs(t *testing.T) {
	server, _ := newTestServer(t)
	response, _ := http.Post(server.URL+"/service/repository/deployments", "application/xml", bytes.NewBufferString(restTimerJobXML))
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/management/timer-jobs")
	page := decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 {
		t.Fatalf("定时作业列表错误: %+v", page)
	}
	jobID, _ := page.Data[0]["id"].(string)
	response, _ = http.Get(server.URL + "/service/management/timer-jobs/" + jobID)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("定时作业详情失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/management/jobs/" + jobID)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("可执行作业详情失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/management/timer-jobs/" + jobID + "/exception-stacktrace")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("无异常栈定时作业应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/management/jobs/" + jobID + "/exception-stacktrace")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("无异常栈作业应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/management/suspended-jobs/" + jobID)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("未挂起作业详情应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/management/suspended-jobs/" + jobID + "/exception-stacktrace")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("未挂起作业异常栈应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	request, _ := http.NewRequest(http.MethodDelete, server.URL+"/service/management/jobs/"+jobID, nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("删除作业失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/management/timer-jobs")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 0 {
		t.Fatalf("删除后定时作业应为空: %+v", page)
	}
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/management/jobs/"+jobID, nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("重复删除应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/management/jobs/missing/exception-stacktrace")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失作业异常栈应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/management/timer-jobs/missing")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失定时作业应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_SuspendedAndHistoryJobs 验证挂起作业与历史作业端点。
// @param t 测试上下文
func TestREST_SuspendedAndHistoryJobs(t *testing.T) {
	server, _ := newTestServer(t)
	response, _ := http.Post(server.URL+"/service/repository/deployments", "application/xml", bytes.NewBufferString(restBoundaryJobXML))
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{"processDefinitionKey": "boundaryJob"})
	var instance struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(response.Body).Decode(&instance)
	_ = response.Body.Close()
	if instance.ID == "" {
		t.Fatalf("启动实例失败")
	}
	response, _ = http.Get(server.URL + "/service/management/suspended-jobs")
	page := decodeQueryPage[map[string]any](t, response)
	if page.Total != 0 {
		t.Fatalf("未挂起时不应有挂起作业: %+v", page)
	}
	suspendBody, _ := json.Marshal(map[string]any{"action": "suspend"})
	request, _ := http.NewRequest(http.MethodPut, server.URL+"/service/runtime/process-instances/"+instance.ID, bytes.NewReader(suspendBody))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("挂起实例失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/management/suspended-jobs")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 {
		t.Fatalf("挂起后作业列表错误: %+v", page)
	}
	jobID, _ := page.Data[0]["id"].(string)
	response, _ = http.Get(server.URL + "/service/management/suspended-jobs/" + jobID)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("挂起作业详情失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/management/timer-jobs")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 0 {
		t.Fatalf("挂起作业不应出现在定时作业列表: %+v", page)
	}
	response, _ = http.Get(server.URL + "/service/management/timer-jobs/" + jobID)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("挂起作业定时详情应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/management/suspended-jobs/" + jobID + "/exception-stacktrace")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("无异常栈挂起作业应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	response, _ = http.Get(server.URL + "/service/management/history-jobs")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 0 {
		t.Fatalf("历史作业应为空: %+v", page)
	}
	response, _ = http.Get(server.URL + "/service/management/history-jobs/missing")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失历史作业应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/management/deadletter-jobs/missing/exception-stacktrace")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失死信作业异常栈应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/management/suspended-jobs/missing")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失挂起作业应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/management/jobs/"+jobID, nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("删除挂起作业失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}
