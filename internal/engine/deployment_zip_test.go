package engine

import (
	"archive/zip"
	"bytes"
	"context"
	"testing"

	"github.com/flowable-go/flowable/api"
)

// makeZip 构造 ZIP 部署包。
// @param entries 条目名到内容
// @return ZIP 字节
func makeZip(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	buffer := &bytes.Buffer{}
	writer := zip.NewWriter(buffer)
	for name, content := range entries {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatalf("创建 ZIP 条目失败: %v", err)
		}
		if _, err := file.Write(content); err != nil {
			t.Fatalf("写入 ZIP 条目失败: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("关闭 ZIP 失败: %v", err)
	}
	return buffer.Bytes()
}

// TestDeploy_ZIP 验证 ZIP 部署。
// @param t 测试上下文
func TestDeploy_ZIP(t *testing.T) {
	engine := newTestEngine(t)
	archive := makeZip(t, map[string][]byte{"processes/simple.bpmn20.xml": []byte(simpleProcess)})
	if _, err := engine.Deploy(context.Background(), api.DeploymentRequest{
		Name:      "zip",
		Resources: map[string][]byte{"demo.zip": archive},
	}); err != nil {
		t.Fatalf("ZIP 部署失败: %v", err)
	}
	definition, err := engine.GetProcessDefinitionByKey(context.Background(), "simple")
	if err != nil || definition.ResourceName != "processes/simple.bpmn20.xml" {
		t.Fatalf("ZIP 部署定义错误: %v %+v", err, definition)
	}
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	}); err != nil {
		t.Fatalf("ZIP 部署启动失败: %v", err)
	}
}

// TestDeploy_ZIPErrors 验证 ZIP 错误。
// @param t 测试上下文
func TestDeploy_ZIPErrors(t *testing.T) {
	engine := newTestEngine(t)
	if _, err := engine.Deploy(context.Background(), api.DeploymentRequest{
		Resources: map[string][]byte{"bad.zip": []byte("not-a-zip")},
	}); err == nil {
		t.Fatalf("非法 ZIP 应部署失败")
	}
	archive := makeZip(t, map[string][]byte{"../evil.bpmn20.xml": []byte(simpleProcess)})
	if _, err := engine.Deploy(context.Background(), api.DeploymentRequest{
		Resources: map[string][]byte{"evil.zip": archive},
	}); err == nil {
		t.Fatalf("非法路径 ZIP 应部署失败")
	}
}
