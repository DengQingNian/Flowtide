package workflow

import (
	"context"
	"strings"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/bpmn/model"
	"github.com/DengQingNian/Flowtide/internal/clock"
	"github.com/DengQingNian/Flowtide/internal/engine"
	"github.com/DengQingNian/Flowtide/internal/idgen"
)

// 部署强校验语料（四种形态：合规 average / 合规 custom / 缺监听器 / 缺权重阈值）。

// dcAverageOK 是合规普通会签语料（weightType=average + 集合配置监听器）。
const dcAverageOK = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="dcAverageOK" name="合规会签" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="t1"/>
    <userTask id="t1" name="普通会签" flowable:assignee="${signer}" weightType="average"
              executorList='[{"identityId":"u1"}]'>
      <extensionElements>
        <flowable:executionListener event="start" class="countersignCollectionConfig"/>
      </extensionElements>
      <multiInstanceLoopCharacteristics isSequential="false" flowable:collection="executorList" flowable:elementVariable="signer"/>
    </userTask>
    <sequenceFlow id="f1" sourceRef="t1" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// dcAverageJavaClass 是以 Java 全限定类名声明监听器的语料（线上语料兼容形态）。
const dcAverageJavaClass = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="dcAverageJavaClass" name="Java类名会签" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="t1"/>
    <userTask id="t1" name="Java类名会签" flowable:assignee="${signer}" weightType="average"
              executorList='[{"identityId":"u1"}]'>
      <extensionElements>
        <flowable:executionListener event="start" class="example.workflow.listener.CountersignCollectionConfigExecutionListener"/>
      </extensionElements>
      <multiInstanceLoopCharacteristics isSequential="false" flowable:collection="executorList" flowable:elementVariable="signer"/>
    </userTask>
    <sequenceFlow id="f1" sourceRef="t1" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// dcWeightOK 是合规权重会签语料（weightType=custom + 权重监听器 + 阈值）。
const dcWeightOK = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="dcWeightOK" name="合规权重会签" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="t1"/>
    <userTask id="t1" name="权重会签" flowable:assignee="${signer}" weightType="custom" weightThreshold="60"
              executorList='[{"identityId":"u1"}]'>
      <extensionElements>
        <flowable:executionListener event="start" class="countersignByWeight"/>
      </extensionElements>
      <multiInstanceLoopCharacteristics isSequential="false" flowable:collection="executorList" flowable:elementVariable="signer"/>
    </userTask>
    <sequenceFlow id="f1" sourceRef="t1" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// dcMissingListener 是缺监听器语料（会签节点无 executionListener）。
const dcMissingListener = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="dcMissingListener" name="缺监听器" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="t1"/>
    <userTask id="t1" name="缺失监听任务" flowable:assignee="${signer}" weightType="average"
              executorList='[{"identityId":"u1"}]'>
      <multiInstanceLoopCharacteristics isSequential="false" flowable:collection="executorList" flowable:elementVariable="signer"/>
    </userTask>
    <sequenceFlow id="f1" sourceRef="t1" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// dcWrongListener 是监听器类型不匹配语料（weightType=custom 却配了集合监听器）。
const dcWrongListener = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="dcWrongListener" name="监听器不匹配" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="t1"/>
    <userTask id="t1" name="不匹配任务" flowable:assignee="${signer}" weightType="custom" weightThreshold="60"
              executorList='[{"identityId":"u1"}]'>
      <extensionElements>
        <flowable:executionListener event="start" class="countersignCollectionConfig"/>
      </extensionElements>
      <multiInstanceLoopCharacteristics isSequential="false" flowable:collection="executorList" flowable:elementVariable="signer"/>
    </userTask>
    <sequenceFlow id="f1" sourceRef="t1" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// dcWeightNoThreshold 是权重阈值缺失语料。
