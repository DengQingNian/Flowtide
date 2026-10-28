package rest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

// TestREST_DeploymentResourceContentType 验证部署资源类型。
// @param t 测试上下文
func TestREST_DeploymentResourceContentType(t *testing.T) {
	server, _ := newTestServer(t)
	cases := []struct {
		name        string
		body        string
		contentType string
	}{
		{"process.bpmn20.xml", restProcess, "application/xml"},
		{"form.form", `{"key":"form1","name":"Form"}`, "application/json"},
		{"data.bin", "binary", "application/octet-stream"},
	}
	for _, tc := range cases {
		response, _ := http.Post(server.URL+"/service/repository/deployments?resourceName="+tc.name, "application/octet-stream", bytes.NewBufferString(tc.body))
		if response.StatusCode != http.StatusCreated {
			t.Fatalf("部署 %s 失败: %d", tc.name, response.StatusCode)
		}
		var deployment struct {
			ID string `json:"id"`
		}
		_ = json.NewDecoder(response.Body).Decode(&deployment)
		_ = response.Body.Close()
		response, _ = http.Get(server.URL + "/service/repository/deployments/" + deployment.ID + "/resources/" + tc.name)
		_ = response.Body.Close()
		if response.Header.Get("Content-Type") != tc.contentType {
			t.Fatalf("%s Content-Type 期望 %s 实际 %s", tc.name, tc.contentType, response.Header.Get("Content-Type"))
		}
	}
}
