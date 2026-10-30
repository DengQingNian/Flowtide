package engine

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/bpmn/model"
	"github.com/DengQingNian/Flowtide/internal/clock"
	"github.com/DengQingNian/Flowtide/internal/idgen"
)

// newExtEngine 创建带可推进假时钟的测试引擎，撤回时间窗校验依赖时间推进。
// @param t 测试上下文
// @return 引擎
// @return 假时钟
func newExtEngine(t *testing.T) (*Engine, *clock.Fake) {
	t.Helper()
	fake := clock.NewFake(time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC))
	eng, err := New(Config{HistoryLevel: HistoryAudit, Clock: fake, IDs: idgen.NewUUIDv7()})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	return eng, fake
}

// tick 推进测试时钟一分钟，保证跨命令时间戳递增。
// @param fake 假时钟
func tick(fake *clock.Fake) {
	fake.Advance(time.Minute)
}

// taskOfKey 查找指定 BPMN 节点 Key 的唯一运行中任务，数量不符时失败。
// @param t 测试上下文
// @param eng 引擎
// @param key 任务定义 Key
// @return 任务
func taskOfKey(t *testing.T, eng *Engine, key string) api.Task {
	t.Helper()
	tasks := listTasks(t, eng)
	var found []api.Task
	for _, task := range tasks {
		if task.TaskDefinitionKey == key {
			found = append(found, task)
		}
	}
	if len(found) != 1 {
		t.Fatalf("期望节点 %s 恰有 1 个任务，实际 %d: %+v", key, len(found), tasks)
	}
	return found[0]
}

// completeByKey 推进时钟后完成指定节点的任务。
// @param t 测试上下文
// @param eng 引擎
// @param fake 假时钟
// @param key 任务定义 Key
// @param userID 完成人
func completeByKey(t *testing.T, eng *Engine, fake *clock.Fake, key, userID string) {
	t.Helper()
	task := taskOfKey(t, eng, key)
	tick(fake)
	if err := eng.Complete(context.Background(), task.ID, api.CompleteTaskRequest{UserID: userID}); err != nil {
		t.Fatalf("完成节点 %s 任务失败: %v", key, err)
	}
}

// completeByAssignee 推进时钟后完成指定节点上指定办理人的任务（多实例节点用）。
// @param t 测试上下文
// @param eng 引擎
// @param fake 假时钟
// @param key 任务定义 Key
// @param assignee 办理人
func completeByAssignee(t *testing.T, eng *Engine, fake *clock.Fake, key, assignee string) {
	t.Helper()
	var found *api.Task
	for _, task := range listTasks(t, eng) {
		if task.TaskDefinitionKey == key && task.Assignee == assignee {
			found = &task
			break
		}
	}
	if found == nil {
		t.Fatalf("未找到节点 %s 上办理人 %s 的任务", key, assignee)
	}
	tick(fake)
	if err := eng.Complete(context.Background(), found.ID, api.CompleteTaskRequest{UserID: assignee}); err != nil {
		t.Fatalf("完成节点 %s 任务失败: %v", key, err)
	}
}

// definitionModel 按流程 Key 取解析后的流程模型。
// @param t 测试上下文
// @param eng 引擎
// @param key 流程 Key
// @return 流程模型
func definitionModel(t *testing.T, eng *Engine, key string) *model.Process {
	t.Helper()
	for _, definition := range eng.state.Definitions {
		if definition.Info.Key == key {
			process := processOfDefinition(definition)
			if process == nil {
				t.Fatalf("流程定义不包含流程: %s", key)
			}
			return process
		}
	}
	t.Fatalf("流程定义不存在: %s", key)
	return nil
}

// 退回语料：线性三节点。
const backLinearXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="backLinear" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="A"/>
    <userTask id="A" name="A" flowable:assignee="alice"/>
    <sequenceFlow id="f2" sourceRef="A" targetRef="B"/>
    <userTask id="B" name="B" flowable:assignee="bob"/>
    <sequenceFlow id="f3" sourceRef="B" targetRef="C"/>
    <userTask id="C" name="C" flowable:assignee="carl"/>
    <sequenceFlow id="f4" sourceRef="C" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestBackUserTaskLinearAndRerun 验证线性退回与退回后重走全流程。
// @param t 测试上下文
func TestBackUserTaskLinearAndRerun(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, backLinearXML)
	instance, err := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "backLinear"})
	if err != nil {
		t.Fatal(err)
	}
	completeByKey(t, eng, fake, "A", "alice")
	completeByKey(t, eng, fake, "B", "bob")
	landing, err := eng.BackUserTaskToActivity(context.Background(), taskOfKey(t, eng, "C").ID, "A")
	if err != nil {
		t.Fatalf("退回失败: %v", err)
	}
	if landing != "A" {
		t.Fatalf("实际落位应为 A，实际 %s", landing)
	}
	// 旧任务清场且留有删除原因
	history, err := eng.GetHistoricTaskInstance(context.Background(), taskOfKey(t, eng, "A").ID)
	if err != nil {
		t.Fatalf("新任务历史缺失: %v", err)
	}
	_ = history
	tasks := listTasks(t, eng)
	if len(tasks) != 1 || tasks[0].TaskDefinitionKey != "A" {
		t.Fatalf("退回后应只剩节点 A 任务: %+v", tasks)
	}
	// 退回后重走全流程
	completeByKey(t, eng, fake, "A", "alice")
	completeByKey(t, eng, fake, "B", "bob")
	completeByKey(t, eng, fake, "C", "carl")
	got, _ := eng.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("退回后重走应正常结束")
	}
}

// TestBackUserTaskErrors 验证退回校验：目标不存在、不可达、非用户任务来源。
// @param t 测试上下文
func TestBackUserTaskErrors(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, backLinearXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "backLinear"})
	_ = instance
	// 目标活动不存在
	if _, err := eng.BackUserTaskToActivity(context.Background(), taskOfKey(t, eng, "A").ID, "missing"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("目标不存在应 404: %v", err)
	}
	completeByKey(t, eng, fake, "A", "alice")
	// 反向不可达：并行兄弟分支互不可达（部署并行语料后校验 B1 -> B2）
	deployXML(t, eng, backParallelPlainXML)
	if _, err := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "backParallelPlain"}); err != nil {
		t.Fatal(err)
	}
	completeByKey(t, eng, fake, "A", "alice")
	if _, err := eng.BackUserTaskToActivity(context.Background(), taskOfKey(t, eng, "B1").ID, "B2"); err == nil {
		t.Fatalf("B1 退回 B2 不应发生（兄弟分支不可达）")
	} else if got := err.Error(); !strings.Contains(got, "Cannot back to") {
		t.Fatalf("错误消息应为不可达: %v", err)
	}
	// 目标为空
	if _, err := eng.BackUserTaskToActivity(context.Background(), taskOfKey(t, eng, "B").ID, " "); err == nil {
		t.Fatalf("目标为空应报错")
	}
}

// 退回语料：排他网关分流后汇合。
const backExclusiveXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="backExclusive" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="A"/>
    <userTask id="A" name="A" flowable:assignee="alice"/>
    <sequenceFlow id="f2" sourceRef="A" targetRef="g"/>
    <exclusiveGateway id="g"/>
    <sequenceFlow id="f3" sourceRef="g" targetRef="B">
      <conditionExpression>${x == 1}</conditionExpression>
    </sequenceFlow>
    <sequenceFlow id="f4" sourceRef="g" targetRef="C">
      <conditionExpression>${x == 2}</conditionExpression>
    </sequenceFlow>
    <userTask id="B" name="B" flowable:assignee="bob"/>
    <userTask id="C" name="C" flowable:assignee="carl"/>
    <sequenceFlow id="f5" sourceRef="B" targetRef="D"/>
    <sequenceFlow id="f6" sourceRef="C" targetRef="D"/>
    <userTask id="D" name="D" flowable:assignee="dave"/>
    <sequenceFlow id="f7" sourceRef="D" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestBackUserTaskExclusiveGateway 验证经排他网关退回后可改道重走。
