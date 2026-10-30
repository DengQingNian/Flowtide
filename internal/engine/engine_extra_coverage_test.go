package engine

import (
	"context"
	"os"
	"testing"

	statestore "github.com/DengQingNian/Flowtide/internal/persistence/state"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/bpmn/model"
)

// TestCounterTypes 验证多实例计数类型。
// @param t 测试上下文
func TestCounterTypes(t *testing.T) {
	engine, _ := New(Config{})
	execution := &executionState{Variables: map[string]any{"i": 1, "i64": int64(2), "f": 3.0, "s": "x"}}
	if engine.counter(execution, "i") != 1 || engine.counter(execution, "i64") != 2 || engine.counter(execution, "f") != 3 {
		t.Fatalf("计数类型转换错误")
	}
	if engine.counter(execution, "s") != 0 || engine.counter(execution, "missing") != 0 {
		t.Fatalf("未知计数应返回 0")
	}
}

// TestCancelBoundaryTimersOnlyTarget 验证只取消目标执行的作业。
// @param t 测试上下文
func TestCancelBoundaryTimersOnlyTarget(t *testing.T) {
	engine, _ := New(Config{})
	engine.state.Jobs["a"] = &jobState{ID: "a", ExecutionID: "e1"}
	engine.state.Jobs["b"] = &jobState{ID: "b", ExecutionID: "e2"}
	engine.cancelBoundaryTimers("e1")
	if _, ok := engine.state.Jobs["a"]; ok {
		t.Fatalf("目标作业应删除")
	}
	if _, ok := engine.state.Jobs["b"]; !ok {
		t.Fatalf("其他作业不应删除")
	}
}

// TestSaveStateErrorRollsBack 验证落盘失败回滚。
// @param t 测试上下文
func TestSaveStateErrorRollsBack(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	directory := t.TempDir()
	engine.cfg.StateStore = statestore.NewFile(directory)
	if err := engine.SetVariable(context.Background(), instance.ID, "x", 1); err == nil {
		t.Fatalf("状态路径为目录时应失败")
	}
	variables, _ := engine.GetVariables(context.Background(), instance.ID)
	if _, ok := variables["x"]; ok {
		t.Fatalf("落盘失败应回滚变量")
	}
	if err := engine.Close(context.Background()); err == nil {
		t.Fatalf("关闭时落盘失败应返回错误")
	}
}

// TestResolveMultiInstanceErrors 验证多实例配置错误。
// @param t 测试上下文
func TestResolveMultiInstanceErrors(t *testing.T) {
	engine, _ := New(Config{})
	execution := &executionState{ID: "e", Variables: map[string]any{"items": []string{"a"}}}
	if _, err := engine.resolveMultiInstanceValues(execution, &model.MultiInstanceLoopCharacteristics{Collection: "missing"}); err == nil {
		t.Fatalf("缺失集合应报错")
	}
	if _, err := engine.resolveMultiInstanceValues(execution, &model.MultiInstanceLoopCharacteristics{LoopCardinality: "bad"}); err == nil {
		t.Fatalf("非法基数应报错")
	}
	if _, err := engine.resolveMultiInstanceValues(execution, &model.MultiInstanceLoopCharacteristics{LoopCardinality: "-1"}); err == nil {
		t.Fatalf("负数基数应报错")
	}
	if _, err := engine.resolveMultiInstanceValues(execution, &model.MultiInstanceLoopCharacteristics{}); err == nil {
		t.Fatalf("空配置应报错")
	}
	values, err := engine.resolveMultiInstanceValues(execution, &model.MultiInstanceLoopCharacteristics{Collection: "items"})
	if err != nil || len(values) != 1 {
		t.Fatalf("集合解析失败: %v %+v", err, values)
	}
	if values, err := engine.resolveMultiInstanceValues(execution, &model.MultiInstanceLoopCharacteristics{LoopCardinality: "2"}); err != nil || len(values) != 2 {
		t.Fatalf("基数解析失败: %v %+v", err, values)
	}
	if _, err := toAnySlice(1); err == nil {
		t.Fatalf("非切片应报错")
	}
	if values, err := toAnySlice(nil); err != nil || values != nil {
		t.Fatalf("nil 应返回 nil")
	}
}

// TestCallDepthChain 验证调用深度计算。
// @param t 测试上下文
func TestCallDepthChain(t *testing.T) {
	engine, _ := New(Config{})
	engine.state.Executions["parent"] = &executionState{ID: "parent", ProcessInstanceID: "parentInstance"}
	engine.state.Instances["parentInstance"] = &instanceState{ID: "parentInstance"}
	engine.state.Instances["childInstance"] = &instanceState{ID: "childInstance", SuperExecutionID: "parent"}
	engine.state.Executions["child"] = &executionState{ID: "child", ProcessInstanceID: "childInstance"}
	if depth := engine.callDepth(engine.state.Executions["child"]); depth != 2 {
		t.Fatalf("调用深度错误: %d", depth)
	}
}

// TestRunDueJobsMissingExecution 验证作业指向缺失执行。
// @param t 测试上下文
func TestRunDueJobsMissingExecution(t *testing.T) {
	engine, _ := New(Config{})
	engine.state.Jobs["j"] = &jobState{ID: "j", ExecutionID: "missing", DueDate: time.Now().Add(-time.Minute)}
	executed, err := engine.RunDueJobs(context.Background())
	if err != nil || executed != 1 {
		t.Fatalf("缺失执行作业应被跳过: %v %d", err, executed)
	}
}

// TestInvalidBoundaryTimer 验证非法边界定时器。
// @param t 测试上下文
func TestInvalidBoundaryTimer(t *testing.T) {
	engine := newTestEngine(t)
	xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="badTimer" isExecutable="true"><startEvent id="s"/><sequenceFlow id="f1" sourceRef="s" targetRef="task"/><userTask id="task"/><boundaryEvent id="b" attachedToRef="task"><timerEventDefinition><timeDuration>bad</timeDuration></timerEventDefinition></boundaryEvent><sequenceFlow id="f2" sourceRef="task" targetRef="e"/><sequenceFlow id="f3" sourceRef="b" targetRef="e"/><endEvent id="e"/></process></definitions>`
	deployXML(t, engine, xml)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "badTimer"}); err == nil {
		t.Fatalf("非法定时器应报错")
	}
}

// TestSaveStateMissingDirectory 验证状态目录不可写。
// @param t 测试上下文
func TestSaveStateMissingDirectory(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "file-*")
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	_ = file.Close()
	engine := newTestEngine(t)
	engine.cfg.StateStore = statestore.NewFile(file.Name() + "/state.json")
	if err := engine.saveState(context.Background()); err == nil {
		t.Fatalf("父路径是文件时应失败")
	}
}