const dcWeightNoThreshold = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="dcWeightNoThreshold" name="缺阈值权重会签" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="t1"/>
    <userTask id="t1" name="缺阈值任务" flowable:assignee="${signer}" weightType="custom"
              executorList='[{"identityId":"u1"}]'>
      <extensionElements>
        <flowable:executionListener event="start" class="countersignByWeight"/>
      </extensionElements>
      <multiInstanceLoopCharacteristics isSequential="false" flowable:collection="executorList" flowable:elementVariable="signer"/>
    </userTask>
    <sequenceFlow id="f1" sourceRef="t1" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// dcWeightTypeMissing 是 weightType 缺失语料（Java 语义：命中标记不置位 → 报未配置监听器）。
const dcWeightTypeMissing = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="dcWeightTypeMissing" name="缺权重类型" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="t1"/>
    <userTask id="t1" name="缺类型任务" flowable:assignee="${signer}"
              executorList='[{"identityId":"u1"}]'>
      <extensionElements>
        <flowable:executionListener event="start" class="countersignCollectionConfig"/>
      </extensionElements>
      <multiInstanceLoopCharacteristics isSequential="false" flowable:collection="executorList" flowable:elementVariable="signer"/>
    </userTask>
    <sequenceFlow id="f1" sourceRef="t1" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// dcPlainTask 是无会签节点语料（不应触发校验）。
const dcPlainTask = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="dcPlainTask" name="普通流程" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="t1"/>
    <userTask id="t1" name="普通任务" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f1" sourceRef="t1" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// deployCheckFixture 是部署校验测试环境（会签服务 + 已部署语料的定义 ID 表）。
type deployCheckFixture struct {
	cs     *CountersignService       // 会签服务（校验经其元数据服务）
	defIDs map[string]string         // 定义 Key → 定义 ID
	defs   map[string]*model.Process // 定义 Key → 流程模型
}

// newDeployCheckFixture 构造部署校验测试环境。
// @param t 测试上下文
// @return 测试环境
func newDeployCheckFixture(t *testing.T) *deployCheckFixture {
	t.Helper()
	eng, err := engine.New(engine.Config{
		HistoryLevel: engine.HistoryAudit,
		Clock:        clock.NewFake(completeFixtureTime),
		IDs:          idgen.NewUUIDv7(),
		TenantID:     "dev",
	})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close(context.Background()) })
	if _, err := eng.RepositoryService().Deploy(context.Background(), api.DeploymentRequest{
		Name:     "deploy-check",
		TenantID: "dev",
		Resources: map[string][]byte{
			"average-ok.bpmn20.xml":          []byte(dcAverageOK),
			"average-java.bpmn20.xml":        []byte(dcAverageJavaClass),
			"weight-ok.bpmn20.xml":           []byte(dcWeightOK),
			"missing-listener.bpmn20.xml":    []byte(dcMissingListener),
			"wrong-listener.bpmn20.xml":      []byte(dcWrongListener),
			"weight-no-threshold.bpmn20.xml": []byte(dcWeightNoThreshold),
			"weight-type-missing.bpmn20.xml": []byte(dcWeightTypeMissing),
			"plain.bpmn20.xml":               []byte(dcPlainTask),
		},
	}); err != nil {
		t.Fatalf("部署失败: %v", err)
	}
	fx := &deployCheckFixture{cs: NewCountersignService(eng, NewModelMetaService(eng)), defIDs: map[string]string{}, defs: map[string]*model.Process{}}
	definitions, err := eng.RepositoryService().ListProcessDefinitions(context.Background())
	if err != nil {
		t.Fatalf("查询定义失败: %v", err)
	}
	for _, definition := range definitions {
		fx.defIDs[definition.Key] = definition.ID
		// 保留流程模型供 ValidateProcessIDMatch 断言
		bpmn, err := eng.RepositoryService().GetBpmnModel(context.Background(), definition.ID)
		if err == nil && len(bpmn.Processes) > 0 {
			fx.defs[definition.Key] = bpmn.Processes[0]
		}
	}
	return fx
}