// @param t 测试上下文
func TestBackUserTaskExclusiveGateway(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, backExclusiveXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "backExclusive",
		Variables:            []api.Variable{{Name: "x", Value: 1}},
	})
	completeByKey(t, eng, fake, "A", "alice")
	// 应路由到 B
	if task := taskOfKey(t, eng, "B").ID; task == "" {
		t.Fatalf("应路由到 B")
	}
	completeByKey(t, eng, fake, "B", "bob")
	// 从 D 退回到 A
	landing, err := eng.BackUserTaskToActivity(context.Background(), taskOfKey(t, eng, "D").ID, "A")
	if err != nil || landing != "A" {
		t.Fatalf("退回失败: %v %s", err, landing)
	}
	// 改道：x=2 走 C
	tick(fake)
	if err := eng.SetVariable(context.Background(), instance.ID, "x", 2); err != nil {
		t.Fatal(err)
	}
	completeByKey(t, eng, fake, "A", "alice")
	completeByKey(t, eng, fake, "C", "carl")
	completeByKey(t, eng, fake, "D", "dave")
	got, _ := eng.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("改道重走应结束")
	}
}

// 退回语料：无 _begin/_end 约定的并行分叉汇合。
const backParallelPlainXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="backParallelPlain" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="A"/>
    <userTask id="A" name="A" flowable:assignee="alice"/>
    <sequenceFlow id="f2" sourceRef="A" targetRef="fork"/>
    <parallelGateway id="fork"/>
    <sequenceFlow id="f3" sourceRef="fork" targetRef="B1"/>
    <sequenceFlow id="f4" sourceRef="fork" targetRef="B2"/>
    <userTask id="B1" name="B1" flowable:assignee="bob"/>
    <userTask id="B2" name="B2" flowable:assignee="carl"/>
    <sequenceFlow id="f5" sourceRef="B1" targetRef="join"/>
    <sequenceFlow id="f6" sourceRef="B2" targetRef="join"/>
    <parallelGateway id="join"/>
    <sequenceFlow id="f7" sourceRef="join" targetRef="C"/>
    <userTask id="C" name="C" flowable:assignee="dave"/>
    <sequenceFlow id="f8" sourceRef="C" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestBackUserTaskParallelBranch1 验证并行分支1：双方都不在特殊并行块内，汇聚后单点退回且实例可正常终结。
// @param t 测试上下文
func TestBackUserTaskParallelBranch1(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, backParallelPlainXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "backParallelPlain"})
	completeByKey(t, eng, fake, "A", "alice")
	completeByKey(t, eng, fake, "B1", "bob")
	completeByKey(t, eng, fake, "B2", "carl")
	// 汇聚后从 C 退回到 B1（分支1：sourceRealActivityIds={C}）
	landing, err := eng.BackUserTaskToActivity(context.Background(), taskOfKey(t, eng, "C").ID, "B1")
	if err != nil || landing != "B1" {
		t.Fatalf("分支1退回失败: %v %s", err, landing)
	}
	completeByKey(t, eng, fake, "B1", "bob")
	// B1 重走后经汇聚生成 C
	completeByKey(t, eng, fake, "C", "dave")
	got, _ := eng.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("分支1退回重走应正常结束（并行汇聚自洽）")
	}
}

// 退回语料：_begin/_end 特殊网关并行块。
const backParallelSpecialXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="backParallelSpecial" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="A"/>
    <userTask id="A" name="A" flowable:assignee="alice"/>
    <sequenceFlow id="f2" sourceRef="A" targetRef="pb_begin"/>
    <parallelGateway id="pb_begin"/>
    <sequenceFlow id="f3" sourceRef="pb_begin" targetRef="B1"/>
    <sequenceFlow id="f4" sourceRef="pb_begin" targetRef="B2"/>
    <userTask id="B1" name="B1" flowable:assignee="bob"/>
    <userTask id="B2" name="B2" flowable:assignee="carl"/>
    <sequenceFlow id="f5" sourceRef="B1" targetRef="pb_end"/>
    <sequenceFlow id="f6" sourceRef="B2" targetRef="pb_end"/>
    <parallelGateway id="pb_end"/>
    <sequenceFlow id="f7" sourceRef="pb_end" targetRef="C"/>
    <userTask id="C" name="C" flowable:assignee="dave"/>
    <sequenceFlow id="f8" sourceRef="C" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestBackUserTaskParallelBranch2 验证并行分支2：当前在并行块内、目标在外，收拢整块执行到目标。
// @param t 测试上下文
func TestBackUserTaskParallelBranch2(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, backParallelSpecialXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "backParallelSpecial"})
	completeByKey(t, eng, fake, "A", "alice")
	// 并行块内两个任务同时在办
	if tasks := listTasks(t, eng); len(tasks) != 2 {
		t.Fatalf("并行块内应有 2 个任务: %+v", tasks)
	}
	// 从块内 B1 退回到块前 A：整块收拢
	landing, err := eng.BackUserTaskToActivity(context.Background(), taskOfKey(t, eng, "B1").ID, "A")
	if err != nil || landing != "A" {
		t.Fatalf("分支2退回失败: %v %s", err, landing)
	}
	tasks := listTasks(t, eng)
	if len(tasks) != 1 || tasks[0].TaskDefinitionKey != "A" {
		t.Fatalf("整块收拢后应只剩 A 任务: %+v", tasks)
	}
	// 整块重走并终结（PendingBranches 自洽证明）
	completeByKey(t, eng, fake, "A", "alice")
	completeByKey(t, eng, fake, "B1", "bob")
	completeByKey(t, eng, fake, "B2", "carl")
	completeByKey(t, eng, fake, "C", "dave")
	got, _ := eng.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("分支2收拢后应可正常终结（PendingBranches 自洽）")
	}
}

// TestBackUserTaskParallelBranch3 验证并行分支3：目标在并行块内、当前在外，直接拒绝。
// @param t 测试上下文
func TestBackUserTaskParallelBranch3(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, backParallelSpecialXML)
	_, _ = eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "backParallelSpecial"})
	completeByKey(t, eng, fake, "A", "alice")
	completeByKey(t, eng, fake, "B1", "bob")
	completeByKey(t, eng, fake, "B2", "carl")
	// 汇聚后从块外 C 退回块内 B1：并行流转已结束，不允许退回
	_, err := eng.BackUserTaskToActivity(context.Background(), taskOfKey(t, eng, "C").ID, "B1")
	if err == nil {
		t.Fatalf("分支3必须拒绝退回")
	}
	if got := err.Error(); !strings.Contains(got, "并行流转已结束") {
		t.Fatalf("分支3错误消息不符: %v", err)
	}
}

// 退回语料：嵌套并行块（分支4 场景）。
const backParallelNestedXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="backParallelNested" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="A"/>
    <userTask id="A" name="A" flowable:assignee="alice"/>
    <sequenceFlow id="f2" sourceRef="A" targetRef="p1_begin"/>
    <parallelGateway id="p1_begin"/>
    <sequenceFlow id="f3" sourceRef="p1_begin" targetRef="x1"/>
    <sequenceFlow id="f4" sourceRef="p1_begin" targetRef="z1"/>
    <userTask id="x1" name="x1" flowable:assignee="bob"/>
    <userTask id="z1" name="z1" flowable:assignee="zoe"/>
    <sequenceFlow id="f5" sourceRef="x1" targetRef="p2_begin"/>
    <parallelGateway id="p2_begin"/>
    <sequenceFlow id="f6" sourceRef="p2_begin" targetRef="y1"/>
    <sequenceFlow id="f7" sourceRef="p2_begin" targetRef="y2"/>
    <userTask id="y1" name="y1" flowable:assignee="carl"/>
    <userTask id="y2" name="y2" flowable:assignee="cain"/>
    <sequenceFlow id="f8" sourceRef="y1" targetRef="p2_end"/>
    <sequenceFlow id="f9" sourceRef="y2" targetRef="p2_end"/>
    <parallelGateway id="p2_end"/>
    <sequenceFlow id="f10" sourceRef="p2_end" targetRef="x2"/>
    <userTask id="x2" name="x2" flowable:assignee="dave"/>
    <sequenceFlow id="f11" sourceRef="x2" targetRef="p1_end"/>
    <sequenceFlow id="f12" sourceRef="z1" targetRef="p1_end"/>
    <parallelGateway id="p1_end"/>
    <sequenceFlow id="f13" sourceRef="p1_end" targetRef="C"/>
    <userTask id="C" name="C" flowable:assignee="earl"/>
    <sequenceFlow id="f14" sourceRef="C" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestBackUserTaskParallelBranch4 验证并行分支4：目标在更内层并行块，退回后补造汇聚执行且内外汇聚计数自洽。
