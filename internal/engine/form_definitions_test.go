package engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/tenant"
)

// formDefinitionProcessXML 是带启动表单与任务表单的流程。
const formDefinitionProcessXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn">
  <process id="formDefinitionProcess" name="Form Definition" isExecutable="true">
    <startEvent id="start" flowable:formKey="startForm">
      <extensionElements>
        <flowable:formProperty id="bpmnStart" name="BPMN 启动字段" type="string"/>
      </extensionElements>
    </startEvent>
    <sequenceFlow id="f1" sourceRef="start" targetRef="task"/>
    <userTask id="task" name="Approve" flowable:formKey="taskForm">
      <extensionElements>
        <flowable:formProperty id="bpmnTask" name="BPMN 任务字段" type="string" required="true"/>
      </extensionElements>
    </userTask>
    <sequenceFlow id="f2" sourceRef="task" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// startFormJSON 是启动表单定义。
const startFormJSON = `{
  "key": "startForm",
  "name": "Start Form",
  "fields": [
    {"id": "initiator", "name": "发起人", "type": "string", "required": true, "readable": true, "writable": true},
    {"id": "channel", "name": "渠道", "type": "string", "readable": true, "writable": true,
     "enumValues": [{"id": "web", "name": "Web"}, {"id": "app", "name": "App"}]}
  ]
}`

// taskFormJSON 是任务表单定义。
const taskFormJSON = `{
  "key": "taskForm",
  "name": "Task Form",
  "fields": [
    {"id": "approved", "name": "通过", "type": "boolean", "required": true, "readable": true, "writable": true},
    {"id": "amount", "name": "金额", "type": "long", "readable": true, "writable": true},
    {"id": "note", "name": "备注", "type": "string", "readable": true, "writable": false},
    {"id": "channel", "name": "渠道", "type": "string", "readable": true, "writable": true,
     "enumValues": [{"id": "web", "name": "Web"}, {"id": "app", "name": "App"}]}
  ]
}`

// orderFormJSON 是列表测试使用的表单定义。
const orderFormJSON = `{
  "key": "orderForm",
  "name": "Order Form",
  "fields": [
    {"id": "amount", "name": "金额", "type": "long", "required": true, "readable": true, "writable": true,
     "enumValues": [{"id": "one", "name": "One"}]}
  ]
}`

// deployFormBundle 部署 BPMN 与表单资源压缩包。
// @param t 测试上下文
// @param engine 引擎
// @param resources 额外资源
func deployFormBundle(t *testing.T, engine *Engine, resources map[string][]byte) {
	t.Helper()
	entries := map[string][]byte{
		"processes/form-definition.bpmn20.xml": []byte(formDefinitionProcessXML),
	}
	for name, content := range resources {
		entries[name] = content
	}
	if _, err := engine.Deploy(context.Background(), api.DeploymentRequest{
		Name:      "form-bundle",
		Resources: map[string][]byte{"bundle.zip": makeZip(t, entries)},
	}); err != nil {
		t.Fatalf("部署表单资源失败: %v", err)
	}
}

// TestFormDefinitions_List 验证从 ZIP 部署读取表单定义列表。
// @param t 测试上下文
func TestFormDefinitions_List(t *testing.T) {
	engine := newTestEngine(t)
	deployFormBundle(t, engine, map[string][]byte{
		"forms/order.form":     []byte(orderFormJSON),
		"forms/secondary.form": []byte(`{"fields": []}`),
		"forms/readme.txt":     []byte("not a form"),
	})
	definition, err := engine.GetProcessDefinitionByKey(context.Background(), "formDefinitionProcess")
	if err != nil {
		t.Fatalf("查询流程定义失败: %v", err)
	}
	forms, err := engine.ListFormDefinitions(context.Background(), definition.ID)
	if err != nil {
		t.Fatalf("查询表单定义失败: %v", err)
	}
	if len(forms) != 2 {
		t.Fatalf("表单定义数量错误: %+v", forms)
	}
	// 按资源名排序，且非 .form 资源被忽略
	if forms[0].ResourceName != "forms/order.form" || forms[0].Key != "orderForm" || forms[0].Name != "Order Form" {
		t.Fatalf("第一个表单定义错误: %+v", forms[0])
	}
	if forms[0].DeploymentID != definition.DeploymentID || forms[0].TenantID != definition.TenantID {
		t.Fatalf("表单定义归属错误: %+v", forms[0])
	}
	if len(forms[0].Fields) != 1 {
		t.Fatalf("表单字段数量错误: %+v", forms[0].Fields)
	}
	field := forms[0].Fields[0]
	if field.ID != "amount" || field.Name != "金额" || field.Type != "long" || !field.Required || !field.Readable || !field.Writable {
		t.Fatalf("表单字段转换错误: %+v", field)
	}
	if len(field.EnumValues) != 1 || field.EnumValues[0].ID != "one" || field.EnumValues[0].Name != "One" {
		t.Fatalf("枚举值转换错误: %+v", field.EnumValues)
	}
	// Key 为空时使用文件名去扩展名
	if forms[1].ResourceName != "forms/secondary.form" || forms[1].Key != "secondary" || forms[1].Name != "" {
		t.Fatalf("文件名回退 Key 错误: %+v", forms[1])
	}
	if forms[1].Fields == nil || len(forms[1].Fields) != 0 {
		t.Fatalf("空字段应返回非 nil 空切片: %+v", forms[1].Fields)
	}
}

