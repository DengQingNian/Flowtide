// facade_mgmt_guard_test.go 覆盖管理门面的装配守卫与参数分支（016 号审计覆盖率补齐）。
package workflow

import (
	"context"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
)

// TestFacadeMgmtGuards_NoEngine 验证管理门面在未装配引擎时统一返回 WORKFLOW-0601。
// @param t 测试上下文
func TestFacadeMgmtGuards_NoEngine(t *testing.T) {
	ctx := context.Background()
	svc := &Service{comp: &components{}}
	cases := []struct {
		name string
		call func() (R, error)
	}{
		{"AddModelCategory", func() (R, error) { return svc.AddModelCategory(ctx, &ModelCategory{}) }},
		{"EditModelCategory", func() (R, error) { return svc.EditModelCategory(ctx, &ModelCategory{}) }},
		{"DeleteModelCategory", func() (R, error) { return svc.DeleteModelCategory(ctx, []string{"m1"}) }},
		{"ListModelCategory", func() (R, error) { return svc.ListModelCategory(ctx, "sys") }},
		{"GetModelCategoryTree", func() (R, error) { return svc.GetModelCategoryTree(ctx, "sys") }},
		{"GetModelCategoryByID", func() (R, error) { return svc.GetModelCategoryByID(ctx, "m1") }},
		{"GetModelCategoryByCode", func() (R, error) { return svc.GetModelCategoryByCode(ctx, "sys", "flow") }},
		{"AddStepDesign", func() (R, error) { return svc.AddStepDesign(ctx, &StepDesign{}) }},
		{"EditStepDesign", func() (R, error) { return svc.EditStepDesign(ctx, &StepDesign{}) }},
		{"DeleteStepDesign", func() (R, error) { return svc.DeleteStepDesign(ctx, []string{"s1"}) }},
		{"ListStepDesignByModel", func() (R, error) { return svc.ListStepDesignByModel(ctx, "m1") }},
		{"AddTemplateDef", func() (R, error) { return svc.AddTemplateDef(ctx, &TemplateDef{}) }},
		{"EditTemplateDef", func() (R, error) { return svc.EditTemplateDef(ctx, &TemplateDef{}) }},
		{"DeleteTemplateDef", func() (R, error) { return svc.DeleteTemplateDef(ctx, []string{"t1"}) }},
		{"ListTemplateDefs", func() (R, error) { return svc.ListTemplateDefs(ctx, "sys") }},
		{"DeployModel", func() (R, error) { return svc.DeployModel(ctx, "sys", "m1") }},
		{"ListHisSteps", func() (R, error) { return svc.ListHisSteps(ctx, "d1") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("装配守卫不应 panic: %v", recovered)
				}
			}()
			_, err := tc.call()
			if err == nil {
				t.Fatalf("未装配时应返回错误")
			}
			var apiErr *api.Error
			if !errorsAs(err, &apiErr) {
				t.Fatalf("应返回 api.Error，实际 %T", err)
			}
			if apiErr.Code != "WORKFLOW-0601" {
				t.Fatalf("期望 WORKFLOW-0601，实际 %s", apiErr.Code)
			}
		})
	}
}

// errorsAs 是 errors.As 的本地短名包装（避免测试文件重复 import errors）。
// @param err 错误
// @param target 目标指针
// @return 是否匹配
func errorsAs(err error, target **api.Error) bool {
	apiErr, ok := err.(*api.Error)
	if !ok {
		return false
	}
	*target = apiErr
	return true
}
