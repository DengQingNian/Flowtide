package rest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/bpmn/model"
	"github.com/flowable-go/flowable/internal/engine"
)

const definitionArtifactsXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn">
  <message id="msg1" name="MessageOne"/>
  <signal id="sig1" name="SignalOne"/>
  <error id="err1" errorCode="CODE1"/>
  <process id="artifacts" name="Artifacts" isExecutable="true">
    <documentation>流程说明</documentation>
    <startEvent id="start" name="开始"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="task"/>
    <userTask id="task" name="审批" flowable:assignee="alice" flowable:formKey="approveForm"
              flowable:candidateUsers="bob,carol" flowable:candidateGroups="managers,admins"/>
    <sequenceFlow id="f2" sourceRef="task" targetRef="end"/>
    <endEvent id="end" name="结束"/>
  </process>
</definitions>`

// newDefinitionArtifactsFixture 构建带流程定义的 REST 服务。
// @param t 测试上下文
// @return REST 服务
// @return 流程定义 ID
func newDefinitionArtifactsFixture(t *testing.T) (*Server, string) {
	t.Helper()
	app, err := engine.New(engine.Config{HistoryLevel: engine.HistoryAudit})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	t.Cleanup(func() {
		_ = app.Close(context.Background())
	})
	if _, err := app.RepositoryService().Deploy(context.Background(), api.DeploymentRequest{
		Name:      "artifacts",
		Resources: map[string][]byte{"artifacts.bpmn": []byte(definitionArtifactsXML)},
	}); err != nil {
		t.Fatalf("部署失败: %v", err)
	}
	definitions, err := app.RepositoryService().ListProcessDefinitions(context.Background())
	if err != nil || len(definitions) != 1 {
		t.Fatalf("查询流程定义失败: %v %+v", err, definitions)
	}
	return New(app), definitions[0].ID
}

// stubDefinitionRepository 是仅覆盖模型查询的仓库服务桩。
type stubDefinitionRepository struct {
	api.RepositoryService                       // 其余方法由嵌入接口提供
	definition            api.ProcessDefinition // 流程定义
	model                 *model.BpmnModel      // BPMN 模型
	modelErr              error                 // 模型查询错误
}

// GetProcessDefinition 返回预设流程定义。
// @param ctx 上下文
// @param id 流程定义 ID
// @return 流程定义
// @return 查询错误
func (s stubDefinitionRepository) GetProcessDefinition(ctx context.Context, id string) (api.ProcessDefinition, error) {
	return s.definition, nil
}

// GetBpmnModel 返回预设 BPMN 模型或错误。
// @param ctx 上下文
// @param definitionID 流程定义 ID
// @return BPMN 模型
// @return 查询错误
func (s stubDefinitionRepository) GetBpmnModel(ctx context.Context, definitionID string) (*model.BpmnModel, error) {
	return s.model, s.modelErr
}

// stubDefinitionEngine 是仅覆盖仓库服务的引擎桩。
type stubDefinitionEngine struct {
	api.Engine                       // 其余方法由嵌入接口提供
	repository api.RepositoryService // 仓库服务
}

// RepositoryService 返回预设仓库服务。
// @return 仓库服务
func (s stubDefinitionEngine) RepositoryService() api.RepositoryService {
	return s.repository
}

// callDefinitionArtifactHandler 直接调用 handler 并返回响应记录器。
// @param handler HTTP 处理函数
// @param definitionID 流程定义 ID
// @return 响应记录器
func callDefinitionArtifactHandler(handler http.HandlerFunc, definitionID string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/service/repository/process-definitions/"+definitionID+"/model", nil)
	request.SetPathValue("id", definitionID)
	recorder := httptest.NewRecorder()
	handler(recorder, request)
	return recorder
}

// TestREST_DefinitionModel 验证流程定义模型端点。
// @param t 测试上下文
func TestREST_DefinitionModel(t *testing.T) {
	server, definitionID := newDefinitionArtifactsFixture(t)
	recorder := callDefinitionArtifactHandler(server.handleDefinitionModel, definitionID)
	if recorder.Code != http.StatusOK {
		t.Fatalf("模型端点状态码错误: %d %s", recorder.Code, recorder.Body.String())
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.Contains(contentType, "application/json") {
		t.Fatalf("模型端点 Content-Type 错误: %s", contentType)
	}

	var dto definitionModelDTO
	if err := json.Unmarshal(recorder.Body.Bytes(), &dto); err != nil {
		t.Fatalf("解析模型响应失败: %v", err)
	}
	if dto.ID != definitionID || dto.Name != "Artifacts" {
		t.Fatalf("模型基础字段错误: %+v", dto)
	}
	if len(dto.Processes) != 1 {
		t.Fatalf("流程数量错误: %+v", dto.Processes)
	}
	process := dto.Processes[0]
	if process.ID != "artifacts" || process.Name != "Artifacts" || process.Documentation != "流程说明" || !process.Executable {
		t.Fatalf("流程字段错误: %+v", process)
	}
	wantOrder := []string{"start", "f1", "task", "f2", "end"}
	if len(process.FlowElements) != len(wantOrder) {
		t.Fatalf("流程元素数量错误: %+v", process.FlowElements)
	}
	for index, id := range wantOrder {
		if process.FlowElements[index].ID != id {
			t.Fatalf("流程元素顺序错误: 第 %d 个为 %s，期望 %s", index, process.FlowElements[index].ID, id)
		}
	}
	byID := make(map[string]definitionFlowElementDTO, len(process.FlowElements))
	for _, element := range process.FlowElements {
		byID[element.ID] = element
	}
	if byID["start"].Type != "startEvent" || byID["start"].Name != "开始" {
		t.Fatalf("开始事件字段错误: %+v", byID["start"])
	}
	if byID["f1"].Type != "sequenceFlow" || byID["f1"].SourceRef != "start" || byID["f1"].TargetRef != "task" {
		t.Fatalf("顺序流字段错误: %+v", byID["f1"])
	}
	task := byID["task"]
	if task.Type != "userTask" || task.Assignee != "alice" || task.FormKey != "approveForm" {
		t.Fatalf("用户任务字段错误: %+v", task)
	}
	if len(task.CandidateUsers) != 2 || task.CandidateUsers[0] != "bob" || task.CandidateUsers[1] != "carol" {
		t.Fatalf("候选用户错误: %+v", task.CandidateUsers)
	}
	if len(task.CandidateGroups) != 2 || task.CandidateGroups[0] != "managers" || task.CandidateGroups[1] != "admins" {
		t.Fatalf("候选组错误: %+v", task.CandidateGroups)
	}
	if byID["end"].Type != "endEvent" {
		t.Fatalf("结束事件字段错误: %+v", byID["end"])
	}
	if len(dto.Messages) != 1 || dto.Messages[0].ID != "msg1" || dto.Messages[0].Name != "MessageOne" {
		t.Fatalf("消息字段错误: %+v", dto.Messages)
	}
	if len(dto.Signals) != 1 || dto.Signals[0].ID != "sig1" || dto.Signals[0].Name != "SignalOne" {
		t.Fatalf("信号字段错误: %+v", dto.Signals)
	}
	if len(dto.Errors) != 1 || dto.Errors[0].ID != "err1" || dto.Errors[0].ErrorCode != "CODE1" {
		t.Fatalf("错误字段错误: %+v", dto.Errors)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &raw); err != nil {
		t.Fatalf("解析原始 JSON 失败: %v", err)
	}
	// 顶层字段必须为 camelCase 且完整
	for _, key := range []string{"id", "name", "processes", "messages", "signals", "errors"} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("缺少顶层字段 %s: %s", key, recorder.Body.String())
		}
	}
	if len(raw) != 6 {
		t.Fatalf("顶层字段数量错误: %s", recorder.Body.String())
	}
	var rawProcesses []map[string]json.RawMessage
	if err := json.Unmarshal(raw["processes"], &rawProcesses); err != nil || len(rawProcesses) != 1 {
		t.Fatalf("解析原始流程失败: %v", err)
	}
	var rawElements []map[string]json.RawMessage
	if err := json.Unmarshal(rawProcesses[0]["flowElements"], &rawElements); err != nil {
		t.Fatalf("解析原始流程元素失败: %v", err)
	}
	// 校验顺序流与用户任务扩展字段使用 camelCase 命名
	if _, ok := rawElements[1]["sourceRef"]; !ok {
		t.Fatalf("顺序流缺少 sourceRef: %s", rawProcesses[0]["flowElements"])
	}
	if _, ok := rawElements[1]["targetRef"]; !ok {
		t.Fatalf("顺序流缺少 targetRef: %s", rawProcesses[0]["flowElements"])
	}
	for _, key := range []string{"assignee", "formKey", "candidateUsers", "candidateGroups"} {
		if _, ok := rawElements[2][key]; !ok {
			t.Fatalf("用户任务缺少 %s: %s", key, rawProcesses[0]["flowElements"])
		}
	}
}

// TestREST_DefinitionArtifactsEmpty 验证决策、决策表与表单定义端点。
// @param t 测试上下文
func TestREST_DefinitionArtifactsEmpty(t *testing.T) {
	server, definitionID := newDefinitionArtifactsFixture(t)
	cases := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{name: "decisions", handler: server.handleDefinitionDecisions},
		{name: "decision-tables", handler: server.handleDefinitionDecisionTables},
		{name: "form-definitions", handler: server.handleDefinitionFormDefinitions},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := callDefinitionArtifactHandler(testCase.handler, definitionID)
			if recorder.Code != http.StatusOK {
				t.Fatalf("%s 状态码错误: %d", testCase.name, recorder.Code)
			}
			if body := strings.TrimSpace(recorder.Body.String()); body != "[]" {
				t.Fatalf("%s 应返回空数组: %s", testCase.name, body)
			}
		})
	}
}

// TestREST_DefinitionArtifactsNotFound 验证流程定义不存在时返回 404。
// @param t 测试上下文
func TestREST_DefinitionArtifactsNotFound(t *testing.T) {
	server, _ := newDefinitionArtifactsFixture(t)
	cases := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{name: "model", handler: server.handleDefinitionModel},
		{name: "decisions", handler: server.handleDefinitionDecisions},
		{name: "decision-tables", handler: server.handleDefinitionDecisionTables},
		{name: "form-definitions", handler: server.handleDefinitionFormDefinitions},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := callDefinitionArtifactHandler(testCase.handler, "missing")
			if recorder.Code != http.StatusNotFound {
				t.Fatalf("%s 缺失定义应 404: %d %s", testCase.name, recorder.Code, recorder.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("%s 解析错误响应失败: %v", testCase.name, err)
			}
			if message, _ := body["message"].(string); message == "" {
				t.Fatalf("%s 错误响应缺少 message: %s", testCase.name, recorder.Body.String())
			}
		})
	}
}

// TestREST_BuildDefinitionModelDTO 验证模型转换的边界分支。
// @param t 测试上下文
func TestREST_BuildDefinitionModelDTO(t *testing.T) {
	empty := buildDefinitionModelDTO(api.ProcessDefinition{ID: "def", Name: "Def"}, nil)
	if empty.ID != "def" || empty.Name != "Def" {
		t.Fatalf("空模型基础字段错误: %+v", empty)
	}
	if empty.Processes == nil || empty.Messages == nil || empty.Signals == nil || empty.Errors == nil {
		t.Fatalf("空模型应返回非 nil 空数组: %+v", empty)
	}
	if len(empty.Processes) != 0 || len(empty.Messages) != 0 || len(empty.Signals) != 0 || len(empty.Errors) != 0 {
		t.Fatalf("空模型数组应为空: %+v", empty)
	}

	bpmnModel := &model.BpmnModel{
		Processes: []*model.Process{
			nil,
			{
				ID:            "p",
				Name:          "P",
				Documentation: "文档",
				Executable:    true,
				Order:         []string{"missing", "flow", "manual"},
				Elements: map[string]model.FlowElement{
					"flow": &model.SequenceFlow{
						BaseElement: model.BaseElement{ID: "flow", Name: "Flow"},
						SourceRef:   "a",
						TargetRef:   "b",
					},
					"manual": &model.ManualTask{
						FlowNodeBase: model.FlowNodeBase{BaseElement: model.BaseElement{ID: "manual", Name: "Manual"}},
					},
				},
			},
		},
		Messages: []*model.Message{nil, {ID: "m", Name: "M"}},
		Signals:  []*model.Signal{nil, {ID: "s", Name: "S"}},
		Errors:   []*model.Error{nil, {ID: "e", ErrorCode: "C"}},
	}
	dto := buildDefinitionModelDTO(api.ProcessDefinition{ID: "def"}, bpmnModel)
	if len(dto.Processes) != 1 {
		t.Fatalf("空流程应被跳过: %+v", dto.Processes)
	}
	process := dto.Processes[0]
	if process.ID != "p" || process.Documentation != "文档" || !process.Executable {
		t.Fatalf("流程转换错误: %+v", process)
	}
	if len(process.FlowElements) != 2 {
		t.Fatalf("缺失元素应被跳过: %+v", process.FlowElements)
	}
	if process.FlowElements[0].ID != "flow" || process.FlowElements[0].SourceRef != "a" || process.FlowElements[0].TargetRef != "b" {
		t.Fatalf("顺序流转换错误: %+v", process.FlowElements[0])
	}
	if process.FlowElements[1].Type != "manualTask" || process.FlowElements[1].SourceRef != "" {
		t.Fatalf("默认元素转换错误: %+v", process.FlowElements[1])
	}
	if len(dto.Messages) != 1 || dto.Messages[0].ID != "m" {
		t.Fatalf("空消息应被跳过: %+v", dto.Messages)
	}
	if len(dto.Signals) != 1 || dto.Signals[0].ID != "s" {
		t.Fatalf("空信号应被跳过: %+v", dto.Signals)
	}
	if len(dto.Errors) != 1 || dto.Errors[0].ErrorCode != "C" {
		t.Fatalf("空错误应被跳过: %+v", dto.Errors)
	}
}

// TestREST_DefinitionModelBpmnError 验证模型读取失败时返回 500。
// @param t 测试上下文
func TestREST_DefinitionModelBpmnError(t *testing.T) {
	server := &Server{engine: stubDefinitionEngine{repository: stubDefinitionRepository{
		definition: api.ProcessDefinition{ID: "def", Name: "Def"},
		modelErr:   errors.New("模型读取失败"),
	}}}
	recorder := callDefinitionArtifactHandler(server.handleDefinitionModel, "def")
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("模型读取失败应 500: %d %s", recorder.Code, recorder.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析错误响应失败: %v", err)
	}
	if exception, _ := body["exception"].(string); exception != "模型读取失败" {
		t.Fatalf("错误响应 exception 字段错误: %s", recorder.Body.String())
	}
}