// TestFormDefinitions_ListEmptyAndNotFound 验证空列表与定义不存在。
// @param t 测试上下文
func TestFormDefinitions_ListEmptyAndNotFound(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	definition, err := engine.GetProcessDefinitionByKey(context.Background(), "simple")
	if err != nil {
		t.Fatalf("查询流程定义失败: %v", err)
	}
	forms, err := engine.ListFormDefinitions(context.Background(), definition.ID)
	if err != nil || forms == nil || len(forms) != 0 {
		t.Fatalf("无表单部署应返回空列表: %v %+v", err, forms)
	}
	if _, err := engine.ListFormDefinitions(context.Background(), "missing"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失流程定义应 404: %v", err)
	}
	// 定义指向不存在的部署时返回空列表而非 panic
	if forms, err := engine.formDefinitionsInDeployment("missing-deployment", ""); err != nil || forms == nil || len(forms) != 0 {
		t.Fatalf("缺失部署应返回空列表: %v %+v", err, forms)
	}
}

// TestFormDefinitions_Tenant 验证表单定义沿用流程定义租户。
// @param t 测试上下文
func TestFormDefinitions_Tenant(t *testing.T) {
	engine, err := New(Config{HistoryLevel: HistoryAudit, TenantID: "tenant-a"})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	t.Cleanup(func() { _ = engine.Close(context.Background()) })
	deployFormBundle(t, engine, map[string][]byte{"forms/order.form": []byte(orderFormJSON)})
	definition, err := engine.GetProcessDefinitionByKey(context.Background(), "formDefinitionProcess")
	if err != nil {
		t.Fatalf("查询流程定义失败: %v", err)
	}
	if definition.TenantID != "tenant-a" {
		t.Fatalf("流程定义租户错误: %+v", definition)
	}
	if _, err := engine.ListFormDefinitions(tenant.With(context.Background(), "tenant-b"), definition.ID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("跨租户应 404: %v", err)
	}
	forms, err := engine.ListFormDefinitions(tenant.With(context.Background(), "tenant-a"), definition.ID)
	if err != nil || len(forms) != 1 || forms[0].TenantID != "tenant-a" {
		t.Fatalf("同租户查询失败: %v %+v", err, forms)
	}
}

// TestFormDefinitions_ParseErrors 验证非法 .form 内容返回 400 错误。
// @param t 测试上下文
func TestFormDefinitions_ParseErrors(t *testing.T) {
	t.Run("invalid-json", func(t *testing.T) {
		engine := newTestEngine(t)
		deployFormBundle(t, engine, map[string][]byte{"forms/bad.form": []byte("{not-json")})
		definition, err := engine.GetProcessDefinitionByKey(context.Background(), "formDefinitionProcess")
		if err != nil {
			t.Fatalf("查询流程定义失败: %v", err)
		}
		if _, err := engine.ListFormDefinitions(context.Background(), definition.ID); !errors.Is(err, api.ErrBadRequest) || !strings.Contains(err.Error(), "ENGINE-1205") {
			t.Fatalf("非法 JSON 应返回 400 ENGINE-1205: %v", err)
		}
		if _, err := engine.GetStartFormData(context.Background(), definition.ID); !errors.Is(err, api.ErrBadRequest) {
			t.Fatalf("启动表单应返回解析错误: %v", err)
		}
		if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
			ProcessDefinitionKey: "formDefinitionProcess",
			Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
		}); err != nil {
			t.Fatalf("启动失败: %v", err)
		}
		task := listTasks(t, engine)[0]
		if _, err := engine.GetTaskFormData(context.Background(), task.ID); !errors.Is(err, api.ErrBadRequest) {
			t.Fatalf("任务表单应返回解析错误: %v", err)
		}
		if err := engine.SubmitTaskFormData(context.Background(), task.ID, map[string]any{}); !errors.Is(err, api.ErrBadRequest) {
			t.Fatalf("提交表单应返回解析错误: %v", err)
		}
	})
	t.Run("missing-fields", func(t *testing.T) {
		engine := newTestEngine(t)
		deployFormBundle(t, engine, map[string][]byte{"forms/empty.form": []byte(`{"key": "empty"}`)})
		definition, err := engine.GetProcessDefinitionByKey(context.Background(), "formDefinitionProcess")
		if err != nil {
			t.Fatalf("查询流程定义失败: %v", err)
		}
		if _, err := engine.ListFormDefinitions(context.Background(), definition.ID); !errors.Is(err, api.ErrBadRequest) || !strings.Contains(err.Error(), "ENGINE-1206") {
			t.Fatalf("缺少 fields 应返回 400 ENGINE-1206: %v", err)
		}
	})
}