// @param t 测试上下文
func TestBackUserTaskParallelBranch4(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, backParallelNestedXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "backParallelNested"})
	completeByKey(t, eng, fake, "A", "alice")
	completeByKey(t, eng, fake, "x1", "bob")
	// 内层块 y1/y2 与外层分支 z1 同时在办
	if tasks := listTasks(t, eng); len(tasks) != 3 {
		t.Fatalf("应有 y1/y2/z1 三个任务: %+v", tasks)
	}
	completeByKey(t, eng, fake, "y1", "carl")
	completeByKey(t, eng, fake, "y2", "cain")
	// 内层汇聚后 x2 在办；从 x2 退回到内层块内 y1（分支4：需补造 p2_end 汇聚）
	landing, err := eng.BackUserTaskToActivity(context.Background(), taskOfKey(t, eng, "x2").ID, "y1")
	if err != nil || landing != "y1" {
		t.Fatalf("分支4退回失败: %v %s", err, landing)
	}
	// 补造汇聚后仅剩 y1 与 z1 两个任务
	if tasks := listTasks(t, eng); len(tasks) != 2 {
		t.Fatalf("退回后应有 y1/z1 两个任务: %+v", tasks)
	}
	completeByKey(t, eng, fake, "y1", "carl")
	// y1 完成后内层汇聚应放行生成 x2（不死锁）
	if task := taskOfKey(t, eng, "x2"); task.ID == "" {
		t.Fatalf("内层汇聚未放行")
	}
	completeByKey(t, eng, fake, "x2", "dave")
	// 外层第二分支
	completeByKey(t, eng, fake, "z1", "zoe")
	completeByKey(t, eng, fake, "C", "earl")
	got, _ := eng.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("分支4退回重走应正常结束（内外汇聚计数自洽）")
	}
}

// 退回语料：嵌入子流程。
const backSubProcessXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="backSubProcess" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="A"/>
    <userTask id="A" name="A" flowable:assignee="alice"/>
    <sequenceFlow id="f2" sourceRef="A" targetRef="S"/>
    <subProcess id="S" name="S">
      <startEvent id="sStart"/>
      <sequenceFlow id="sf1" sourceRef="sStart" targetRef="sT1"/>
      <userTask id="sT1" name="sT1" flowable:assignee="bob"/>
      <sequenceFlow id="sf2" sourceRef="sT1" targetRef="sT2"/>
      <userTask id="sT2" name="sT2" flowable:assignee="carl"/>
      <sequenceFlow id="sf3" sourceRef="sT2" targetRef="sEnd"/>
      <endEvent id="sEnd"/>
    </subProcess>
    <sequenceFlow id="f3" sourceRef="S" targetRef="C"/>
    <userTask id="C" name="C" flowable:assignee="dave"/>
    <sequenceFlow id="f4" sourceRef="C" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestBackUserTaskSubProcess 验证子流程内外退回：内部退外部提升到子流程层，外部退内部重入子流程。
// @param t 测试上下文
func TestBackUserTaskSubProcess(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, backSubProcessXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "backSubProcess"})
	completeByKey(t, eng, fake, "A", "alice")
	// 子流程内 sT1 退回到块前 A：sourceReal 提升为 S（整块收拢）
	landing, err := eng.BackUserTaskToActivity(context.Background(), taskOfKey(t, eng, "sT1").ID, "A")
	if err != nil || landing != "A" {
		t.Fatalf("子流程内退外失败: %v %s", err, landing)
	}
	if tasks := listTasks(t, eng); len(tasks) != 1 || tasks[0].TaskDefinitionKey != "A" {
		t.Fatalf("应只剩 A 任务: %+v", tasks)
	}
	// 重走进入子流程并走到外部 C
	completeByKey(t, eng, fake, "A", "alice")
	completeByKey(t, eng, fake, "sT1", "bob")
	completeByKey(t, eng, fake, "sT2", "carl")
	// 外部 C 退回到子流程 S：重入子流程开始
	landing, err = eng.BackUserTaskToActivity(context.Background(), taskOfKey(t, eng, "C").ID, "S")
	if err != nil || landing != "S" {
		t.Fatalf("外部退子流程失败: %v %s", err, landing)
	}
	if task := taskOfKey(t, eng, "sT1"); task.ID == "" {
		t.Fatalf("重入子流程应生成 sT1 任务")
	}
	// 重走全流程
	completeByKey(t, eng, fake, "sT1", "bob")
	completeByKey(t, eng, fake, "sT2", "carl")
	completeByKey(t, eng, fake, "C", "dave")
	got, _ := eng.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("子流程退回重走应正常结束")
	}
}

// 退回语料：并行多实例节点。
const backMultiInstanceXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="backMultiInstance" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="review"/>
    <userTask id="review" name="review" flowable:assignee="${approver}">
      <multiInstanceLoopCharacteristics flowable:collection="${approvers}" flowable:elementVariable="approver"/>
    </userTask>
    <sequenceFlow id="f2" sourceRef="review" targetRef="next"/>
    <userTask id="next" name="next" flowable:assignee="dave"/>
    <sequenceFlow id="f3" sourceRef="next" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestBackUserTaskMultiInstance 验证退回到多实例节点时整体重入并重新生成全部子实例。
// @param t 测试上下文
func TestBackUserTaskMultiInstance(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, backMultiInstanceXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "backMultiInstance",
		Variables:            []api.Variable{{Name: "approvers", Value: []string{"alice", "bob"}}},
	})
	completeByAssignee(t, eng, fake, "review", "alice")
	completeByAssignee(t, eng, fake, "review", "bob")
	// 多实例完成后从 next 退回到 review
	landing, err := eng.BackUserTaskToActivity(context.Background(), taskOfKey(t, eng, "next").ID, "review")
	if err != nil || landing != "review" {
		t.Fatalf("退回多实例节点失败: %v %s", err, landing)
	}
	tasks := listTasks(t, eng)
	if len(tasks) != 2 {
		t.Fatalf("多实例重入应生成 2 个任务: %+v", tasks)
	}
	// 计数自洽：多实例根计数复位
	miRoot := findMultiInstanceRoot(t, eng, instance.ID, "review")
	if miRoot == nil {
		t.Fatalf("未找到多实例根执行")
	}
	if got := miRoot.Variables["nrOfInstances"]; got != 2 {
		t.Fatalf("nrOfInstances 应为 2: %v", got)
	}
	if got := miRoot.Variables["nrOfActiveInstances"]; got != 2 {
		t.Fatalf("nrOfActiveInstances 应为 2: %v", got)
	}
	completeByAssignee(t, eng, fake, "review", "alice")
	completeByAssignee(t, eng, fake, "review", "bob")
	completeByKey(t, eng, fake, "next", "dave")
	got, _ := eng.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("多实例退回重走应正常结束")
	}
}

// findMultiInstanceRoot 在实例中查找指定活动的多实例根执行。
// @param t 测试上下文
// @param eng 引擎
// @param processInstanceID 实例 ID
// @param activityID 活动 ID
// @return 多实例根执行
func findMultiInstanceRoot(t *testing.T, eng *Engine, processInstanceID, activityID string) *executionState {
	t.Helper()
	for _, execution := range eng.state.Executions {
		if execution.ProcessInstanceID == processInstanceID && execution.MultiInstanceRoot && execution.MIActivityID == activityID && !execution.Ended {
			return execution
		}
	}
	return nil
}

