package workflow

import (
	"context"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
	"github.com/DengQingNian/Flowtide/internal/engine"
	"github.com/DengQingNian/Flowtide/internal/idgen"
)

// metaFixture 是含会签/执行类型/步骤代号/子流程的测试语料。
const metaFixture = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="metaProc" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="apply"/>
    <userTask id="apply" name="申请" flowable:assignee="${initiator}" stepCode="S00"/>
    <sequenceFlow id="f1" sourceRef="apply" targetRef="gate"/>
    <exclusiveGateway id="gate" default="f3"/>
    <sequenceFlow id="f2" sourceRef="gate" targetRef="countersign">
      <conditionExpression>${pass == true}</conditionExpression>
    </sequenceFlow>
    <sequenceFlow id="f3" sourceRef="gate" targetRef="simple"/>
    <userTask id="countersign" name="会签" flowable:candidateUsers="u1,u2"
              stepCode="S01" executorType="person" executorNumType="multi"
              executorList='[{"identityId":"u1","agreeWeight":1,"rejectWeight":-1},{"identityId":"u2","agreeWeight":2,"rejectWeight":-2}]'
              weightType="custom" weightThreshold="60"
              weightList='[{"identityId":"u1","agreeWeight":1.5,"rejectWeight":-3}]'>
      <extensionElements>
        <flowable:executionListener event="start" class="countersignByWeight"/>
      </extensionElements>
      <multiInstanceLoopCharacteristics isSequential="false" flowable:collection="${executorList}" flowable:elementVariable="executor">
        <completionCondition>${totalWeight &gt;= weightThreshold}</completionCondition>
      </multiInstanceLoopCharacteristics>
    </userTask>
    <sequenceFlow id="f4" sourceRef="countersign" targetRef="end"/>
    <userTask id="simple" name="普通审批" stepCode="S02"/>
    <sequenceFlow id="f5" sourceRef="simple" targetRef="end"/>
    <subProcess id="sub" name="子流程">
      <startEvent id="subStart"/>
      <sequenceFlow id="sf0" sourceRef="subStart" targetRef="innerTask"/>
      <userTask id="innerTask" name="子流程内任务" stepCode="S10"/>
      <sequenceFlow id="sf1" sourceRef="innerTask" targetRef="subEnd"/>
      <endEvent id="subEnd"/>
    </subProcess>
    <sequenceFlow id="f6" sourceRef="simple" targetRef="sub"/>
    <sequenceFlow id="f7" sourceRef="sub" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// newMetaEngine 构造部署语料后的元数据测试环境。
// @param t 测试上下文
// @return 引擎
// @return 定义 ID
func newMetaEngine(t *testing.T) (api.Engine, string) {
	t.Helper()
	eng, err := engine.New(engine.Config{
		HistoryLevel: engine.HistoryAudit,
		Clock:        clock.NewFake(time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)),
		IDs:          idgen.NewUUIDv7(),
	})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close(context.Background()) })
	if _, err := eng.RepositoryService().Deploy(context.Background(), api.DeploymentRequest{
		Name:      "meta-test",
		Resources: map[string][]byte{"meta.bpmn20.xml": []byte(metaFixture)},
	}); err != nil {
		t.Fatalf("部署失败: %v", err)
	}
	definitions, err := eng.RepositoryService().ListProcessDefinitions(context.Background())
	if err != nil || len(definitions) == 0 {
		t.Fatalf("查询定义失败: %v", err)
	}
	return eng, definitions[0].ID
}

// TestUserTaskMeta 验证用户任务元数据提取（含会签与扩展属性）。
// @param t 测试上下文
func TestUserTaskMeta(t *testing.T) {
	eng, definitionID := newMetaEngine(t)
	service := NewModelMetaService(eng)
	meta, err := service.UserTaskMeta(context.Background(), definitionID, "countersign")
	if err != nil {
		t.Fatalf("查询元数据失败: %v", err)
	}
	if meta.StepCode != "S01" || meta.ExecutorType != "person" || meta.ExecutorNumType != "multi" {
		t.Fatalf("扩展属性错误: %+v", meta)
	}
	if meta.WeightType != "custom" || meta.WeightThreshold != "60" {
		t.Fatalf("权重属性错误: %+v", meta)
	}
	configs := ParseExecutorConfigs(meta.ExecutorListRaw)
	if len(configs) != 2 || configs[0].IdentityID != "u1" || configs[1].AgreeWeight != 2 {
		t.Fatalf("executorList 解析错误: %+v", configs)
	}
	weights := ParseWeightList(meta.WeightListRaw)
	if len(weights) != 1 || weights[0].AgreeWeight != 1.5 || weights[0].RejectWeight != -3 {
		t.Fatalf("weightList 解析错误: %+v", weights)
	}
	if meta.MultiInstance == nil || meta.MultiInstance.Sequential || meta.MultiInstance.Collection != "${executorList}" {
		t.Fatalf("多实例配置错误: %+v", meta.MultiInstance)
	}
	if meta.MultiInstance.ElementVariable != "executor" {
		t.Fatalf("元素变量错误: %q", meta.MultiInstance.ElementVariable)
	}
	if len(meta.ExecutionListenerClasses) != 1 || meta.ExecutionListenerClasses[0] != "countersignByWeight" {
		t.Fatalf("执行监听器名错误: %v", meta.ExecutionListenerClasses)
	}
}