// TestFormDefinitions_StartTaskHistoric 验证启动、任务与历史任务表单使用 .form 字段。
// @param t 测试上下文
func TestFormDefinitions_StartTaskHistoric(t *testing.T) {
	engine := newTestEngine(t)
	deployFormBundle(t, engine, map[string][]byte{
		"forms/start.form": []byte(startFormJSON),
		"forms/task.form":  []byte(taskFormJSON),
	})
	ctx := context.Background()
	definition, err := engine.GetProcessDefinitionByKey(ctx, "formDefinitionProcess")
	if err != nil {
		t.Fatalf("查询流程定义失败: %v", err)
	}
	startForm, err := engine.GetStartFormData(ctx, definition.ID)
	if err != nil {
		t.Fatalf("启动表单错误: %v", err)
	}
	if startForm.FormKey != "startForm" || len(startForm.Properties) != 2 {
		t.Fatalf("启动表单应使用 .form 字段: %+v", startForm)
	}
	if startForm.Properties[0].ID != "initiator" || !startForm.Properties[0].Required {
		t.Fatalf("启动表单字段属性错误: %+v", startForm.Properties[0])
	}
	if len(startForm.Properties[1].EnumValues) != 2 || startForm.Properties[1].EnumValues[1].ID != "app" {
		t.Fatalf("启动表单枚举错误: %+v", startForm.Properties[1])
	}
	if _, err := engine.StartProcessInstance(ctx, api.StartRequest{
		ProcessDefinitionKey: "formDefinitionProcess",
		Variables: []api.Variable{
			{Name: "initiator", Value: "alice"},
			{Name: "amount", Value: int64(7)},
			{Name: "note", Value: "seed"},
			{Name: "channel", Value: "app"},
		},
	}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	task := listTasks(t, engine)[0]
	form, err := engine.GetTaskFormData(ctx, task.ID)
	if err != nil {
		t.Fatalf("任务表单错误: %v", err)
	}
	if form.FormKey != "taskForm" || len(form.Properties) != 4 {
		t.Fatalf("任务表单应使用 .form 字段: %+v", form)
	}
	if form.Properties[0].ID != "approved" || !form.Properties[0].Required || form.Properties[0].Value != nil {
		t.Fatalf("布尔字段错误: %+v", form.Properties[0])
	}
	if form.Properties[1].Value != int64(7) || form.Properties[2].Value != "seed" || form.Properties[3].Value != "app" {
		t.Fatalf("任务表单变量值错误: %+v", form.Properties)
	}
	if form.Properties[2].Writable {
		t.Fatalf("只读字段不应可写: %+v", form.Properties[2])
	}
	historic, err := engine.GetHistoricTaskFormData(ctx, task.ID)
	if err != nil || historic.FormKey != "taskForm" || len(historic.Properties) != 4 {
		t.Fatalf("历史任务表单应使用 .form 字段: %v %+v", err, historic)
	}
	if historic.Properties[1].Value != int64(7) {
		t.Fatalf("历史任务表单变量值错误: %+v", historic.Properties[1])
	}
	if err := engine.SubmitTaskFormData(ctx, task.ID, map[string]any{"amount": "10"}); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("缺少必填字段应失败: %v", err)
	}
	if err := engine.SubmitTaskFormData(ctx, task.ID, map[string]any{"approved": true, "note": "x"}); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("提交只读字段应失败: %v", err)
	}
	if err := engine.SubmitTaskFormData(ctx, task.ID, map[string]any{"approved": true, "amount": "bad"}); err == nil {
		t.Fatalf("字段类型错误应失败")
	}
	if err := engine.SubmitTaskFormData(ctx, task.ID, map[string]any{"approved": true, "amount": "10", "channel": "web"}); err != nil {
		t.Fatalf("提交表单失败: %v", err)
	}
	variables, err := engine.GetVariables(ctx, task.ProcessInstanceID)
	if err != nil {
		t.Fatalf("查询变量失败: %v", err)
	}
	if variables["approved"] != true || variables["amount"] != int64(10) || variables["channel"] != "web" {
		t.Fatalf("表单提交变量错误: %+v", variables)
	}
	completed, err := engine.GetHistoricTaskFormData(ctx, task.ID)
	if err != nil || completed.FormKey != "taskForm" || len(completed.Properties) != 4 || completed.Properties[0].Value != true {
		t.Fatalf("完成后历史表单错误: %v %+v", err, completed)
	}
}