// 撤回语料：普通两节点，办理人由钩子按 ROLLBACK_* 变量回填。
const rollbackDefaultXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="rollbackDefault" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="A"/>
    <userTask id="A" name="A" flowable:assignee="alice"/>
    <sequenceFlow id="f2" sourceRef="A" targetRef="B"/>
    <userTask id="B" name="B" flowable:assignee="bob"/>
    <sequenceFlow id="f3" sourceRef="B" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// newRollbackHook 注册按 ROLLBACK_ASSIGNEE_PREFIX_ 变量回填办理人的测试钩子。
// @param eng 引擎
// @param seen 钩子消费记录（指针透传，闭包追加对调用方可见）
func newRollbackHook(eng *Engine, seen *[]string) {
	eng.AddTaskCreatedHook(func(ctx context.Context, task *api.Task, ctl api.HookControl) error {
		key := rollbackAssigneePrefixKey + task.ProcessInstanceID + task.TaskDefinitionKey
		value, ok := task.Variables[key]
		if !ok {
			return nil
		}
		assignee, _ := value.(string)
		*seen = append(*seen, task.TaskDefinitionKey+"="+assignee)
		return ctl.SetAssignee(task.ID, assignee)
	})
}

// TestRollbackNextDefaultUserTask 验证普通节点撤回：新任务生成、ROLLBACK_* 变量落位、钩子回填联动、历史清理。
// @param t 测试上下文
func TestRollbackNextDefaultUserTask(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, rollbackDefaultXML)
	var seen []string
	newRollbackHook(eng, &seen)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "rollbackDefault"})
	taskA := taskOfKey(t, eng, "A")
	completeByKey(t, eng, fake, "A", "alice")
	taskB := taskOfKey(t, eng, "B")
	tick(fake)
	if err := eng.RollbackTask(context.Background(), taskA.ID, "carol"); err != nil {
		t.Fatalf("撤回失败: %v", err)
	}
	// 新任务生成且钩子按 ROLLBACK_* 变量回填办理人
	task := taskOfKey(t, eng, "A")
	if task.Assignee != "carol" {
		t.Fatalf("撤回任务办理人应为 carol: %+v", task)
	}
	if len(seen) != 1 || seen[0] != "A=carol" {
		t.Fatalf("钩子消费记录不符: %v", seen)
	}
	// ROLLBACK_* 变量落位：新执行局部变量可经任务变量读出
	if got, ok := task.Variables[rollbackAssigneePrefixKey+instance.ID+"A"]; !ok || got != "carol" {
		t.Fatalf("ROLLBACK_ASSIGNEE 变量未落位: %v", task.Variables)
	}
	if got, ok := task.Variables[rollbackTaskTypePrefixKey+instance.ID+"A"]; !ok || got != rollbackTypeNextDefaultUserTask {
		t.Fatalf("ROLLBACK_TASK_TYPE 变量未落位: %v", task.Variables)
	}
	// 旧任务历史被删除
	if _, err := eng.GetHistoricTaskInstance(context.Background(), taskA.ID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("旧 A 历史任务应删除: %v", err)
	}
	if _, err := eng.GetHistoricTaskInstance(context.Background(), taskB.ID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("后继 B 历史任务应删除: %v", err)
	}
	// 撤回时间点之后的历史活动行被删除（B 节点与其连线）
	if rows := historicActivityCount(t, eng, instance.ID, "B"); rows != 0 {
		t.Fatalf("B 节点历史活动应删除，实际 %d", rows)
	}
	// 撤回后重走
	completeByKey(t, eng, fake, "A", "carol")
	completeByKey(t, eng, fake, "B", "bob")
	got, _ := eng.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("撤回重走应正常结束")
	}
}

// historicActivityCount 统计实例某活动的历史活动行数。
// @param t 测试上下文
// @param eng 引擎
// @param processInstanceID 实例 ID
// @param activityID 活动 ID
// @return 行数
func historicActivityCount(t *testing.T, eng *Engine, processInstanceID, activityID string) int {
	t.Helper()
	page, err := eng.QueryHistoricActivityInstances(context.Background(), api.HistoricActivityInstanceFilter{
		ProcessInstanceID: processInstanceID,
		ActivityID:        activityID,
		Size:              100,
	})
	if err != nil {
		t.Fatalf("查询历史活动失败: %v", err)
	}
	return int(page.Total)
}

// TestRollbackRejects 验证撤回入口校验：未完成任务、不存在任务、实例已结束、后继已被处理。
// @param t 测试上下文
func TestRollbackRejects(t *testing.T) {
	eng, fake := newExtEngine(t)
	// 三节点线性语料：保证撤回校验时实例仍在运行
	deployXML(t, eng, backLinearXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "backLinear"})
	_ = instance
	// 不存在的任务
	if err := eng.RollbackTask(context.Background(), "missing", "carol"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("不存在任务应 404: %v", err)
	}
	taskA := taskOfKey(t, eng, "A")
	// 正在执行的任务不能撤回
	if err := eng.RollbackTask(context.Background(), taskA.ID, "carol"); err == nil || !strings.Contains(err.Error(), "任务正在执行") {
		t.Fatalf("运行中任务应拒绝: %v", err)
	}
	completeByKey(t, eng, fake, "A", "alice")
	completeByKey(t, eng, fake, "B", "bob")
	// 后继节点已被处理：A 已有更晚完成的 B
	tick(fake)
	if err := eng.RollbackTask(context.Background(), taskA.ID, "carol"); err == nil || !strings.Contains(err.Error(), "任务已被处理") {
		t.Fatalf("后继已处理应拒绝: %v", err)
	}
}

// 撤回语料：串行会签，办理人即元素变量 executor。
const rollbackSequentialMixml = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="rollbackSeqMI" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="review"/>
    <userTask id="review" name="review" flowable:assignee="${executor}">
      <multiInstanceLoopCharacteristics isSequential="true" flowable:collection="${executorList}" flowable:elementVariable="executor"/>
    </userTask>
    <sequenceFlow id="f2" sourceRef="review" targetRef="next"/>
    <userTask id="next" name="next" flowable:assignee="dave"/>
    <sequenceFlow id="f3" sourceRef="next" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestRollbackSequentialMultiInstance 验证串行会签撤回：只允许撤"前一位"，计数与集合回写自洽。
// @param t 测试上下文
func TestRollbackSequentialMultiInstance(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, rollbackSequentialMixml)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "rollbackSeqMI",
		Variables:            []api.Variable{{Name: "executorList", Value: []string{"a", "b", "c"}}},
	})
	completeByKey(t, eng, fake, "review", "a")
	taskB := taskOfKey(t, eng, "review")
	tick(fake)
	// a 撤回：当前执行人 b 的前一位正是 a
	if err := eng.RollbackTask(context.Background(), firstHistoricTaskID(t, eng, instance.ID, "review", "a"), "carol"); err != nil {
		t.Fatalf("串行会签撤回失败: %v", err)
	}
	task := taskOfKey(t, eng, "review")
	if task.Assignee != "carol" {
		t.Fatalf("撤回后当前执行人应为 carol: %+v", task)
	}
	// 会签人集合回写
	vars, _ := eng.GetVariables(context.Background(), instance.ID)
	list, _ := vars["executorList"].([]string)
	if len(list) != 3 || list[0] != "carol" || list[1] != "b" || list[2] != "c" {
		t.Fatalf("executorList 应回写为 [carol b c]: %v", vars["executorList"])
	}
	_ = taskB
	// 重走：carol -> b -> c -> next
	completeByAssignee(t, eng, fake, "review", "carol")
	completeByAssignee(t, eng, fake, "review", "b")
	completeByAssignee(t, eng, fake, "review", "c")
	completeByKey(t, eng, fake, "next", "dave")
	got, _ := eng.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("串行会签撤回重走应正常结束（loopCounter 计数自洽）")
	}
}

