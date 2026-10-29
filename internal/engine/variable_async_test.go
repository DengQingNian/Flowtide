package engine

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/clock"
	statestore "github.com/flowable-go/flowable/internal/persistence/state"
	"github.com/flowable-go/flowable/internal/tenant"
)

// TestAsyncVariablesDeferredRestore 验证排队不立即改值，文件及行恢复后仍保留精确类型和大载荷。
// @param t 测试上下文
// @return 无
func TestAsyncVariablesDeferredRestore(t *testing.T) {
	ctx := context.Background()
	store := statestore.NewMemory()
	source, err := New(Config{StateStore: store, HistoryLevel: HistoryFull, Clock: clock.NewFake(time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC))})
	if err != nil {
		t.Fatal(err)
	}
	deployXML(t, source, simpleProcess)
	instance, err := source.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "simple"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"large": int64(9007199254740993), "binary": []byte{0, 1, 255}, "date": time.UnixMilli(1700000000123).UTC(), "text": strings.Repeat("异步数据", 2000)}
	variables := make([]api.Variable, 0, len(want))
	for name, value := range want {
		variables = append(variables, api.Variable{Name: name, Value: value})
	}
	if err := source.SetVariablesAsync(ctx, api.AsyncVariableRequest{ProcessInstanceID: instance.ID, Variables: variables}); err != nil {
		t.Fatal(err)
	}
	if _, err := source.GetVariable(ctx, instance.ID, "large"); !errors.Is(err, api.ErrNotFound) {
		t.Fatal("异步请求提前修改了变量")
	}
	rows, err := source.SnapshotRows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.Close(ctx); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"快照", "行"} {
		t.Run(mode, func(t *testing.T) {
			var restored *Engine
			var err error
			if mode == "快照" {
				restored, err = New(Config{StateStore: store})
			} else {
				restored = newTestEngine(t)
				err = restored.RestoreRows(ctx, rows)
			}
			if err != nil {
				t.Fatal(err)
			}
			if count, err := restored.RunDueJobs(ctx); err != nil || count != 1 {
				t.Fatalf("异步作业执行: %d %v", count, err)
			}
			got, err := restored.GetVariables(ctx, instance.ID)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("异步恢复类型或值丢失: %v", err)
			}
			if count, err := restored.RunDueJobs(ctx); err != nil || count != 0 {
				t.Fatalf("异步作业重复执行: %d %v", count, err)
			}
		})
	}
}

// TestAsyncVariablesValidation 验证非法批次、冲突和租户越权不会排队或产生部分变量。
// @param t 测试上下文
// @return 无
func TestAsyncVariablesValidation(t *testing.T) {
	app := newTestEngine(t)
	deployXML(t, app, simpleProcess)
	instance, err := app.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "simple", Variables: []api.Variable{{Name: "exists", Value: "原值"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range []api.AsyncVariableRequest{
		{ProcessInstanceID: instance.ID},
		{ProcessInstanceID: "missing", Variables: []api.Variable{{Name: "x", Value: 1}}},
		{ProcessInstanceID: instance.ID, ExecutionID: instance.ID, Variables: []api.Variable{{Name: "x", Value: 1}}},
		{ProcessInstanceID: instance.ID, Variables: []api.Variable{{Name: "x", Value: 1}, {Name: "", Value: 2}}},
		{ProcessInstanceID: instance.ID, Variables: []api.Variable{{Name: "x", Value: 1}, {Name: "x", Value: 2}}},
		{ProcessInstanceID: instance.ID, CreateOnly: true, Variables: []api.Variable{{Name: "x", Value: 1}, {Name: "exists", Value: 2}}},
		{ProcessInstanceID: instance.ID, RequireExisting: true, Variables: []api.Variable{{Name: "missing", Value: 2}}},
		{ExecutionID: instance.ID, Variables: []api.Variable{{Name: "x", Value: 1, Scope: "global"}}},
		{ExecutionID: instance.ID, Variables: []api.Variable{{Name: "x", Value: 1, Scope: "unknown"}}},
		{ExecutionID: instance.ID, Variables: []api.Variable{{Name: "x", Value: 1}, {Name: "y", Value: 2, Scope: "global"}}},
		{ExecutionID: instance.ID, Variables: []api.Variable{{Name: "x", Value: make(chan int)}}},
	} {
		if err := app.SetVariablesAsync(context.Background(), request); err == nil {
			t.Fatalf("非法异步批次未拒绝: %+v", request)
		}
	}
	if err := app.SetVariablesAsync(tenant.With(context.Background(), "foreign"), api.AsyncVariableRequest{ProcessInstanceID: instance.ID, Variables: []api.Variable{{Name: "x", Value: 1}}}); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("异步越权未拒绝: %v", err)
	}
	if len(app.state.Jobs) != 0 {
		t.Fatal("失败异步请求留下作业")
	}
	values, err := app.GetVariables(context.Background(), instance.ID)
	if err != nil || len(values) != 1 || values["exists"] != "原值" {
		t.Fatalf("失败批次改变变量: %+v %v", values, err)
	}
}

// TestAsyncVariablesRetryAndDeadLetter 验证损坏载荷整批回滚、重试、死信行恢复及运维重放。
// @param t 测试上下文
// @return 无
func TestAsyncVariablesRetryAndDeadLetter(t *testing.T) {
	ctx := context.Background()
	app := newTestEngine(t)
	deployXML(t, app, simpleProcess)
	instance, err := app.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "simple"})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.SetVariablesAsync(ctx, api.AsyncVariableRequest{ProcessInstanceID: instance.ID, Variables: []api.Variable{{Name: "a", Value: 1}, {Name: "b", Value: []byte("恢复值")}}}); err != nil {
		t.Fatal(err)
	}
	var id string
	var original []byte
	for key, job := range app.state.Jobs {
		id = key
		original = append([]byte(nil), job.VariablePayload...)
		job.VariablePayload = []byte(`{"version":1,"variables":{"a":{"type":"int","text":"2"},"b":{"type":"broken"}}}`)
	}
	for i := 0; i < 3; i++ {
		if count, err := app.RunDueJobs(ctx); err != nil || count != 1 {
			t.Fatalf("第%d次重试: %d %v", i, count, err)
		}
		if _, err := app.GetVariable(ctx, instance.ID, "a"); !errors.Is(err, api.ErrNotFound) {
			t.Fatal("失败批次留下前半批变量")
		}
		app.cfg.Clock.(*clock.Fake).Advance(10 * time.Second)
	}
	if len(app.state.DeadLetters) != 1 || len(app.state.Jobs) != 0 {
		t.Fatal("重试耗尽未进入死信")
	}
	rows, err := app.SnapshotRows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	restored := newTestEngine(t)
	if err := restored.RestoreRows(ctx, rows); err != nil {
		t.Fatal(err)
	}
	if len(restored.state.DeadLetters[id].VariablePayload) == 0 {
		t.Fatal("死信载荷在行恢复后丢失")
	}
	restored.state.DeadLetters[id].VariablePayload = original
	if err := restored.MoveDeadLetterJobToExecutable(ctx, id, 2); err != nil {
		t.Fatal(err)
	}
	if count, err := restored.RunDueJobs(ctx); err != nil || count != 1 {
		t.Fatalf("死信重放: %d %v", count, err)
	}
	if value, err := restored.GetVariable(ctx, instance.ID, "b"); err != nil || !reflect.DeepEqual(value, []byte("恢复值")) {
		t.Fatalf("重放后值错误: %v %v", value, err)
	}
}

