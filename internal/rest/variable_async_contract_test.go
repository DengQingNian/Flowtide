package rest

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/engine"
)

// TestAsyncVariablesRESTContract 验证官方异步变量状态码、延迟生效、冲突、单对象及整数精度。
// @param t 测试上下文
// @return 无
func TestAsyncVariablesRESTContract(t *testing.T) {
	app, err := engine.New(engine.Config{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := app.Deploy(ctx, api.DeploymentRequest{Resources: map[string][]byte{"rest.bpmn20.xml": []byte(restProcess)}}); err != nil {
		t.Fatal(err)
	}
	instance, err := app.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "rest", Variables: []api.Variable{{Name: "initiator", Value: "alice"}}})
	if err != nil {
		t.Fatal(err)
	}
	handler := New(app).Handler()
	base := "/service/runtime/process-instances/" + instance.ID + "/variables-async"
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"POST", base, `[{"name":"large","type":"long","value":9007199254740993}]`, 201},
		{"POST", base, `[{"name":"new","value":1},{"name":"initiator","value":"conflict"}]`, 409},
		{"PUT", base, `[{"name":"pending","value":"稍后"}]`, 201},
		{"PUT", base + "/initiator", `{"name":"initiator","value":"bob"}`, 204},
		{"PUT", base + "/missing", `{"name":"missing","value":1}`, 404},
		{"PUT", base + "/initiator", `{"name":"other","value":1}`, 400},
		{"POST", base, `{} {}`, 400},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		r.Header.Set("Content-Type", "application/json")
		handler.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Errorf("%s %s: %d %s，期望%d", tc.method, tc.path, w.Code, w.Body.String(), tc.status)
		}
	}
	if value, err := app.GetVariable(ctx, instance.ID, "initiator"); err != nil || value != "alice" {
		t.Errorf("异步请求提前改值: %v %v", value, err)
	}
	if _, err := app.GetVariable(ctx, instance.ID, "pending"); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("异步请求提前创建变量: %v", err)
	}
	if jobs, err := app.ListJobs(ctx); err != nil || len(jobs) != 3 {
		t.Fatalf("异步排队数量: %+v %v", jobs, err)
	}
	if count, err := app.RunDueJobs(ctx); err != nil || count != 3 {
		t.Fatalf("异步作业执行: %d %v", count, err)
	}
	if value, err := app.GetVariable(ctx, instance.ID, "large"); err != nil || value != int64(9007199254740993) {
		t.Errorf("异步大整数精度丢失: %T %v %v", value, value, err)
	}
	if value, err := app.GetVariable(ctx, instance.ID, "initiator"); err != nil || value != "bob" {
		t.Errorf("异步更新未生效: %v %v", value, err)
	}
	if _, err := app.GetVariable(ctx, instance.ID, "new"); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("冲突批次部分生效: %v", err)
	}
}