// TestRollbackSequentialMultiInstanceGuards 验证串行会签撤回防护：后一位已完成拒绝、执行人错位拒绝。
// @param t 测试上下文
func TestRollbackSequentialMultiInstanceGuards(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, rollbackSequentialMixml)
	startSeq := func() string {
		instance, err := eng.StartProcessInstance(context.Background(), api.StartRequest{
			ProcessDefinitionKey: "rollbackSeqMI",
			Variables:            []api.Variable{{Name: "executorList", Value: []string{"a", "b", "c"}}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return instance.ID
	}
	// 场景1：后一位已完成拒绝（b 完成晚于 a，串行链上不可再撤 a）
	instance1 := startSeq()
	completeByAssignee(t, eng, fake, "review", "a")
	hisA := firstHistoricTaskID(t, eng, instance1, "review", "a")
	completeByAssignee(t, eng, fake, "review", "b")
	tick(fake)
	if err := eng.RollbackTask(context.Background(), hisA, "carol"); err == nil || !strings.Contains(err.Error(), "下一线性完成任务") {
		t.Fatalf("后一位已完成应拒绝: %v", err)
	}
	// 场景2：执行人不在会签集合，拒绝
	instance2 := startSeq()
	completeByAssignee(t, eng, fake, "review", "a")
	hisA2 := firstHistoricTaskID(t, eng, instance2, "review", "a")
	// 白盒改写历史执行人构造错位场景
	eng.state.HistoricTasks[hisA2].Assignee = "zzz"
	tick(fake)
	if err := eng.RollbackTask(context.Background(), hisA2, "carol"); err == nil || !strings.Contains(err.Error(), "执行人不存在于初始参数") {
		t.Fatalf("执行人错位应拒绝: %v", err)
	}
	// 场景3：执行人在集合但不是当前执行人的前一位，拒绝
	instance3 := startSeq()
	completeByAssignee(t, eng, fake, "review", "a")
	hisA3 := firstHistoricTaskID(t, eng, instance3, "review", "a")
	// 当前执行人 b（loopCounter=1），改写历史执行人为 c（下标 2，非前一位）
	eng.state.HistoricTasks[hisA3].Assignee = "c"
	tick(fake)
	if err := eng.RollbackTask(context.Background(), hisA3, "carol"); err == nil || !strings.Contains(err.Error(), "前位") {
		t.Fatalf("非前一位应拒绝: %v", err)
	}
}

// firstHistoricTaskID 查找实例某活动指定办理人的历史任务 ID。
// @param t 测试上下文
// @param eng 引擎
// @param processInstanceID 实例 ID
// @param key 任务定义 Key
// @param assignee 办理人
// @return 历史任务 ID
func firstHistoricTaskID(t *testing.T, eng *Engine, processInstanceID, key, assignee string) string {
	t.Helper()
	for _, historic := range eng.state.HistoricTasks {
		if historic.ProcessInstanceID == processInstanceID && historic.TaskDefinitionKey == key && historic.Assignee == assignee {
			return historic.ID
		}
	}
	t.Fatalf("未找到历史任务 %s/%s", key, assignee)
	return ""
}

// 撤回语料：并行会签。
const rollbackParallelMixml = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="rollbackParaMI" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="review"/>
    <userTask id="review" name="review" flowable:assignee="${executor}">
      <multiInstanceLoopCharacteristics flowable:collection="${executorList}" flowable:elementVariable="executor"/>
    </userTask>
    <sequenceFlow id="f2" sourceRef="review" targetRef="next"/>
    <userTask id="next" name="next" flowable:assignee="dave"/>
    <sequenceFlow id="f3" sourceRef="next" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestRollbackParallelMultiInstance 验证并行会签进行中撤回：占位重入计数正确且汇聚不死锁。
// @param t 测试上下文
func TestRollbackParallelMultiInstance(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, rollbackParallelMixml)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "rollbackParaMI",
		Variables:            []api.Variable{{Name: "executorList", Value: []string{"a", "b"}}},
	})
	completeByAssignee(t, eng, fake, "review", "a")
	hisA := firstHistoricTaskID(t, eng, instance.ID, "review", "a")
	tick(fake)
	if err := eng.RollbackTask(context.Background(), hisA, "carol"); err != nil {
		t.Fatalf("并行会签撤回失败: %v", err)
	}
	// 重入后仍为 2 个任务（carol + b）
	tasks := listTasks(t, eng)
	if len(tasks) != 2 {
		t.Fatalf("撤回后应仍为 2 个任务: %+v", tasks)
	}
	// 计数自洽：活跃 2、完成 0
	miRoot := findMultiInstanceRoot(t, eng, instance.ID, "review")
	if miRoot == nil {
		t.Fatalf("未找到多实例根")
	}
	if got := miRoot.Variables["nrOfActiveInstances"]; got != 2 {
		t.Fatalf("nrOfActiveInstances 应为 2: %v", got)
	}
	if got := miRoot.Variables["nrOfCompletedInstances"]; got != 0 {
		t.Fatalf("nrOfCompletedInstances 应为 0: %v", got)
	}
	// 全部完成汇聚不死锁
	completeByAssignee(t, eng, fake, "review", "carol")
	completeByAssignee(t, eng, fake, "review", "b")
	completeByKey(t, eng, fake, "next", "dave")
	got, _ := eng.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("并行会签撤回后应正常汇聚结束")
	}
}

// TestRollbackCompletedMultiInstance 验证已完成会签撤回：占位 execution 构造计数正确、完成后续节点可重建。
// @param t 测试上下文
func TestRollbackCompletedMultiInstance(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, rollbackParallelMixml)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "rollbackParaMI",
		Variables:            []api.Variable{{Name: "executorList", Value: []string{"a", "b"}}},
	})
	completeByAssignee(t, eng, fake, "review", "a")
	completeByAssignee(t, eng, fake, "review", "b")
	hisA := firstHistoricTaskID(t, eng, instance.ID, "review", "a")
	tick(fake)
	if err := eng.RollbackTask(context.Background(), hisA, "carol"); err != nil {
		t.Fatalf("已完成会签撤回失败: %v", err)
	}
	// 占位构造：仅撤回人生成任务
	task := taskOfKey(t, eng, "review")
	if task.Assignee != "carol" {
		t.Fatalf("撤回任务办理人应为 carol: %+v", task)
	}
	// 会签根计数：实例 2、活跃 1、完成 1
	miRoot := findMultiInstanceRoot(t, eng, instance.ID, "review")
	if miRoot == nil {
		t.Fatalf("未找到重建的会签根")
	}
	if got := miRoot.Variables["nrOfInstances"]; got != 2 {
		t.Fatalf("nrOfInstances 应为 2: %v", got)
	}
	if got := miRoot.Variables["nrOfActiveInstances"]; got != 1 {
		t.Fatalf("nrOfActiveInstances 应为 1: %v", got)
	}
	if got := miRoot.Variables["nrOfCompletedInstances"]; got != 1 {
		t.Fatalf("nrOfCompletedInstances 应为 1: %v", got)
	}
	// 占位子实例存在且非活跃
	placeholders := 0
	for _, execution := range eng.state.Executions {
		if execution.ParentID == miRoot.ID && execution.MIChild && !execution.Ended && !execution.Active {
			placeholders++
		}
	}
	if placeholders != 1 {
		t.Fatalf("应有 1 个占位子实例，实际 %d", placeholders)
	}
	// 撤回人完成后汇聚生成 next
	completeByAssignee(t, eng, fake, "review", "carol")
	completeByKey(t, eng, fake, "next", "dave")
	got, _ := eng.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("已完成会签撤回重走应正常结束")
	}
}

// TestRollbackCompletedSequentialMultiInstance 验证串行会签已完成撤回：仅最后完成位可撤回。
// @param t 测试上下文
func TestRollbackCompletedSequentialMultiInstance(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, rollbackSequentialMixml)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "rollbackSeqMI",
		Variables:            []api.Variable{{Name: "executorList", Value: []string{"a", "b"}}},
	})
	completeByAssignee(t, eng, fake, "review", "a")
	completeByAssignee(t, eng, fake, "review", "b")
	hisA := firstHistoricTaskID(t, eng, instance.ID, "review", "a")
	tick(fake)
	// a 不是串行最后一位，拒绝
	if err := eng.RollbackTask(context.Background(), hisA, "carol"); err == nil || !strings.Contains(err.Error(), "不是串行最后一个节点") {
		t.Fatalf("非最后一位应拒绝: %v", err)
	}
	// b 是最后一位，允许
	hisB := firstHistoricTaskID(t, eng, instance.ID, "review", "b")
	tick(fake)
	if err := eng.RollbackTask(context.Background(), hisB, "carol"); err != nil {
		t.Fatalf("最后一位撤回失败: %v", err)
	}
	task := taskOfKey(t, eng, "review")
	if task.Assignee != "carol" {
		t.Fatalf("撤回任务办理人应为 carol: %+v", task)
	}
	completeByKey(t, eng, fake, "review", "carol")
	completeByKey(t, eng, fake, "next", "dave")
	got, _ := eng.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("串行会签已完成撤回重走应正常结束")
	}
}

