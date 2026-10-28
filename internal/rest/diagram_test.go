package rest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

const restProcessWithDI = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn" xmlns:bpmndi="http://www.omg.org/spec/BPMN/20100524/DI" xmlns:omgdc="http://www.omg.org/spec/DD/20100524/DC" xmlns:omgdi="http://www.omg.org/spec/DD/20100524/DI"><process id="diag" name="Diagram" isExecutable="true"><startEvent id="start" name="Start"/><sequenceFlow id="f1" sourceRef="start" targetRef="task"/><userTask id="task" name="Approve" flowable:assignee="alice"/><sequenceFlow id="f2" sourceRef="task" targetRef="end"/><endEvent id="end" name="End"/></process><bpmndi:BPMNDiagram id="d1"><bpmndi:BPMNPlane bpmnElement="diag"><bpmndi:BPMNShape bpmnElement="start" id="s1"><omgdc:Bounds x="100" y="100" width="36" height="36"/></bpmndi:BPMNShape><bpmndi:BPMNShape bpmnElement="task" id="s2"><omgdc:Bounds x="200" y="90" width="120" height="60"/></bpmndi:BPMNShape><bpmndi:BPMNShape bpmnElement="end" id="s3"><omgdc:Bounds x="400" y="100" width="36" height="36"/></bpmndi:BPMNShape><bpmndi:BPMNEdge bpmnElement="f1" id="e1"><omgdi:waypoint x="136" y="118"/><omgdi:waypoint x="200" y="118"/></bpmndi:BPMNEdge><bpmndi:BPMNEdge bpmnElement="f2" id="e2"><omgdi:waypoint x="320" y="120"/><omgdi:waypoint x="400" y="118"/></bpmndi:BPMNEdge></bpmndi:BPMNPlane></bpmndi:BPMNDiagram></definitions>`

// TestREST_DiagramImages 验证流程图图片端点。
// @param t 测试上下文
func TestREST_DiagramImages(t *testing.T) {
	server, _ := newTestServer(t)
	response, _ := http.Post(server.URL+"/service/repository/deployments", "application/xml", bytes.NewBufferString(restProcessWithDI))
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/repository/process-definitions")
	var page struct {
		Data []struct {
			ID  string `json:"id"`
			Key string `json:"key"`
		} `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&page)
	_ = response.Body.Close()
	if len(page.Data) == 0 {
		t.Fatalf("流程定义为空")
	}
	definitionID := page.Data[0].ID

	response, _ = http.Get(server.URL + "/service/repository/process-definitions/" + definitionID + "/image")
	pngData, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "image/png" ||
		!bytes.HasPrefix(pngData, []byte{0x89, 'P', 'N', 'G'}) {
		t.Fatalf("定义图片错误: %d %s", response.StatusCode, response.Header.Get("Content-Type"))
	}

	request, _ := http.NewRequest(http.MethodGet, server.URL+"/service/repository/process-definitions/"+definitionID+"/image", nil)
	request.Header.Set("Accept", "image/svg+xml")
	response, _ = http.DefaultClient.Do(request)
	svgData, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "image/svg+xml" ||
		!bytes.Contains(svgData, []byte("<svg")) {
		t.Fatalf("定义 SVG 错误: %d %s", response.StatusCode, svgData)
	}

	response, _ = http.Get(server.URL + "/service/repository/process-definitions/missing/image")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失定义图片应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	deployREST(t, server)
	response, _ = http.Get(server.URL + "/service/repository/process-definitions")
	_ = json.NewDecoder(response.Body).Decode(&page)
	_ = response.Body.Close()
	plainID := ""
	for _, item := range page.Data {
		if item.Key == "rest" {
			plainID = item.ID
		}
	}
	if plainID == "" {
		t.Fatalf("未找到无图形定义")
	}
	response, _ = http.Get(server.URL + "/service/repository/process-definitions/" + plainID + "/image")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("无图形定义应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	instanceID := startQueryInstance(t, server.URL, "diag", "bk1", "Diagram")
	response, _ = http.Get(server.URL + "/service/runtime/process-instances/" + instanceID + "/diagram")
	instancePNG, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || !bytes.HasPrefix(instancePNG, []byte{0x89, 'P', 'N', 'G'}) {
		t.Fatalf("实例流程图错误: %d", response.StatusCode)
	}
	response, _ = http.Get(server.URL + "/service/runtime/process-instances/missing/diagram")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失实例流程图应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}
