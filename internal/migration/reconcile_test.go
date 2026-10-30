package migration

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/DengQingNian/Flowtide/internal/engine"
	statestore "github.com/DengQingNian/Flowtide/internal/persistence/state"
	"github.com/DengQingNian/Flowtide/internal/tenant"
)

// TestReconcileState 验证源快照与准备状态逐表核对，等量改值也必须发现。
// @param t 测试上下文
// @return 无
func TestReconcileState(t *testing.T) {
	ctx := context.Background()
	source := conversionFixture(t, "a")
	data, report, err := Convert(ctx, source, Options{})
	if err != nil || report.Blocked {
		t.Fatalf("准备夹具: %+v %v", report.Issues, err)
	}
	got, err := ReconcileState(ctx, source, data, Options{})
	if err != nil || !got.Matched || len(got.Tables) == 0 || got.StateSHA256 == "" {
		t.Fatalf("相同状态核对失败: %+v %v", got, err)
	}
	// 只改变变量值，不改变实例、任务或变量行数，不能仅靠数量放行。
	store := statestore.NewMemory()
	if err := store.Save(ctx, data); err != nil {
		t.Fatal(err)
	}
	app, err := engine.New(engine.Config{StateStore: store, HistoryLevel: engine.HistoryNone})
	if err != nil {
		t.Fatal(err)
	}
	tenantCtx := tenant.With(ctx, "a")
	instances, err := app.ListProcessInstances(tenantCtx)
	if err != nil || len(instances) != 1 {
		t.Fatalf("夹具实例: %+v %v", instances, err)
	}
	if err := app.SetVariable(tenantCtx, instances[0].ID, "amount", int64(9007199254740994)); err != nil {
		t.Fatal(err)
	}
	if err := app.Close(ctx); err != nil {
		t.Fatal(err)
	}
	changed, err := store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mismatch, err := ReconcileState(ctx, source, changed, Options{})
	if err != nil || mismatch.Matched {
		t.Fatalf("同数改值未阻断: %+v %v", mismatch, err)
	}
	encoded, _ := json.Marshal(mismatch)
	if bytes.Contains(encoded, []byte("敏感业务值")) {
		t.Fatal("核对报告泄露变量值")
	}
	if len(mismatch.ChangedTables) == 0 {
		t.Fatal("未返回差异表")
	}
}

// TestReconcileStateFailures 验证阻断源库、坏状态及取消不产生成功报告。
// @param t 测试上下文
// @return 无
func TestReconcileStateFailures(t *testing.T) {
	source := conversionFixture(t, "a")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ReconcileState(ctx, source, []byte(`{}`), Options{}); err == nil {
		t.Fatal("取消未失败")
	}
	if _, err := ReconcileState(context.Background(), source, []byte(`bad`), Options{}); err == nil {
		t.Fatal("坏状态未失败")
	}
	source.Rows[0].Values["非法字段"] = "不输出"
	source.SchemaVersion = "unsupported"
	if _, err := ReconcileState(context.Background(), source, []byte(`{}`), Options{}); err == nil {
		t.Fatal("预检阻断未失败")
	}
}