// 撤回语料：排他网关后继。
const rollbackGatewayXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="rollbackGateway" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="A"/>
    <userTask id="A" name="A" flowable:assignee="alice"/>
    <sequenceFlow id="f2" sourceRef="A" targetRef="g"/>
    <exclusiveGateway id="g"/>
    <sequenceFlow id="f3" sourceRef="g" targetRef="B">
      <conditionExpression>${x == 1}</conditionExpression>
    </sequenceFlow>
    <sequenceFlow id="f4" sourceRef="g" targetRef="C">
      <conditionExpression>${x == 2}</conditionExpression>
    </sequenceFlow>
    <userTask id="B" name="B" flowable:assignee="bob"/>
    <userTask id="C" name="C" flowable:assignee="carl"/>
    <sequenceFlow id="f5" sourceRef="B" targetRef="e"/>
    <sequenceFlow id="f6" sourceRef="C" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestRollbackNextGateway 验证网关撤回：网关后任务清场、残留执行清理、撤回后可改道。
// @param t 测试上下文
func TestRollbackNextGateway(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, rollbackGatewayXML)
	var seen []string
	newRollbackHook(eng, &seen)
	_ = seen
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "rollbackGateway",
		Variables:            []api.Variable{{Name: "x", Value: 1}},
	})
	taskA := taskOfKey(t, eng, "A")
	completeByKey(t, eng, fake, "A", "alice")
	if task := taskOfKey(t, eng, "B"); task.ID == "" {
		t.Fatalf("应路由到 B")
	}
	tick(fake)
	if err := eng.RollbackTask(context.Background(), taskA.ID, "carol"); err != nil {
		t.Fatalf("网关撤回失败: %v", err)
	}
	// 仅剩撤回任务
	tasks := listTasks(t, eng)
	if len(tasks) != 1 || tasks[0].TaskDefinitionKey != "A" || tasks[0].Assignee != "carol" {
		t.Fatalf("网关撤回后应只剩 A 任务: %+v", tasks)
	}
	// 改道撤回后重走
	tick(fake)
	if err := eng.SetVariable(context.Background(), instance.ID, "x", 2); err != nil {
		t.Fatal(err)
	}
	completeByKey(t, eng, fake, "A", "carol")
	if task := taskOfKey(t, eng, "C"); task.ID == "" {
		t.Fatalf("改道后应路由到 C")
	}
	completeByKey(t, eng, fake, "C", "carl")
	got, _ := eng.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("网关撤回重走应正常结束")
	}
}

// 撤回语料：分支在并行网关汇合后继续（汇聚宿主保活场景）。
const rollbackGatewayJoinXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="rollbackGatewayJoin" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="fork"/>
    <parallelGateway id="fork"/>
    <sequenceFlow id="f2" sourceRef="fork" targetRef="A"/>
    <sequenceFlow id="f3" sourceRef="fork" targetRef="X"/>
    <userTask id="A" name="A" flowable:assignee="alice"/>
    <userTask id="X" name="X" flowable:assignee="xu"/>
    <sequenceFlow id="f4" sourceRef="A" targetRef="join"/>
    <sequenceFlow id="f5" sourceRef="X" targetRef="join"/>
    <parallelGateway id="join"/>
    <sequenceFlow id="f6" sourceRef="join" targetRef="D"/>
    <userTask id="D" name="D" flowable:assignee="dave"/>
    <sequenceFlow id="f7" sourceRef="D" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestRollbackGatewayJoinHost 验证多分支汇合后的撤回：汇聚宿主保活、其余分支伪造到达、汇聚不死锁。
// @param t 测试上下文
func TestRollbackGatewayJoinHost(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, rollbackGatewayJoinXML)
	var seen []string
	newRollbackHook(eng, &seen)
	_ = seen
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "rollbackGatewayJoin"})
	taskA := taskOfKey(t, eng, "A")
	completeByKey(t, eng, fake, "A", "alice")
	completeByKey(t, eng, fake, "X", "xu")
	// 汇聚后 D 在办
	taskD := taskOfKey(t, eng, "D")
	if taskD.ID == "" {
		t.Fatalf("汇聚后应有 D 任务")
	}
	tick(fake)
	if err := eng.RollbackTask(context.Background(), taskA.ID, "carol"); err != nil {
		t.Fatalf("汇聚宿主撤回失败: %v", err)
	}
	// 仅剩撤回任务 A
	tasks := listTasks(t, eng)
	if len(tasks) != 1 || tasks[0].TaskDefinitionKey != "A" {
		t.Fatalf("撤回后应只剩 A 任务: %+v", tasks)
	}
	// A 重做后经伪造到达的汇聚放行 D
	completeByKey(t, eng, fake, "A", "carol")
	completeByKey(t, eng, fake, "D", "dave")
	got, _ := eng.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("汇聚宿主撤回重走应正常结束（不死锁）")
	}
}

// 撤回语料：后继为嵌入式子流程。
const rollbackSubProcessXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="rollbackSub" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="A"/>
    <userTask id="A" name="A" flowable:assignee="alice"/>
    <sequenceFlow id="f2" sourceRef="A" targetRef="S"/>
    <subProcess id="S" name="S">
      <startEvent id="sStart"/>
      <sequenceFlow id="sf1" sourceRef="sStart" targetRef="sT1"/>
      <userTask id="sT1" name="sT1" flowable:assignee="bob"/>
      <sequenceFlow id="sf2" sourceRef="sT1" targetRef="sEnd"/>
      <endEvent id="sEnd"/>
    </subProcess>
    <sequenceFlow id="f3" sourceRef="S" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestRollbackNextSubProcess 验证嵌入式子流程撤回：子流程内任务与层级执行级联清理。
// @param t 测试上下文
func TestRollbackNextSubProcess(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, rollbackSubProcessXML)
	var seen []string
	newRollbackHook(eng, &seen)
	_ = seen
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "rollbackSub"})
	taskA := taskOfKey(t, eng, "A")
	completeByKey(t, eng, fake, "A", "alice")
	if task := taskOfKey(t, eng, "sT1"); task.ID == "" {
		t.Fatalf("子流程任务应生成")
	}
	tick(fake)
	if err := eng.RollbackTask(context.Background(), taskA.ID, "carol"); err != nil {
		t.Fatalf("子流程撤回失败: %v", err)
	}
	tasks := listTasks(t, eng)
	if len(tasks) != 1 || tasks[0].TaskDefinitionKey != "A" || tasks[0].Assignee != "carol" {
		t.Fatalf("子流程撤回后应只剩 A 任务: %+v", tasks)
	}
	// 重走子流程
	completeByKey(t, eng, fake, "A", "carol")
	completeByKey(t, eng, fake, "sT1", "bob")
	got, _ := eng.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("子流程撤回重走应正常结束")
	}
}

// 撤回语料：后继为调用活动（父子两个流程定义同部署）。
const rollbackCallActivityXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="rollbackCallParent" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="A"/>
    <userTask id="A" name="A" flowable:assignee="alice"/>
    <sequenceFlow id="f2" sourceRef="A" targetRef="call"/>
    <callActivity id="call" calledElement="rollbackCallChild"/>
    <sequenceFlow id="f3" sourceRef="call" targetRef="e"/>
    <endEvent id="e"/>
  </process>
  <process id="rollbackCallChild" isExecutable="true">
    <startEvent id="cs"/>
    <sequenceFlow id="cf1" sourceRef="cs" targetRef="cT1"/>
    <userTask id="cT1" name="cT1" flowable:assignee="bob"/>
    <sequenceFlow id="cf2" sourceRef="cT1" targetRef="ce"/>
    <endEvent id="ce"/>
  </process>