// TestFormDefinitions_FallbackToBPMN 验证同部署无匹配 .form 时回退 BPMN 字段。
// @param t 测试上下文
func TestFormDefinitions_FallbackToBPMN(t *testing.T) {
	engine := newTestEngine(t)
	if _, err := engine.Deploy(context.Background(), api.DeploymentRequest{
		Name: "fallback",
		Resources: map[string][]byte{"fallback.zip": makeZip(t, map[string][]byte{
			"processes/form.bpmn20.xml": []byte(formXML),
			"forms/other.form":          []byte(`{"key": "otherForm", "fields": []}`),
		})},
	}); err != nil {
		t.Fatalf("部署失败: %v", err)
	}
	definition, err := engine.GetProcessDefinitionByKey(context.Background(), "formProcess")
	if err != nil {
		t.Fatalf("查询流程定义失败: %v", err)
	}
	startForm, err := engine.GetStartFormData(context.Background(), definition.ID)
	if err != nil || startForm.FormKey != "startForm" || len(startForm.Properties) != 1 || startForm.Properties[0].ID != "initiator" {
		t.Fatalf("启动表单应回退 BPMN 字段: %v %+v", err, startForm)
	}
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "formProcess",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	task := listTasks(t, engine)[0]
	form, err := engine.GetTaskFormData(context.Background(), task.ID)
	if err != nil || form.FormKey != "taskForm" || len(form.Properties) != 3 || form.Properties[0].ID != "approved" {
		t.Fatalf("任务表单应回退 BPMN 字段: %v %+v", err, form)
	}
}

// TestFormDefinitions_NoFormKey 验证没有表单 Key 时沿用 BPMN 空字段行为。
// @param t 测试上下文
func TestFormDefinitions_NoFormKey(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	definition, err := engine.GetProcessDefinitionByKey(context.Background(), "simple")
	if err != nil {
		t.Fatalf("查询流程定义失败: %v", err)
	}
	form, err := engine.GetStartFormData(context.Background(), definition.ID)
	if err != nil || form.FormKey != "" || len(form.Properties) != 0 {
		t.Fatalf("无 FormKey 应返回空字段: %v %+v", err, form)
	}
}

// TestFormDefinitionHelpers 验证资源名与字段转换等辅助函数。
// @param t 测试上下文
func TestFormDefinitionHelpers(t *testing.T) {
	if !isFormResource("dir/A.FORM") {
		t.Fatalf("大写扩展名应识别为表单资源")
	}
	if isFormResource("dir/form.xml") {
		t.Fatalf("非 .form 资源不应识别为表单")
	}
	if key := formResourceKey("dir/order.form"); key != "order" {
		t.Fatalf("资源名去扩展名错误: %s", key)
	}
	if key := formResourceKey("dir/order"); key != "order" {
		t.Fatalf("无扩展名应原样返回: %s", key)
	}
	form, err := parseFormDefinition("dir/fallback.FORM", []byte(`{"key": "  ", "name": "N", "fields": []}`), "deployment", "tenant")
	if err != nil {
		t.Fatalf("空白 Key 应回退文件名: %v", err)
	}
	if form.Key != "fallback" || form.ResourceName != "dir/fallback.FORM" || form.TenantID != "tenant" || form.Fields == nil {
		t.Fatalf("表单定义解析错误: %+v", form)
	}
	properties := formDefinitionProperties([]api.FormProperty{{
		ID:         "amount",
		EnumValues: []api.FormValue{{ID: "one", Name: "One"}},
	}}, map[string]any{"amount": int64(3)})
	if len(properties) != 1 || properties[0].Value != int64(3) || len(properties[0].EnumValues) != 1 {
		t.Fatalf("表单字段值填充错误: %+v", properties)
	}
	if _, err := parseFormDefinition("bad.form", []byte("{"), "d", "t"); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("非法 JSON 解析应返回 400: %v", err)
	}
	if _, err := parseFormDefinition("bad.form", []byte(`{"fields": null}`), "d", "t"); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("null fields 应返回 400: %v", err)
	}
}
