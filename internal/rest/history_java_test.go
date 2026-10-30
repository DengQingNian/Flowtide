package rest

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
	"github.com/DengQingNian/Flowtide/internal/engine"
)

type historyJavaFixtureData struct {
	Baseline    string            `json:"baseline"`
	Clock       time.Time         `json:"clock"`
	StartUserID string            `json:"startUserId"`
	Models      map[string]string `json:"models"`
	Cases       []historyJavaCase `json:"cases"`
}

// historyJavaCase 保存原版历史请求、完整响应及历史实体计数。
type historyJavaCase struct {
	Name         string           `json:"name"`         // 场景名
	Method       string           `json:"method"`       // HTTP 方法
	Path         string           `json:"path"`         // 路径模板
	RequestBody  string           `json:"requestBody"`  // 请求模板
	Status       int              `json:"status"`       // 原版状态码
	ResponseBody json.RawMessage  `json:"responseBody"` // 原版完整正文，二进制为字节标记
	AfterCounts  map[string]int64 `json:"afterCounts"`  // 原版历史实体计数
}

// TestJavaHistoryRESTReference 对照原版历史控制器的完整契约及历史副作用。
// @param t 测试上下文
// @return 无
func TestJavaHistoryRESTReference(t *testing.T) {
	data, err := os.ReadFile("../../testdata/compatibility/java/007-history-rest.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture historyJavaFixtureData
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Baseline != "f6017e32a4bdd3e59a8881f58b0ee181652d1d9e" || len(fixture.Cases) != 44 {
		t.Fatal("历史Java基线或场景数错误")
	}
	ctxHistoryJava := engine.WithActor(context.Background(), fixture.StartUserID)
	app, handler, fake, definitionID := newHistoryJavaApp(t, fixture)
	for _, scenario := range fixture.Cases {
		t.Run(scenario.Name, func(t *testing.T) {
			ids := buildHistoryJavaFixture(t, fixture, app, handler, fake, definitionID)
			path, body := scenario.Path, scenario.RequestBody
			createdBefore, _ := app.ListProcessComments(ctxHistoryJava, ids["{runningId}"])
			for label, id := range ids {
				path = strings.ReplaceAll(path, label, id)
				body = strings.ReplaceAll(body, label, id)
			}
			response := historyJavaRequest(handler, scenario.Method, path, body)
			// 新建评论的动态标识统一为占位符。
			if scenario.Name == "proc-comment-new" {
				after, _ := app.ListProcessComments(ctxHistoryJava, ids["{runningId}"])
				for _, comment := range after {
					known := false
					for _, before := range createdBefore {
						if comment.ID == before.ID {
							known = true
						}
					}
					if !known {
						ids["{createdCommentId}"] = comment.ID
					}
				}
			}
			if response.Code != scenario.Status {
				t.Errorf("状态码%d，原版%d", response.Code, scenario.Status)
			}
			var expected any
			if err := json.Unmarshal(scenario.ResponseBody, &expected); err != nil {
				t.Fatal(err)
			}
			if bytesRaw, ok := expected.(map[string]any); ok {
				// 二进制响应按字节标记无损比较。
				encoded, ok := bytesRaw["$bytes"].(string)
				if !ok || len(bytesRaw) != 1 {
					t.Fatal("未知二进制标记")
				}
				want, err := base64.StdEncoding.DecodeString(encoded)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(response.Body.Bytes(), want) {
					t.Fatalf("二进制正文不同: 实际%s 原版%s", response.Body.String(), encoded)
				}
			} else {
				actual := runtimeJavaNormalize(response.Body.String(), ids)
				if scenario.Name == "proc-comment-new" {
				}
				if text, isText := expected.(string); isText {
					if text == "" {
						if strings.TrimSpace(actual) != "" {
							t.Errorf("原版空正文被改为%s", actual)
						}
					} else {
						assertRuntimeJSON(t, []byte(actual), []byte(text))
					}
				} else {
					t.Fatalf("未知响应标记: %s", scenario.ResponseBody)
				}
			}
			assertHistoryJavaCounts(t, app, scenario.AfterCounts, ids)
		})
	}
}

// buildHistoryJavaFixture 以与 Java 运行器相同的时钟步进重建历史夹具。
// @param t 测试上下文
// @param fixture 参考制品
// @param app 共享引擎
// @param handler 共享处理器
// @param fake 可控时钟
// @param definitionID 流程定义标识
// @return 实际标识到占位符
func buildHistoryJavaFixture(t *testing.T, fixture historyJavaFixtureData, app *engine.Engine, handler http.Handler, fake *clock.Fake, definitionID string) map[string]string {
	t.Helper()
	ctx := engine.WithActor(context.Background(), fixture.StartUserID)
	base := fixture.Clock.Truncate(time.Millisecond)
	// 复位到基准时间，使每个场景的时间线与 Java 运行器一致。
	fake.Set(base)
	// 清空上一场景的运行时实例及其历史，计数器保持续号与 Java 一致。
	for _, instance := range mustListInstances(t, app) {
		if err := app.DeleteProcessInstance(ctx, instance.ID, "reset"); err != nil {
			t.Fatal(err)
		}
	}
	for _, historic := range mustListHistoric(t, app) {
		if err := app.DeleteHistoricProcessInstance(ctx, historic.ID); err != nil {
			t.Fatal(err)
		}
	}
	ids := map[string]string{"{definitionId}": definitionID}
	// 已结束实例：候选、签收、局部变量、流程评论与变量更新都会产生历史行。
	ended, err := app.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "history_reference", Name: "历史甲", BusinessKey: "alpha", UserID: fixture.StartUserID, Variables: []api.Variable{
		{Name: "amount", Value: int64(9007199254740993)},
		{Name: "flag", Value: "before"},
		{Name: "createdAt", Value: time.Date(2026, 2, 3, 4, 5, 6, 789000000, time.UTC)},
		{Name: "payload", Value: []byte{1, 2, 0, 255}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	ids["{endedId}"] = ended.ID
	executions, err := app.ListExecutions(ctx, ended.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, execution := range executions {
		if execution.ParentID == ended.ID && !execution.Ended {
			ids["{endedExecutionId}"] = execution.ID
		}
	}
	endedApprove := historyJavaTask(t, app, ended.ID, "approve")
	fake.Set(base.Add(time.Hour))
	if err := app.AddCandidateUser(ctx, endedApprove.ID, "carol"); err != nil {
		t.Fatal(err)
	}
	if err := app.AddCandidateGroup(ctx, endedApprove.ID, "review"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.UpdateTask(ctx, endedApprove.ID, api.TaskUpdate{Assignee: &historyJavaAlice}); err != nil {
		t.Fatal(err)
	}
	if err := app.SetTaskVariable(ctx, endedApprove.ID, "localPayload", []byte{9, 0, 250}); err != nil {
		t.Fatal(err)
	}
	if err := app.SetTaskVariable(ctx, endedApprove.ID, "localNote", "局部备注"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.AddInstanceIdentityLink(ctx, ended.ID, api.IdentityLink{Type: "participant", UserID: "bob"}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.AddProcessComment(ctx, ended.ID, fixture.StartUserID, "评审意见一"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.AddProcessComment(ctx, ended.ID, fixture.StartUserID, "评审意见二"); err != nil {
		t.Fatal(err)
	}
	fake.Set(base.Add(2 * time.Hour))
	if err := app.SetVariable(ctx, ended.ID, "flag", "after"); err != nil {
		t.Fatal(err)
	}
	fake.Set(base.Add(2*time.Hour + time.Second))
	if err := app.SetVariable(ctx, ended.ID, "amount", int64(43)); err != nil {
		t.Fatal(err)
	}
	fake.Set(base.Add(2*time.Hour + time.Minute))
	if err := app.Complete(ctx, endedApprove.ID, api.CompleteTaskRequest{}); err != nil {
		t.Fatal(err)
	}
	endedReview := historyJavaTask(t, app, ended.ID, "review")
	fake.Set(base.Add(3 * time.Hour))
	if err := app.Complete(ctx, endedReview.ID, api.CompleteTaskRequest{}); err != nil {
		t.Fatal(err)
	}
	// 运行中实例：停在审批任务并保持一条评论。
	fake.Set(base.Add(4 * time.Hour))
	running, err := app.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "history_reference", Name: "历史乙", BusinessKey: "beta", UserID: fixture.StartUserID, Variables: []api.Variable{{Name: "amount", Value: int64(42)}, {Name: "flag", Value: "before"}}})
	if err != nil {
		t.Fatal(err)
	}
	ids["{runningId}"] = running.ID
	runningApprove := historyJavaTask(t, app, running.ID, "approve")
	ids["{runningExecutionId}"] = runningApprove.ExecutionID
	if _, err := app.UpdateTask(ctx, runningApprove.ID, api.TaskUpdate{Assignee: &historyJavaAlice}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.AddProcessComment(ctx, running.ID, fixture.StartUserID, "运行意见"); err != nil {
		t.Fatal(err)
	}
	ids["{runningApproveTaskId}"] = runningApprove.ID
	ids["{endedApproveTaskId}"] = endedApprove.ID
	ids["{endedReviewTaskId}"] = endedReview.ID
	detailID, varInstanceID := historyJavaPayloadIDs(t, app, ended.ID)
	ids["{detailId}"] = detailID
	ids["{varInstanceId}"] = varInstanceID
	comments, err := app.ListProcessComments(ctx, ended.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 3 {
		t.Fatalf("Java 夹具评论数不同: %d", len(comments))
	}
	ids["{commentId}"] = comments[len(comments)-1].ID
	// 其余活动、变量、明细、评论标识按查询默认顺序编号，与 Java 运行器一致。
	index := 0
	activities, err := app.ListHistoricActivityInstances(ctx, ended.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, activity := range activities {
		value := activity.ID
		index++
		label := fmt.Sprintf("{endedActivity%d}", index)
		if _, taken := ids[label]; !taken && !sweepValues(ids, value) {
			ids[label] = value
		}
	}
	index = 0
	runningActivities, err := app.ListHistoricActivityInstances(ctx, running.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, activity := range runningActivities {
		value := activity.ID
		index++
		label := fmt.Sprintf("{runningActivity%d}", index)
		if _, taken := ids[label]; !taken && !sweepValues(ids, value) {
			ids[label] = value
		}
	}
	index = 0
	variables, err := app.ListHistoricVariableInstances(ctx, ended.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, variable := range variables {
		scope := "global"
		if variable.TaskID != "" {
			scope = "local"
		}
		label := "{endedVar-" + variable.Name + "-" + scope + "}"
		if _, taken := ids[label]; !taken && !sweepValues(ids, variable.ID) {
			ids[label] = variable.ID
		}
	}
	index = 0
	runningVariables, err := app.ListHistoricVariableInstances(ctx, running.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, variable := range runningVariables {
		scope := "global"
		if variable.TaskID != "" {
			scope = "local"
		}
		label := "{runningVar-" + variable.Name + "-" + scope + "}"
		if _, taken := ids[label]; !taken && !sweepValues(ids, variable.ID) {
			ids[label] = variable.ID
		}
	}
	index = 0
	details, err := app.ListHistoricDetails(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	detailCounts := map[string]int{}
	for _, detail := range details {
		detailCounts[detail.Name]++
		label := fmt.Sprintf("{detail-%s-%d}", detail.Name, detailCounts[detail.Name])
		if _, taken := ids[label]; !taken && !sweepValues(ids, detail.ID) {
			ids[label] = detail.ID
		}
	}
	index = 0
	for _, comment := range comments {
		value := comment.ID
		index++
		label := fmt.Sprintf("{endedComment%d}", index)
		if _, taken := ids[label]; !taken && !sweepValues(ids, value) {
			ids[label] = value
		}
	}
	index = 0
	runningComments, err := app.ListProcessComments(ctx, running.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, comment := range runningComments {
		value := comment.ID
		index++
		label := fmt.Sprintf("{runningComment%d}", index)
		if _, taken := ids[label]; !taken && !sweepValues(ids, value) {
			ids[label] = value
		}
	}
	// 临时探针：输出运行实例链接的时间线
	links, _ := app.ListHistoricProcessIdentityLinks(ctx, ids["{runningId}"])
	for _, link := range links {
		t.Logf("探针: %s %s @%s", link.Type, link.UserID, link.CreateTime.Format("15:04:05"))
	}
	return ids
}

// mustListInstances 返回全部运行时实例。
// @param t 测试上下文
// @param app 引擎
// @return 实例列表
func mustListInstances(t *testing.T, app *engine.Engine) []api.ProcessInstance {
	t.Helper()
	instances, err := app.ListProcessInstances(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return instances
}

// mustListHistoric 返回全部历史流程实例。
// @param t 测试上下文
// @param app 引擎
// @return 历史实例列表
func mustListHistoric(t *testing.T, app *engine.Engine) []api.HistoricProcessInstance {
	t.Helper()
	instances, err := app.ListHistoricProcessInstances(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return instances
}

// newHistoryJavaApp 创建共享引擎与处理器；任务日志等计数器跨场景保持续号。
// @param t 测试上下文
// @param fixture 参考制品
// @return 引擎
// @return HTTP 处理器
// @return 可控时钟
// @return 流程定义标识
func newHistoryJavaApp(t *testing.T, fixture historyJavaFixtureData) (*engine.Engine, http.Handler, *clock.Fake, string) {
	t.Helper()
	ctx := engine.WithActor(context.Background(), fixture.StartUserID)
	base := fixture.Clock.Truncate(time.Millisecond)
	fake := clock.NewFake(base)
	app, err := engine.New(engine.Config{Clock: fake, IDs: &historyJavaIDs{}, HistoryLevel: engine.HistoryFull})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := app.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	resources := map[string][]byte{}
	for name, xml := range fixture.Models {
		resources[name] = []byte(xml)
	}
	if _, err := app.Deploy(ctx, api.DeploymentRequest{Resources: resources}); err != nil {
		t.Fatal(err)
	}
	definitionID := ""
	definitions, err := app.ListProcessDefinitions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range definitions {
		if definition.Key == "history_reference" {
			definitionID = definition.ID
		}
	}
	handler := New(app, WithBasicAuth(map[string]string{fixture.StartUserID: "reference-secret"})).Handler()
	return app, handler, fake, definitionID
}

// historyJavaTask 按定义键返回夹具中的当前任务。
// @param t 测试上下文
// @param app 引擎
// @param processInstanceID 实例标识
// @param definitionKey 任务定义键
// @return 当前任务
func historyJavaTask(t *testing.T, app *engine.Engine, processInstanceID, definitionKey string) api.Task {
	t.Helper()
	tasks, err := app.ListTasks(context.Background(), api.TaskFilter{ProcessInstanceID: processInstanceID})
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range tasks.Data {
		if task.TaskDefinitionKey == definitionKey {
			return task
		}
	}
	t.Fatalf("夹具缺少任务: %s", definitionKey)
	return api.Task{}
}

// historyJavaPayloadIDs 返回已结束实例的字节载荷明细与历史变量标识。
// @param t 测试上下文
// @param app 引擎
// @param processInstanceID 实例标识
// @return 字节载荷明细标识
// @return 字节载荷历史变量标识
func historyJavaPayloadIDs(t *testing.T, app *engine.Engine, processInstanceID string) (string, string) {
	t.Helper()
	details, err := app.ListHistoricDetails(context.Background(), processInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	detailID := ""
	for _, detail := range details {
		if detail.Name == "payload" {
			detailID = detail.ID
		}
	}
	if detailID == "" {
		t.Fatal("夹具缺少字节载荷明细")
	}
	variables, err := app.ListHistoricVariableInstances(context.Background(), processInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	for _, variable := range variables {
		if variable.Name == "payload" {
			return detailID, variable.ID
		}
	}
	t.Fatal("夹具缺少字节载荷历史变量")
	return "", ""
}

// historyJavaRequest 以固定认证主体执行参考请求。
// @param handler 正式处理链
// @param method 方法
// @param path 路径
// @param body 请求正文
// @return 录制的完整响应
func historyJavaRequest(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://localhost"+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.SetBasicAuth("starter", "reference-secret")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

// assertHistoryJavaCounts 核对历史实体计数与原版一致。
// @param t 测试上下文
// @param app 引擎
// @param expected 原版计数
// @param ids 占位符映射
func assertHistoryJavaCounts(t *testing.T, app *engine.Engine, expected map[string]int64, ids map[string]string) {
	t.Helper()
	ctx := context.Background()
	endedID := ids["{endedId}"]
	runningID := ids["{runningId}"]
	counts := map[string]int64{}
	processes, err := app.ListHistoricProcessInstances(ctx)
	if err != nil {
		t.Fatal(err)
	}
	counts["processInstances"] = int64(len(processes))
	endedTasks, err := app.ListHistoricTaskInstances(ctx, endedID)
	if err != nil {
		t.Fatal(err)
	}
	runningTasks, err := app.ListHistoricTaskInstances(ctx, runningID)
	if err != nil {
		t.Fatal(err)
	}
	counts["tasks"] = int64(len(endedTasks) + len(runningTasks))
	endedActivities, err := app.ListHistoricActivityInstances(ctx, endedID)
	if err != nil {
		t.Fatal(err)
	}
	runningActivities, err := app.ListHistoricActivityInstances(ctx, runningID)
	if err != nil {
		t.Fatal(err)
	}
	counts["activities"] = int64(len(endedActivities) + len(runningActivities))
	endedDetails, err := app.ListHistoricDetails(ctx, endedID)
	if err != nil {
		t.Fatal(err)
	}
	runningDetails, err := app.ListHistoricDetails(ctx, runningID)
	if err != nil {
		t.Fatal(err)
	}
	counts["details"] = int64(len(endedDetails) + len(runningDetails))
	endedVariables, err := app.ListHistoricVariableInstances(ctx, endedID)
	if err != nil {
		t.Fatal(err)
	}
	runningVariables, err := app.ListHistoricVariableInstances(ctx, runningID)
	if err != nil {
		t.Fatal(err)
	}
	counts["variables"] = int64(len(endedVariables) + len(runningVariables))
	comments, err := app.ListProcessComments(ctx, endedID)
	if err != nil {
		t.Fatal(err)
	}
	counts["comments"] = int64(len(comments))
	links, err := app.ListHistoricProcessIdentityLinks(ctx, endedID)
	if err != nil {
		t.Fatal(err)
	}
	counts["identityLinks"] = int64(len(links))
	logs, err := app.ListHistoricTaskLogs(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	counts["taskLogs"] = int64(len(logs))
	for key, want := range expected {
		if counts[key] != want {
			t.Errorf("历史计数%s: 实际%d 原版%d", key, counts[key], want)
		}
	}
}

// historyJavaAlice 是夹具签收人。
var historyJavaAlice = "alice"

// historyJavaIDs 与 Java 运行器一样生成单调标识，使默认标识排序能够逐项比较。
type historyJavaIDs struct {
	next int // 当前序号
}

// NewID 返回固定宽度的递增标识，不依赖宿主机时钟或随机数。
// @return 二十位标识
func (g *historyJavaIDs) NewID() string {
	g.next++
	return fmt.Sprintf("%020d", g.next)
}

// sweepValues 判断标识是否已被其他占位符占用。
// @param ids 占位符映射
// @param value 实际标识
// @return 是否已占用
func sweepValues(ids map[string]string, value string) bool {
	for _, existing := range ids {
		if existing == value {
			return true
		}
	}
	return false
}

// canonicalHistoryArrays 对原版链接、明细与变量数组按稳定键排序，不构成顺序契约。
// @param value 响应对象或数组
// @return 无
func canonicalHistoryArrays(value any) {
	switch typed := value.(type) {
	case []any:
		if len(typed) > 0 {
			if isLinkObject(typed[0]) {
				sort.SliceStable(typed, func(i, j int) bool { return arrayKey(typed[i], "type") < arrayKey(typed[j], "type") })
				return
			}
			if isDetailObject(typed[0]) {
				sort.SliceStable(typed, func(i, j int) bool { return detailKey(typed[i]) < detailKey(typed[j]) })
				return
			}
			if isVariableObject(typed[0]) {
				sort.SliceStable(typed, func(i, j int) bool { return variableKey(typed[i]) < variableKey(typed[j]) })
				return
			}
		}
		for _, item := range typed {
			canonicalHistoryArrays(item)
		}
	case map[string]any:
		for _, item := range typed {
			canonicalHistoryArrays(item)
		}
	}
}

// arrayKey 生成链接排序键。
// @param value 链接对象
// @param _ 保留参数

// isDetailObject 判断对象是否为原版明细结构。
// @param value 候选对象
// @return 是否明细
func isDetailObject(value any) bool {
	object, ok := value.(map[string]any)
	return ok && object["detailType"] != nil && object["variable"] != nil
}

// detailKey 生成明细排序键。
// @param value 明细对象
// @return 排序键
func detailKey(value any) string {
	object := value.(map[string]any)
	variable, _ := object["variable"].(map[string]any)
	return fmt.Sprint(object["time"], variable["name"])
}

// isVariableObject 判断对象是否为历史变量结构。
// @param value 候选对象
// @return 是否变量
func isVariableObject(value any) bool {
	object, ok := value.(map[string]any)
	return ok && object["variable"] != nil && object["detailType"] == nil
}

// variableKey 生成变量排序键。
// @param value 变量对象
// @return 排序键
func variableKey(value any) string {
	object := value.(map[string]any)
	variable, _ := object["variable"].(map[string]any)
	return fmt.Sprint(variable["name"], object["processInstanceId"], object["taskId"])
}

// canonicalHistoryArrays 对原版链接、明细与变量数组按稳定键排序，不构成顺序契约。
// @param value 响应对象或数组
// @return 无

// arrayKey 生成链接排序键。
// @param value 链接对象
// @param _ 保留参数
// @return 排序键
func arrayKey(value any, _ string) string {
	object := value.(map[string]any)
	return fmt.Sprint(object["type"], object["userId"], object["groupId"], object["taskId"], object["processInstanceId"])
}

// isDetailObject 判断对象是否为原版明细结构。
// @param value 候选对象

// detailKey 生成明细排序键。
// @param value 明细对象

// isVariableObject 判断对象是否为历史变量结构。
// @param value 候选对象

// variableKey 生成变量排序键。
// @param value 变量对象

// isLinkObject 判断对象是否为原版链接结构。
// @param value 候选对象
// @return 是否链接
func isLinkObject(value any) bool {
	object, ok := value.(map[string]any)
	if !ok {
		return false
	}
	for _, key := range []string{"type", "userId", "groupId", "taskId", "processInstanceId"} {
		if _, exists := object[key]; !exists {
			return false
		}
	}
	return true
}

// arrayKey 生成链接排序键。
// @param value 链接对象
// @param _ 保留参数