// TestAsyncVariablesTenantExecutor 验证 HTTP 绑定租户只能执行本租户作业，未绑定后台扫描可覆盖全库。
// @param t 测试上下文
// @return 无
func TestAsyncVariablesTenantExecutor(t *testing.T) {
	app := newTestEngine(t)
	for _, id := range []string{"a", "b"} {
		ctx := tenant.With(context.Background(), id)
		if _, err := app.Deploy(ctx, api.DeploymentRequest{Resources: map[string][]byte{"test.bpmn20.xml": []byte(simpleProcess)}}); err != nil {
			t.Fatal(err)
		}
		instance, err := app.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "simple"})
		if err != nil {
			t.Fatal(err)
		}
		if err := app.SetVariablesAsync(ctx, api.AsyncVariableRequest{ProcessInstanceID: instance.ID, Variables: []api.Variable{{Name: "pending", Value: true}}}); err != nil {
			t.Fatal(err)
		}
	}
	if count, err := app.RunDueJobs(tenant.With(context.Background(), "a")); err != nil || count != 1 {
		t.Fatalf("租户a执行了其他租户作业: %d %v", count, err)
	}
	if count, err := app.RunDueJobs(tenant.With(context.Background(), "")); err != nil || count != 0 {
		t.Fatalf("默认租户执行了其他租户作业: %d %v", count, err)
	}
	if count, err := app.RunDueJobs(context.Background()); err != nil || count != 1 {
		t.Fatalf("后台未执行剩余租户作业: %d %v", count, err)
	}
}