// TestFirstUserTask 验证从开始事件穿网关找第一个用户任务。
// @param t 测试上下文
func TestFirstUserTask(t *testing.T) {
	eng, definitionID := newMetaEngine(t)
	service := NewModelMetaService(eng)
	meta, err := service.FirstUserTask(context.Background(), definitionID)
	if err != nil {
		t.Fatalf("查询首任务失败: %v", err)
	}
	if meta.ActivityID != "apply" || meta.StepCode != "S00" {
		t.Fatalf("首任务错误: %+v", meta)
	}
	if meta.Assignee != "${initiator}" {
		t.Fatalf("处理人表达式错误: %q", meta.Assignee)
	}
}

// TestUserTaskMetaInSubProcess 验证子流程内节点可达。
// @param t 测试上下文
func TestUserTaskMetaInSubProcess(t *testing.T) {
	eng, definitionID := newMetaEngine(t)
	service := NewModelMetaService(eng)
	meta, err := service.UserTaskMeta(context.Background(), definitionID, "innerTask")
	if err != nil {
		t.Fatalf("查询子流程任务失败: %v", err)
	}
	if meta.StepCode != "S10" {
		t.Fatalf("子流程任务元数据错误: %+v", meta)
	}
}

// TestListUserTaskMetas 验证按文档顺序列出全部用户任务。
// @param t 测试上下文
func TestListUserTaskMetas(t *testing.T) {
	eng, definitionID := newMetaEngine(t)
	service := NewModelMetaService(eng)
	metas, err := service.ListUserTaskMetas(context.Background(), definitionID)
	if err != nil {
		t.Fatalf("列出元数据失败: %v", err)
	}
	if len(metas) != 4 {
		t.Fatalf("用户任务数错误: %d", len(metas))
	}
	ids := []string{metas[0].ActivityID, metas[1].ActivityID, metas[2].ActivityID, metas[3].ActivityID}
	want := []string{"apply", "countersign", "simple", "innerTask"}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("顺序错误: got %v want %v", ids, want)
		}
	}
}

// TestNodeExtensionsOnFlow 验证连线扩展读取。
// @param t 测试上下文
func TestNodeExtensionsOnFlow(t *testing.T) {
	eng, definitionID := newMetaEngine(t)
	service := NewModelMetaService(eng)
	extensions, err := service.NodeExtensions(context.Background(), definitionID, "innerTask")
	if err != nil {
		t.Fatalf("查询扩展失败: %v", err)
	}
	if extensions == nil || extensions.Attr("stepCode") != "S10" {
		t.Fatalf("节点扩展错误: %+v", extensions)
	}
}

// TestUserTaskMetaErrors 验证错误路径。
// @param t 测试上下文
func TestUserTaskMetaErrors(t *testing.T) {
	eng, definitionID := newMetaEngine(t)
	service := NewModelMetaService(eng)
	if _, err := service.UserTaskMeta(context.Background(), definitionID, "gate"); err == nil {
		t.Fatalf("网关不是用户任务应报错")
	}
	if _, err := service.UserTaskMeta(context.Background(), definitionID, "nope"); err == nil {
		t.Fatalf("不存在节点应报错")
	}
	if _, err := service.NodeExtensions(context.Background(), definitionID, "nope"); err == nil {
		t.Fatalf("不存在元素应报错")
	}
}

// TestParseExecutorConfigsFallback 验证逗号串回退与空值处理。
// @param t 测试上下文
func TestParseExecutorConfigsFallback(t *testing.T) {
	configs := ParseExecutorConfigs("u1, u2 ,")
	if len(configs) != 2 || configs[0].IdentityID != "u1" || configs[1].ExecutorType != "person" {
		t.Fatalf("逗号串解析错误: %+v", configs)
	}
	if ParseExecutorConfigs("") != nil {
		t.Fatalf("空串应返回 nil")
	}
	if ParseExecutorConfigs("not-json[{") == nil || len(ParseExecutorConfigs("not-json[{")) != 1 {
		t.Fatalf("非法 JSON 应回退逗号拆分")
	}
	if ParseWeightList("") != nil || ParseWeightList("u1") != nil {
		t.Fatalf("weightList 非法输入应返回 nil")
	}
}
