package migration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/engine"
	statestore "github.com/flowable-go/flowable/internal/persistence/state"
	"github.com/flowable-go/flowable/internal/tenant"
)

// verifyStateFixture 以实际引擎生成包含部署、定义、运行实例、任务与历史的离线状态。
// @param t 测试上下文
// @param tenantID 夹具租户，空字符串表示默认租户
// @return 引擎写出的状态字节
func verifyStateFixture(t *testing.T, tenantID string) []byte {
	t.Helper()
	store := statestore.NewMemory()
	workflow, err := engine.New(engine.Config{StateStore: store, HistoryLevel: engine.HistoryAudit, RecordSequenceFlowHistory: true, TenantID: tenantID})
	if err != nil {
		t.Fatal(err)
	}
	ctx := tenant.With(context.Background(), tenantID)
	xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="verify" isExecutable="true"><startEvent id="start"/><sequenceFlow id="a" sourceRef="start" targetRef="first"/><userTask id="first"/><sequenceFlow id="b" sourceRef="first" targetRef="second"/><userTask id="second"/><sequenceFlow id="c" sourceRef="second" targetRef="end"/><endEvent id="end"/></process></definitions>`
	if _, err := workflow.Deploy(ctx, api.DeploymentRequest{TenantID: tenantID, Resources: map[string][]byte{"verify.bpmn20.xml": []byte(xml)}}); err != nil {
		t.Fatal(err)
	}
	start := func(key string) string {
		instance, err := workflow.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "verify", TenantID: tenantID, BusinessKey: key, Variables: []api.Variable{{Name: "amount", Value: int64(42)}, {Name: "label", Value: key}}})
		if err != nil {
			t.Fatal(err)
		}
		return instance.ID
	}
	// 第一个实例停留在 second 任务，用于产生运行实例与当前任务。
	runningID := start("running")
	// 第二个实例完成后进入历史，用于产生已结束实例。
	endedID := start("ended")
	completeTask := func(instanceID, taskDefinitionKey string) {
		tasks, err := workflow.ListTasks(ctx, api.TaskFilter{ProcessInstanceID: instanceID})
		if err != nil {
			t.Fatal(err)
		}
		for _, task := range tasks.Data {
			if task.TaskDefinitionKey == taskDefinitionKey {
				if err := workflow.Complete(ctx, task.ID, api.CompleteTaskRequest{}); err != nil {
					t.Fatal(err)
				}
				return
			}
		}
		t.Fatalf("实例 %s 缺少任务 %s", instanceID, taskDefinitionKey)
	}
	completeTask(runningID, "first")
	completeTask(endedID, "first")
	completeTask(endedID, "second")
	data, err := store.Load(context.Background())
	if err != nil || len(data) == 0 {
		t.Fatalf("读取夹具状态失败：%d %v", len(data), err)
	}
	return data
}

// TestVerifyStateReport 验证完整状态的恢复报告、数量、摘要及输入只读性。
// @param t 测试上下文
// @return 无
func TestVerifyStateReport(t *testing.T) {
	data := verifyStateFixture(t, "")
	before := append([]byte(nil), data...)
	report, err := VerifyState(context.Background(), data)
	if err != nil {
		t.Fatal(err)
	}
	// 状态包含一个部署、一个定义、一个运行实例和一个已结束实例。
	if report.Format != StateFormat || report.Version != 2 {
		t.Fatalf("格式或版本错误：%+v", report)
	}
	if report.Deployments != 1 || report.Definitions != 1 {
		t.Fatalf("部署或定义数量错误：%+v", report)
	}
	if report.RunningInstances != 1 || report.EndedInstances != 1 {
		t.Fatalf("实例数量错误：%+v", report)
	}
	// 两个实例各完成 first，第二个实例还完成 second，因此历史任务共四条。
	if report.Tasks != 1 || report.HistoricProcessInstances != 2 || report.HistoricTaskInstances != 4 {
		t.Fatalf("任务或历史数量错误：%+v", report)
	}
	// start 请求各写入两个流程变量，两个实例合计四条历史变量。
	if report.HistoricVariables != 4 {
		t.Fatalf("历史变量数量错误：%+v", report)
	}
	if !report.SequenceFlowHistory || len(report.Limitations) == 0 {
		t.Fatalf("状态标记或边界说明缺失：%+v", report)
	}
	sum := sha256.Sum256(data)
	if report.Bytes != len(data) || report.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("摘要或字节数错误：%+v", report)
	}
	// 校验不修改输入字节，调用方可继续把同一字节写回原文件。
	if !bytes.Equal(before, data) {
		t.Fatal("校验修改了输入状态字节")
	}
}

// TestVerifyStateTenantReport 验证显式租户状态在未指定租户时仍被完整统计。
// @param t 测试上下文
// @return 无
func TestVerifyStateTenantReport(t *testing.T) {
	data := verifyStateFixture(t, "租户甲")
	report, err := VerifyState(context.Background(), data)
	if err != nil {
		t.Fatal(err)
	}
	// 校验命令没有 -tenant，必须覆盖状态中出现的全部租户。
	if report.Deployments != 1 || report.Definitions != 1 || report.RunningInstances != 1 || report.EndedInstances != 1 || report.Tasks != 1 {
		t.Fatalf("租户状态统计不完整：%+v", report)
	}
	if report.HistoricProcessInstances != 2 || report.HistoricTaskInstances != 4 || report.HistoricVariables != 4 {
		t.Fatalf("租户历史统计不完整：%+v", report)
	}
}

// TestVerifyStateEmptyValidState 验证不包含实体的合法旧版状态可以恢复并报告零数量。
// @param t 测试上下文
// @return 无
func TestVerifyStateEmptyValidState(t *testing.T) {
	data := []byte(`{"version":0}`)
	report, err := VerifyState(context.Background(), data)
	if err != nil {
		t.Fatal(err)
	}
	if report.Format != StateFormat || report.Version != 0 || report.Bytes != len(data) || report.SequenceFlowHistory {
		t.Fatalf("空状态报告错误：%+v", report)
	}
	if report.Deployments != 0 || report.Definitions != 0 || report.RunningInstances != 0 || report.EndedInstances != 0 || report.Tasks != 0 || report.HistoricProcessInstances != 0 || report.HistoricTaskInstances != 0 || report.HistoricVariables != 0 {
		t.Fatalf("空状态数量非零：%+v", report)
	}
}

// TestVerifyStateRejectsInvalidInput 验证空文件、非法 JSON、版本、损坏类型与取消均返回错误。
// @param t 测试上下文
// @return 无
func TestVerifyStateRejectsInvalidInput(t *testing.T) {
	valid := verifyStateFixture(t, "")
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	cases := []struct {
		name string          // 用例名称
		ctx  context.Context // 校验上下文
		data []byte          // 状态字节
	}{
		{name: "空字节", ctx: context.Background(), data: nil},
		{name: "仅空白", ctx: context.Background(), data: []byte(" \n\t")},
		{name: "非法JSON", ctx: context.Background(), data: []byte("{")},
		{name: "负版本", ctx: context.Background(), data: []byte(`{"version":-1}`)},
		{name: "未知新版本", ctx: context.Background(), data: []byte(`{"version":3}`)},
		{name: "损坏部署类型", ctx: context.Background(), data: []byte(`{"version":2,"deployments":"bad"}`)},
		{name: "损坏租户类型", ctx: context.Background(), data: []byte(`{"version":2,"definitions":[{"info":{"tenantId":5}}]}`)},
		{name: "已取消", ctx: cancelled, data: valid},
	}
	for _, testCase := range cases {
		report, err := VerifyState(testCase.ctx, testCase.data)
		// 任何失败都必须返回零值报告，避免调用方把部分统计当作完整结论。
		if err == nil {
			t.Fatalf("%s 未返回错误：%+v", testCase.name, report)
		}
		if report.Format != "" || report.Version != 0 || report.Bytes != 0 || report.SHA256 != "" || report.Deployments != 0 || report.HistoricProcessInstances != 0 || report.Limitations != nil {
			t.Fatalf("%s 返回了部分报告：%+v", testCase.name, report)
		}
	}
}