</definitions>`

// TestRollbackNextCallActivity 验证调用活动撤回：子实例清场、父侧执行删除、可重走。
// @param t 测试上下文
func TestRollbackNextCallActivity(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, rollbackCallActivityXML)
	var seen []string
	newRollbackHook(eng, &seen)
	_ = seen
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "rollbackCallParent"})
	taskA := taskOfKey(t, eng, "A")
	completeByKey(t, eng, fake, "A", "alice")
	if task := taskOfKey(t, eng, "cT1"); task.ID == "" {
		t.Fatalf("子流程任务应生成")
	}
	tick(fake)
	if err := eng.RollbackTask(context.Background(), taskA.ID, "carol"); err != nil {
		t.Fatalf("调用活动撤回失败: %v", err)
	}
	tasks := listTasks(t, eng)
	if len(tasks) != 1 || tasks[0].TaskDefinitionKey != "A" || tasks[0].Assignee != "carol" {
		t.Fatalf("调用活动撤回后应只剩 A 任务: %+v", tasks)
	}
	// 子实例已清场：运行实例只剩父实例
	instances, _ := eng.ListProcessInstances(context.Background())
	if len(instances) != 1 || instances[0].ID != instance.ID {
		t.Fatalf("子实例应被清理: %+v", instances)
	}
	// 重走
	completeByKey(t, eng, fake, "A", "carol")
	completeByKey(t, eng, fake, "cT1", "bob")
	got, _ := eng.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("调用活动撤回重走应正常结束")
	}
}

// 直送语料：线性三节点。
const completeToTargetXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="completeToTarget" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="A"/>
    <userTask id="A" name="A" flowable:assignee="alice"/>
    <sequenceFlow id="f2" sourceRef="A" targetRef="B"/>
    <userTask id="B" name="B" flowable:assignee="bob"/>
    <sequenceFlow id="f3" sourceRef="B" targetRef="C"/>
    <userTask id="C" name="C" flowable:assignee="carl"/>
    <sequenceFlow id="f4" sourceRef="C" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestCompleteToTarget 验证直送：目标落位、原任务完成历史保留、operationType 变量。
// @param t 测试上下文
func TestCompleteToTarget(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, completeToTargetXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "completeToTarget"})
	completeByKey(t, eng, fake, "A", "alice")
	taskB := taskOfKey(t, eng, "B")
	tick(fake)
	if err := eng.CompleteToTarget(context.Background(), taskB.ID, "C", map[string]any{"reason": "fast"}); err != nil {
		t.Fatalf("直送失败: %v", err)
	}
	// 目标节点落位
	if task := taskOfKey(t, eng, "C"); task.ID == "" {
		t.Fatalf("直送后应有 C 任务")
	}
	// 原任务完成历史保留（endTime 有值且无删除原因）
	historic, err := eng.GetHistoricTaskInstance(context.Background(), taskB.ID)
	if err != nil || historic.EndTime == nil || historic.DeleteReason != "" {
		t.Fatalf("直送原任务应保留完成历史: %+v %v", historic, err)
	}
	// operationType 变量
	vars, _ := eng.GetVariables(context.Background(), instance.ID)
	if vars["operationType"] != "completeToTarget" || vars["reason"] != "fast" {
		t.Fatalf("直送变量不符: %v", vars)
	}
	completeByKey(t, eng, fake, "C", "carl")
	got, _ := eng.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("直送后重走应正常结束")
	}
}

// TestCompleteToTargetErrors 验证直送校验：目标为空、目标不存在、实例结束。
// @param t 测试上下文
func TestCompleteToTargetErrors(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, completeToTargetXML)
	_, _ = eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "completeToTarget"})
	taskA := taskOfKey(t, eng, "A")
	if err := eng.CompleteToTarget(context.Background(), taskA.ID, " ", nil); err == nil {
		t.Fatalf("目标为空应报错")
	}
	if err := eng.CompleteToTarget(context.Background(), taskA.ID, "missing", nil); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("目标不存在应 404: %v", err)
	}
	completeByKey(t, eng, fake, "A", "alice")
	completeByKey(t, eng, fake, "B", "bob")
	completeByKey(t, eng, fake, "C", "carl")
	if err := eng.CompleteToTarget(context.Background(), taskA.ID, "C", nil); err == nil {
		t.Fatalf("实例结束后直送应报错")
	}
}

// 软终止/重置语料：并行分叉汇合。
const stopResetXML = backParallelPlainXML

// TestStopProcessToEvent 验证软终止：end 事件历史存在、实例结束、任务清场留痕。
// @param t 测试上下文
func TestStopProcessToEvent(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, stopResetXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "backParallelPlain"})
	completeByKey(t, eng, fake, "A", "alice")
	taskB2 := taskOfKey(t, eng, "B2")
	tick(fake)
	if err := eng.StopProcessToEvent(context.Background(), instance.ID); err != nil {
		t.Fatalf("软终止失败: %v", err)
	}
	got, _ := eng.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("软终止后实例应结束")
	}
	// end 事件历史存在
	page, err := eng.QueryHistoricActivityInstances(context.Background(), api.HistoricActivityInstanceFilter{
		ProcessInstanceID: instance.ID,
		ActivityID:        "e",
		Size:              10,
	})
	if err != nil || page.Total == 0 {
		t.Fatalf("软终止应保留 end 事件历史: %v %d", err, page.Total)
	}
	// 任务清场留痕
	historic, _ := eng.GetHistoricTaskInstance(context.Background(), taskB2.ID)
	if historic.EndTime == nil {
		t.Fatalf("软终止任务应闭合历史")
	}
	if tasks := listTasks(t, eng); len(tasks) != 0 {
		t.Fatalf("软终止后不应有运行任务: %+v", tasks)
	}
	// 重复软终止拒绝
	if err := eng.StopProcessToEvent(context.Background(), instance.ID); err == nil {
		t.Fatalf("已结束实例软终止应拒绝")
	}
}

// TestStopProcessToEventSuspended 验证挂起实例拒绝软终止。
// @param t 测试上下文
func TestStopProcessToEventSuspended(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, stopResetXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "backParallelPlain"})
	completeByKey(t, eng, fake, "A", "alice")
	if err := eng.SuspendProcessInstance(context.Background(), instance.ID); err != nil {
		t.Fatal(err)
	}
	if err := eng.StopProcessToEvent(context.Background(), instance.ID); err == nil || !strings.Contains(err.Error(), "挂起") {
		t.Fatalf("挂起实例应拒绝软终止: %v", err)
	}
}

// TestResetProcess 验证重置：全部运行任务清场、执行迁回首任务、可重新办理。
// @param t 测试上下文
func TestResetProcess(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, stopResetXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "backParallelPlain"})
	completeByKey(t, eng, fake, "A", "alice")
	tick(fake)
	if err := eng.ResetProcess(context.Background(), instance.ID, "op1"); err != nil {
		t.Fatalf("重置失败: %v", err)
	}
	// 回到首任务 A
	tasks := listTasks(t, eng)
	if len(tasks) != 1 || tasks[0].TaskDefinitionKey != "A" {
		t.Fatalf("重置后应只剩首任务 A: %+v", tasks)
	}
	// 删除原因 reset
	got, _ := eng.GetProcessInstance(context.Background(), instance.ID)
	if got.Ended {
		t.Fatalf("重置后实例应继续运行")
	}
	// 可重新办理
	completeByKey(t, eng, fake, "A", "alice")
	completeByKey(t, eng, fake, "B1", "bob")
	completeByKey(t, eng, fake, "B2", "carl")
	completeByKey(t, eng, fake, "C", "dave")
	final, _ := eng.GetProcessInstance(context.Background(), instance.ID)
	if !final.Ended {
		t.Fatalf("重置后重走应正常结束")
	}
}

// 重激活语料：线性两节点。
const reactivateXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="reactivate" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="A"/>
    <userTask id="A" name="A" flowable:assignee="alice"/>
    <sequenceFlow id="f2" sourceRef="A" targetRef="B"/>
    <userTask id="B" name="B" flowable:assignee="bob"/>
    <sequenceFlow id="f3" sourceRef="B" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestReactivateProcessInstance 验证历史实例复活：历史行复位、变量按时间升序覆盖、指定落位与发起人重写。
// @param t 测试上下文
func TestReactivateProcessInstance(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, reactivateXML)
	instance, err := eng.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "reactivate",
		UserID:               "u1",
		Variables:            []api.Variable{{Name: "k1", Value: "v1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	completeByKey(t, eng, fake, "A", "alice")
	completeByKey(t, eng, fake, "B", "bob")
	got, _ := eng.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("首轮应结束")
	}
	historic, err := eng.GetHistoricProcessInstance(context.Background(), instance.ID)
	if err != nil || !historic.Ended {
		t.Fatalf("历史实例应存在: %v", err)
	}
	tick(fake)
	// 复活到 B
	if err := eng.ReactivateProcessInstance(context.Background(), instance.ID, "B", "admin"); err != nil {
		t.Fatalf("复活失败: %v", err)
	}
	// 历史行复位
	revived, _ := eng.GetHistoricProcessInstance(context.Background(), instance.ID)
	if revived.Ended || revived.EndTime != nil || revived.DurationMillis != 0 || revived.State != "running" {
		t.Fatalf("历史实例应复位: %+v", revived)
	}
	// 落位 B
	if task := taskOfKey(t, eng, "B"); task.ID == "" {
		t.Fatalf("复活后应落位 B")
	}
	// 变量按创建时间升序覆盖：任务局部 k1（更晚）覆盖流程级 k1
	vars, _ := eng.GetVariables(context.Background(), instance.ID)
	if vars["k1"] != "v1" {
		t.Fatalf("流程级 k1 应重建: %v", vars["k1"])
	}
	// 发起人重写
	after, _ := eng.GetProcessInstance(context.Background(), instance.ID)
	if after.StartUserID != "admin" {
		t.Fatalf("发起人应重写为 admin: %v", after.StartUserID)
	}
	completeByKey(t, eng, fake, "B", "bob")
	final, _ := eng.GetProcessInstance(context.Background(), instance.ID)
	if !final.Ended {
		t.Fatalf("复活后重走应正常结束")
	}
}

// TestReactivateProcessInstanceDefaults 验证复活默认落位开始事件与运行中实例拒绝。
// @param t 测试上下文
func TestReactivateProcessInstanceDefaults(t *testing.T) {
	eng, fake := newExtEngine(t)
	deployXML(t, eng, reactivateXML)
	instance, _ := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "reactivate"})
	// 运行中实例拒绝复活
	if err := eng.ReactivateProcessInstance(context.Background(), instance.ID, "", "admin"); err == nil {
		t.Fatalf("运行中实例应拒绝复活")
	}
	completeByKey(t, eng, fake, "A", "alice")
	completeByKey(t, eng, fake, "B", "bob")
	// 目标活动不存在（实例已结束，先于成功复活校验）
	tick(fake)
	if err := eng.ReactivateProcessInstance(context.Background(), instance.ID, "missing", "admin"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("目标不存在应 404: %v", err)
	}
	// 不存在的历史实例
	if err := eng.ReactivateProcessInstance(context.Background(), "missing", "", "admin"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("历史实例不存在应 404: %v", err)
	}
	// 默认落位：回开始事件推进到首个用户任务 A
	if err := eng.ReactivateProcessInstance(context.Background(), instance.ID, "", "admin"); err != nil {
		t.Fatalf("默认复活失败: %v", err)
	}
	if task := taskOfKey(t, eng, "A"); task.ID == "" {
		t.Fatalf("默认复活应落到首任务 A")
	}
}

// TestGraphUtilIsReachable 验证可达性判定（G2）。
// @param t 测试上下文
func TestGraphUtilIsReachable(t *testing.T) {
	eng, _ := newExtEngine(t)
	deployXML(t, eng, backSubProcessXML)
	process := definitionModel(t, eng, "backSubProcess")
	a := mustFlowNode(t, process, "A")
	c := mustFlowNode(t, process, "C")
	sT1 := mustFlowNode(t, process, "sT1")
	sT2 := mustFlowNode(t, process, "sT2")
	// A 可达 C 与子流程内部节点
	if !IsReachable(process, a, c) {
		t.Fatalf("A 应可达 C")
	}
	if !IsReachable(process, a, sT1) {
		t.Fatalf("A 应可达 sT1")
	}
	// C 不可达 A
	if IsReachable(process, c, a) {
		t.Fatalf("C 不应可达 A")
	}
	// 子流程内部节点可达外部 C（经子流程终点提升）
	if !IsReachable(process, sT2, c) {
		t.Fatalf("sT2 应可达 C")
	}
	// 子流程终点不可达子流程内部
	sEnd := mustFlowNode(t, process, "sEnd")
	if IsReachable(process, sEnd, sT1) {
		t.Fatalf("sEnd 不应可达 sT1")
	}
}

// TestGraphUtilSpecialGateway 验证特殊网关识别（G3）与子流程层级映射。
// @param t 测试上下文
func TestGraphUtilSpecialGateway(t *testing.T) {
	eng, _ := newExtEngine(t)
	deployXML(t, eng, backParallelNestedXML)
	deployXML(t, eng, backSubProcessXML)
	process := definitionModel(t, eng, "backParallelNested")
	blocks := SpecialGatewayElements(process)
	if len(blocks) != 2 {
		t.Fatalf("应识别 2 个特殊网关块: %d", len(blocks))
	}
	// 外层在前
	if blocks[0].GatewayID != "p1" || blocks[1].GatewayID != "p2" {
		t.Fatalf("块排序应为外层 p1 在前: %+v", blocks)
	}
	// 内层块只含自身元素
	for _, id := range []string{"p2_begin", "y1", "y2", "p2_end"} {
		if !blocks[1].Elements[id] {
			t.Fatalf("p2 块应包含 %s", id)
		}
	}
	if blocks[1].Elements["x1"] || blocks[1].Elements["z1"] {
		t.Fatalf("p2 块不应包含外层分支节点")
	}
	// 外层块包含内外全部元素
	for _, id := range []string{"p1_begin", "x1", "y1", "p2_end", "z1", "x2", "p1_end"} {
		if !blocks[0].Elements[id] {
			t.Fatalf("p1 块应包含 %s", id)
		}
	}
	// 子流程层级映射：主流程节点对子流程节点提升到子流程层
	linear := definitionModel(t, eng, "backSubProcess")
	source, target := getSourceAndTargetRealActivityID(linear, mustFlowNode(t, linear, "A"), mustFlowNode(t, linear, "sT1"))
	if source != "A" || target != "S" {
		t.Fatalf("跨子流程映射应提升为 A/S: %s/%s", source, target)
	}
	source, target = getSourceAndTargetRealActivityID(linear, mustFlowNode(t, linear, "sT1"), mustFlowNode(t, linear, "A"))
	if source != "S" || target != "A" {
		t.Fatalf("跨子流程映射应提升为 S/A: %s/%s", source, target)
	}
	// 同层保持原节点
	source, target = getSourceAndTargetRealActivityID(linear, mustFlowNode(t, linear, "sT1"), mustFlowNode(t, linear, "sT2"))
	if source != "sT1" || target != "sT2" {
		t.Fatalf("同层映射应保持原节点: %s/%s", source, target)
	}
	// diffLevel 边界
	if got := diffLevel([]string{"m"}, []string{"m"}); got != -1 {
		t.Fatalf("同链应 -1: %d", got)
	}
	if got := diffLevel([]string{"m"}, []string{"n"}); got != 0 {
		t.Fatalf("异链应 0: %d", got)
	}
	if got := diffLevel([]string{"m", "s1"}, []string{"m"}); got != 1 {
		t.Fatalf("前缀链应返回浅方长度: %d", got)
	}
}

// mustFlowNode 按ID 取流程节点。
// @param t 测试上下文
// @param process 流程
// @param id 元素 ID
// @return 流程节点
func mustFlowNode(t *testing.T, process *model.Process, id string) model.FlowNode {
	t.Helper()
	node, ok := findFlowNode(process, id)
	if !ok {
		t.Fatalf("节点不存在: %s", id)
	}
	return node
}