// expectError 断言错误包含期望文案（公共辅助）。
// @param t 测试上下文
// @param err 实际错误
// @param contains 期望包含的文案
func expectError(t *testing.T, err error, contains string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), contains) {
		t.Fatalf("应报错含 %q: %v", contains, err)
	}
}

// TestDeployCheck_Compliant 验证合规语料（average/custom/Java 类名）通过校验。
// @param t 测试上下文
func TestDeployCheck_Compliant(t *testing.T) {
	fx := newDeployCheckFixture(t)
	ctx := context.Background()
	if err := fx.cs.ValidateCountersignConfig(ctx, fx.defIDs["dcAverageOK"]); err != nil {
		t.Fatalf("average 合规应通过: %v", err)
	}
	if err := fx.cs.ValidateCountersignConfig(ctx, fx.defIDs["dcAverageJavaClass"]); err != nil {
		t.Fatalf("Java 类名语料应通过: %v", err)
	}
	if err := fx.cs.ValidateCountersignConfig(ctx, fx.defIDs["dcWeightOK"]); err != nil {
		t.Fatalf("custom 合规应通过: %v", err)
	}
	// 无会签节点不触发校验
	if err := fx.cs.ValidateCountersignConfig(ctx, fx.defIDs["dcPlainTask"]); err != nil {
		t.Fatalf("无会签语料应通过: %v", err)
	}
}

// TestDeployCheck_MissingListener 验证缺监听器/类型不匹配/weightType 缺失报错。
// @param t 测试上下文
func TestDeployCheck_MissingListener(t *testing.T) {
	fx := newDeployCheckFixture(t)
	ctx := context.Background()
	expectError(t, fx.cs.ValidateCountersignConfig(ctx, fx.defIDs["dcMissingListener"]), "会签节点【缺失监听任务】未配置集合变量设置监听器")
	expectError(t, fx.cs.ValidateCountersignConfig(ctx, fx.defIDs["dcWrongListener"]), "会签节点【不匹配任务】未配置集合变量设置监听器")
	expectError(t, fx.cs.ValidateCountersignConfig(ctx, fx.defIDs["dcWeightTypeMissing"]), "会签节点【缺类型任务】未配置集合变量设置监听器")
}

// TestDeployCheck_WeightThresholdMissing 验证权重阈值缺失报错。
// @param t 测试上下文
func TestDeployCheck_WeightThresholdMissing(t *testing.T) {
	fx := newDeployCheckFixture(t)
	expectError(t, fx.cs.ValidateCountersignConfig(context.Background(), fx.defIDs["dcWeightNoThreshold"]), "会签节点【缺阈值任务】未设置会签权重阈值")
}

// TestDeployCheck_DefinitionMissing 验证定义不存在的错误透传。
// @param t 测试上下文
func TestDeployCheck_DefinitionMissing(t *testing.T) {
	fx := newDeployCheckFixture(t)
	expectError(t, fx.cs.ValidateCountersignConfig(context.Background(), "ghost-id"), "流程定义")
}

// TestValidateProcessIDMatch 验证流程 id 与 Key 一致性校验（对位 Java 导入校验段）。
// @param t 测试上下文
func TestValidateProcessIDMatch(t *testing.T) {
	fx := newDeployCheckFixture(t)
	// 一致通过
	if err := ValidateProcessIDMatch(fx.defs["dcAverageOK"], "dcAverageOK"); err != nil {
		t.Fatalf("一致应通过: %v", err)
	}
	// 不一致
	expectError(t, ValidateProcessIDMatch(fx.defs["dcAverageOK"], "other-key"), "导入流程与当前流程不匹配，请检查后重试。")
	// id 为空
	expectError(t, ValidateProcessIDMatch(&model.Process{ID: ""}, "any"), "流程定义key为空")
	// 模型缺失
	expectError(t, ValidateProcessIDMatch(nil, "any"), "流程模型不存在")
}