// TestAsyncVariablesGlobalAndCondition 验证 global 写入父作用域以及变量作业触发条件流程续跑。
// @param t 测试上下文
// @return 无
func TestAsyncVariablesGlobalAndCondition(t *testing.T) {
	ctx := context.Background()
	app := newTestEngine(t)
	deployXML(t, app, simpleProcess)
	instance, err := app.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "simple", Variables: []api.Variable{{Name: "x", Value: "父值"}}})
	if err != nil {
		t.Fatal(err)
	}
	root, _ := app.rootExecution(app.state.Instances[instance.ID])
	app.state.Executions["child"] = &executionState{ID: "child", ParentID: root.ID, ProcessInstanceID: instance.ID, DefinitionID: root.DefinitionID, Variables: map[string]any{"x": "子值"}}
	if err := app.SetVariablesAsync(ctx, api.AsyncVariableRequest{ExecutionID: "child", Variables: []api.Variable{{Name: "x", Value: "父更新", Scope: "global"}, {Name: "y", Value: true, Scope: "global"}}}); err != nil {
		t.Fatal(err)
	}
	if count, err := app.RunDueJobs(ctx); err != nil || count != 1 {
		t.Fatalf("global作业: %d %v", count, err)
	}
	if root.Variables["x"] != "父更新" || root.Variables["y"] != true || app.state.Executions["child"].Variables["x"] != "子值" {
		t.Fatal("global异步更新污染了局部作用域")
	}
	conditional, err := New(Config{HistoryLevel: HistoryFull})
	if err != nil {
		t.Fatal(err)
	}
	deployXML(t, conditional, conditionalCatchXML)
	waiting, err := conditional.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "conditionalCatch", Variables: []api.Variable{{Name: "ready", Value: false}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := conditional.SetVariablesAsync(ctx, api.AsyncVariableRequest{ProcessInstanceID: waiting.ID, Variables: []api.Variable{{Name: "ready", Value: true}}}); err != nil {
		t.Fatal(err)
	}
	if count, err := conditional.RunDueJobs(ctx); err != nil || count != 1 {
		t.Fatalf("条件异步作业: %d %v", count, err)
	}
	if process, err := conditional.GetHistoricProcessInstance(ctx, waiting.ID); err != nil || !process.Ended {
		t.Fatalf("异步变量未触发条件流程: %+v %v", process, err)
	}
}

// TestAsyncPayloadRollbackIsolation 验证命令备份不与待执行或死信作业共享可变载荷。
// @param t 测试上下文
// @return 无
func TestAsyncPayloadRollbackIsolation(t *testing.T) {
	source := newState()
	source.Jobs["pending"] = &jobState{ID: "pending", VariablePayload: []byte{1, 2}}
	source.DeadLetters["dead"] = &jobState{ID: "dead", VariablePayload: []byte{3, 4}}
	backup := cloneState(source)
	backup.Jobs["pending"].VariablePayload[0] = 9
	backup.DeadLetters["dead"].VariablePayload[0] = 9
	if source.Jobs["pending"].VariablePayload[0] != 1 || source.DeadLetters["dead"].VariablePayload[0] != 3 {
		t.Fatal("回滚备份共享异步载荷")
	}
}

// TestAsyncCancellationAndSaveFailure 验证取消和保存失败不能发布变量作业，挂起实例必须等待激活。
// @param t 测试上下文
// @return 无
func TestAsyncCancellationAndSaveFailure(t *testing.T) {
	ctx := context.Background()
	app := newTestEngine(t)
	deployXML(t, app, simpleProcess)
	instance, err := app.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "simple"})
	if err != nil {
		t.Fatal(err)
	}
	request := api.AsyncVariableRequest{ProcessInstanceID: instance.ID, Variables: []api.Variable{{Name: "x", Value: 1}}}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := app.SetVariablesAsync(cancelled, request); !errors.Is(err, context.Canceled) {
		t.Errorf("取消请求仍排队: %v", err)
	}
	app.cfg.StateStore = asyncSaveFailureStore{}
	if err := app.SetVariablesAsync(ctx, request); err == nil {
		t.Fatal("保存失败未返回错误")
	}
	if len(app.state.Jobs) != 0 {
		t.Fatal("取消或保存失败后仍有可执行作业")
	}
	app.cfg.StateStore = nil
	if err := app.SetVariablesAsync(ctx, request); err != nil {
		t.Fatal(err)
	}
	selected, err := app.dueJobIDs(ctx)
	if err != nil || len(selected) != 1 {
		t.Fatalf("挂起前选择作业失败: %v %v", selected, err)
	}
	if err := app.SuspendProcessInstance(ctx, instance.ID); err != nil {
		t.Fatal(err)
	}
	// 复现调度器已选中、真正执行前实例才挂起的命令交错。
	if count, err := app.runOneJob(ctx, selected[0]); err != nil || count != 0 {
		t.Fatalf("选择后挂起仍执行变量作业: %d %v", count, err)
	}
	if count, err := app.RunDueJobs(ctx); err != nil || count != 0 {
		t.Fatalf("挂起实例变量作业执行了: %d %v", count, err)
	}
	if err := app.ActivateProcessInstance(ctx, instance.ID); err != nil {
		t.Fatal(err)
	}
	if count, err := app.RunDueJobs(ctx); err != nil || count != 1 {
		t.Fatalf("激活后变量作业未执行: %d %v", count, err)
	}
}

// asyncSaveFailureStore 模拟提交阶段存储故障，验证变量作业不越过持久化边界。
type asyncSaveFailureStore struct{}

// Load 返回空快照。
// @param ctx 请求上下文
// @return 空快照
// @return 无错误
func (asyncSaveFailureStore) Load(ctx context.Context) ([]byte, error) { return nil, nil }

// Save 模拟存储写入失败。
// @param ctx 请求上下文
// @param data 待保存快照
// @return 写入错误
func (asyncSaveFailureStore) Save(ctx context.Context, data []byte) error { return context.Canceled }
