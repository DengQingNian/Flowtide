package workflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
)

// mmTestStamp 固定测试时间戳（审计列断言基准）。
var mmTestStamp = time.Date(2026, 1, 1, 8, 0, 0, 0, time.Local)

// mmTestCtx 构造测试上下文与假时钟。
// @return 操作上下文
// @return 假时钟（初始时间 mmTestStamp）
func mmTestCtx() (context.Context, *clock.Fake) {
	return context.Background(), clock.NewFake(mmTestStamp)
}

// mmCodeOf 读取领域错误码（非 api.Error 返回空串）。
// @param err 领域错误
// @return 错误码
func mmCodeOf(err error) string {
	var apiErr *api.Error
	// 领域错误统一为 *api.Error，取稳定错误码断言
	if errors.As(err, &apiErr) {
		return apiErr.Code
	}
	return ""
}

// mmModelSample 构造一个完整模型样例（分类/模型两用实体）。
// @param id 主键
// @param code 模型代号
// @param tenant 租户
// @return 模型实体
func mmModelSample(id, code, tenant string) *ModelCategory {
	return &ModelCategory{
		BaseColumns:          BaseColumns{ID: id, Creator: "op", CreatorName: "张三", CreateTime: mmTestStamp, Updater: "op", UpdateTime: mmTestStamp, TenantID: tenant},
		ResourceID:           "res-" + id,
		ModelName:            "模型-" + code,
		ModelCode:            code,
		Tips:                 "提示",
		Target:               "_blank",
		Roles:                `["r1"]`,
		MenuIcon:             "icon",
		Remark:               "备注",
		Sort:                 2,
		ParentID:             "",
		Enable:               1,
		ReturnSort:           1,
		IsLeaf:               1,
		ProcessDefinitionKey: code,
		Visible:              1,
		EngineModelID:        "engine-" + id,
		LastDeployTime:       nil,
	}
}

// TestModelMgmtMemModelStore 验证模型内存存储的 CRUD、审计列填充与克隆防外泄。
// @param t 测试上下文
func TestModelMgmtMemModelStore(t *testing.T) {
	ctx, clk := mmTestCtx()
	store := NewMemModelMgmtStore(clk)

	// 插入：审计时间由存储时钟填充
	m := mmModelSample("m1", "leave", "sys")
	m.CreateTime = time.Time{}
	m.UpdateTime = time.Time{}
	if err := store.Insert(ctx, m); err != nil {
		t.Fatalf("插入失败: %v", err)
	}
	got, found, err := store.GetByPK(ctx, "m1")
	if err != nil || !found {
		t.Fatalf("按主键查询失败: found=%v err=%v", found, err)
	}
	if !got.CreateTime.Equal(mmTestStamp) || !got.UpdateTime.Equal(mmTestStamp) {
		t.Fatalf("审计时间未按注入时钟填充: %+v", got.BaseColumns)
	}

	// 克隆防外泄：改写返回副本不影响存储
	got.ModelName = "被篡改"
	again, _, _ := store.GetByPK(ctx, "m1")
	if again.ModelName != "模型-leave" {
		t.Fatalf("返回副本被外泄修改: %s", again.ModelName)
	}

	// 按代号查询：租户隔离
	if _, found, _ := store.GetByCode(ctx, "sys", "leave"); !found {
		t.Fatal("同租户按代号应命中")
	}
	if _, found, _ := store.GetByCode(ctx, "other", "leave"); found {
		t.Fatal("跨租户按代号不应命中")
	}

	// 租户列表与排序：m2 排序号更小应排前
	m2 := mmModelSample("m2", "expense", "sys")
	m2.Sort = 1
	if err := store.Insert(ctx, m2); err != nil {
		t.Fatalf("插入失败: %v", err)
	}
	if err := store.Insert(ctx, mmModelSample("m3", "leave", "other")); err != nil {
		t.Fatalf("插入失败: %v", err)
	}
	rows, err := store.ListByTenant(ctx, "sys")
	if err != nil {
		t.Fatalf("租户列表失败: %v", err)
	}
	if len(rows) != 2 || rows[0].ID != "m2" || rows[1].ID != "m1" {
		t.Fatalf("租户列表/排序错误: %+v", rows)
	}

	// 更新：更新时间刷新；不存在报错
	m1Update := mmModelSample("m1", "leave", "sys")
	m1Update.ModelName = "改名"
	clk.Advance(time.Hour)
	if err := store.Update(ctx, m1Update); err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	updated, _, _ := store.GetByPK(ctx, "m1")
	if updated.ModelName != "改名" || updated.CreateTime.Equal(updated.UpdateTime) {
		t.Fatalf("更新语义错误: %+v", updated.BaseColumns)
	}
	missing := mmModelSample("nope", "x", "sys")
	if err := store.Update(ctx, missing); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("更新不存在应报未找到: %v", err)
	}

	// 回写部署时间
	deployAt := mmTestStamp.Add(2 * time.Hour)
	if err := store.UpdateLastDeployTime(ctx, "m1", deployAt); err != nil {
		t.Fatalf("回写部署时间失败: %v", err)
	}
	deployed, _, _ := store.GetByPK(ctx, "m1")
	if deployed.LastDeployTime == nil || !deployed.LastDeployTime.Equal(deployAt) {
		t.Fatalf("部署时间回写错误: %v", deployed.LastDeployTime)
	}
	if err := store.UpdateLastDeployTime(ctx, "nope", deployAt); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("回写不存在应报未找到: %v", err)
	}

	// 删除：命中移除，缺失忽略
	if err := store.DeleteByIDs(ctx, []string{"m2", "nope"}); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if _, found, _ := store.GetByPK(ctx, "m2"); found {
		t.Fatal("m2 应已删除")
	}

	// 空实体/空主键防御
	if err := store.Insert(ctx, nil); err == nil {
		t.Fatal("空实体插入应报错")
	}
	if err := store.Insert(ctx, &ModelCategory{BaseColumns: BaseColumns{TenantID: "sys"}}); err == nil {
		t.Fatal("空主键插入应报错")
	}
}

// TestModelMgmtModelService 验证模型服务的增删改查与业务校验。
// @param t 测试上下文
func TestModelMgmtModelService(t *testing.T) {
	ctx, clk := mmTestCtx()
	svc := NewModelService(NewMemModelMgmtStore(clk), clk)

	// 参数校验
	if _, err := svc.Add(ctx, nil); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("空请求应报参数错误: %v", err)
	}
	noCode := mmModelSample("", "", "sys")
	if _, err := svc.Add(ctx, noCode); err == nil || mmCodeOf(err) != "WORKFLOW-3001" {
		t.Fatalf("缺代号应报 WORKFLOW-3001: %v", err)
	}
	noTenant := mmModelSample("", "leave", "")
	if _, err := svc.Add(ctx, noTenant); err == nil || mmCodeOf(err) != "WORKFLOW-3001" {
		t.Fatalf("缺租户应报 WORKFLOW-3001: %v", err)
	}

	// 新增：生成主键并填充审计
	added, err := svc.Add(ctx, mmModelSample("", "leave", "sys"))
	if err != nil {
		t.Fatalf("新增失败: %v", err)
	}
	if added.ID == "" || added.CreateTime.IsZero() || added.TenantID != "sys" {
		t.Fatalf("新增未补齐主键/审计: %+v", added.BaseColumns)
	}

	// 代号重复：同租户拒绝，跨租户放行
	if _, err := svc.Add(ctx, mmModelSample("", "leave", "sys")); err == nil || mmCodeOf(err) != "WORKFLOW-3003" {
		t.Fatalf("同租户重复代号应报 WORKFLOW-3003: %v", err)
	}
	if _, err := svc.Add(ctx, mmModelSample("", "leave", "other")); err != nil {
		t.Fatalf("跨租户同代号应放行: %v", err)
	}

	// 编辑：保留创建审计，刷新更新时间
	edit := mmModelSample(added.ID, "leave", "sys")
	edit.CreatorName = ""
	clk.Advance(time.Hour)
	edited, err := svc.Edit(ctx, edit)
	if err != nil {
		t.Fatalf("编辑失败: %v", err)
	}
	if edited.Remark != edit.Remark || !edited.CreateTime.Equal(mmTestStamp) {
		t.Fatalf("编辑应保留创建审计: %+v", edited.BaseColumns)
	}
	if !edited.UpdateTime.After(mmTestStamp) {
		t.Fatalf("编辑应刷新更新时间: %+v", edited.BaseColumns)
	}
	if _, err := svc.Edit(ctx, mmModelSample("nope", "x", "sys")); err == nil || mmCodeOf(err) != "WORKFLOW-3002" {
		t.Fatalf("编辑不存在应报 WORKFLOW-3002: %v", err)
	}
	// 编辑改为他行代号应拒绝（同租户内冲突）
	if _, err := svc.Add(ctx, mmModelSample("", "expense", "sys")); err != nil {
		t.Fatalf("新增失败: %v", err)
	}
	if _, err := svc.Edit(ctx, mmModelSample(added.ID, "expense", "sys")); err == nil || mmCodeOf(err) != "WORKFLOW-3003" {
		t.Fatalf("编辑改成重复代号应拒绝: %v", err)
	}

	// 按代号取模型
	got, found, err := svc.GetByCode(ctx, "sys", "leave")
	if err != nil || !found || got.ID != added.ID {
		t.Fatalf("按代号取模型失败: found=%v err=%v", found, err)
	}

	// 列表
	rows, err := svc.List(ctx, "sys")
	if err != nil || len(rows) != 2 {
		t.Fatalf("列表失败: len=%d err=%v", len(rows), err)
	}

	// 批量删除与空参防御
	if err := svc.Delete(ctx); err == nil || mmCodeOf(err) != "WORKFLOW-3001" {
		t.Fatalf("空删除应报 WORKFLOW-3001: %v", err)
	}
	if err := svc.Delete(ctx); err == nil || mmCodeOf(err) != "WORKFLOW-3001" {
		t.Fatalf("空删除应报 WORKFLOW-3001: %v", err)
	}
	if err := svc.Delete(ctx, added.ID, "second-id"); err != nil {
		t.Fatalf("批量删除失败: %v", err)
	}
	if rows, _ := svc.List(ctx, "sys"); len(rows) != 1 {
		t.Fatalf("删除后应剩 1 条: %d", len(rows))
	}
}

// TestModelMgmtModelTree 验证模型树形组装：父子挂接、排序与孤儿根提升。
// @param t 测试上下文
func TestModelMgmtModelTree(t *testing.T) {
	ctx, clk := mmTestCtx()
	svc := NewModelService(NewMemModelMgmtStore(clk), clk)

	root1 := mmModelSample("", "r1", "sys")
	root1.Sort = 2
	root2 := mmModelSample("", "r2", "sys")
	root2.Sort = 1
	child1 := mmModelSample("", "c1", "sys")
	child1.ParentID = "PENDING"
	child1.Sort = 2
	child2 := mmModelSample("", "c2", "sys")
	child2.Sort = 1
	orphan := mmModelSample("", "orphan", "sys")
	orphan.ParentID = "ghost-parent"
	orphan.Sort = 3

	r1, _ := svc.Add(ctx, root1)
	if _, err := svc.Add(ctx, root2); err != nil {
		t.Fatalf("新增失败: %v", err)
	}
	c2, _ := svc.Add(ctx, child2)
	c1, _ := svc.Add(ctx, child1)
	if _, err := svc.Add(ctx, orphan); err != nil {
		t.Fatalf("新增失败: %v", err)
	}
	// 补挂父子关系（Add 时不感知父 ID 生成顺序，测试内显式编辑）
	link := mmModelSample(c1.ID, "c1", "sys")
	link.ParentID = r1.ID
	link.Sort = 2
	if _, err := svc.Edit(ctx, link); err != nil {
		t.Fatalf("挂接父子失败: %v", err)
	}
	link2 := mmModelSample(c2.ID, "c2", "sys")
	link2.ParentID = r1.ID
	link2.Sort = 1
	if _, err := svc.Edit(ctx, link2); err != nil {
		t.Fatalf("挂接父子失败: %v", err)
	}

	roots, err := svc.Tree(ctx, "sys")
	if err != nil {
		t.Fatalf("树查询失败: %v", err)
	}
	// 根序：r2(sort1)、r1(sort2)、orphan(sort3，父缺失提升为根)
	if len(roots) != 3 {
		t.Fatalf("根数量错误: %d", len(roots))
	}
	if roots[0].Model.ModelCode != "r2" || roots[1].Model.ModelCode != "r1" || roots[2].Model.ModelCode != "orphan" {
		t.Fatalf("根排序错误: %s,%s,%s", roots[0].Model.ModelCode, roots[1].Model.ModelCode, roots[2].Model.ModelCode)
	}
	// 子序：c2(sort1)、c1(sort2)
	if len(roots[1].Children) != 2 || roots[1].Children[0].Model.ModelCode != "c2" || roots[1].Children[1].Model.ModelCode != "c1" {
		t.Fatalf("子节点排序错误: %+v", roots[1].Children)
	}
}

// TestModelMgmtStepDesignService 验证设计时步骤服务的增删改查与模型内代号唯一性。
// @param t 测试上下文
func TestModelMgmtStepDesignService(t *testing.T) {
	ctx, clk := mmTestCtx()
	svc := NewStepDesignService(NewMemStepDesignStore(clk), clk)

	// 参数校验
	if _, err := svc.Add(ctx, nil); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("空请求应报参数错误: %v", err)
	}
	blank := &StepDesign{BaseColumns: BaseColumns{TenantID: "sys"}}
	if _, err := svc.Add(ctx, blank); err == nil || mmCodeOf(err) != "WORKFLOW-3004" {
		t.Fatalf("缺模型/代号应报 WORKFLOW-3004: %v", err)
	}

	// 新增与模型内代号唯一
	s1 := &StepDesign{BaseColumns: BaseColumns{TenantID: "sys"}, Name: "部门审批", StepCode: "deptAudit", ModelID: "m1", Sort: 2, AllowReturn: "1", BackStepCodes: "start", ReturnPercent: "50", CountersignPercentReturn: "1"}
	added, err := svc.Add(ctx, s1)
	if err != nil {
		t.Fatalf("新增失败: %v", err)
	}
	if added.ID == "" {
		t.Fatal("新增应生成主键")
	}
	dup := &StepDesign{BaseColumns: BaseColumns{TenantID: "sys"}, Name: "重复", StepCode: "deptAudit", ModelID: "m1"}
	if _, err := svc.Add(ctx, dup); err == nil || mmCodeOf(err) != "WORKFLOW-3005" {
		t.Fatalf("模型内重复代号应报 WORKFLOW-3005: %v", err)
	}
	// 同代号不同模型放行
	if _, err := svc.Add(ctx, &StepDesign{BaseColumns: BaseColumns{TenantID: "sys"}, Name: "他模型", StepCode: "deptAudit", ModelID: "m2"}); err != nil {
		t.Fatalf("跨模型同代号应放行: %v", err)
	}

	// 列表按模型且按排序号升序
	s2 := &StepDesign{BaseColumns: BaseColumns{TenantID: "sys"}, Name: "发起", StepCode: "start", ModelID: "m1", Sort: 1}
	if _, err := svc.Add(ctx, s2); err != nil {
		t.Fatalf("新增失败: %v", err)
	}
	rows, err := svc.ListByModel(ctx, "m1")
	if err != nil || len(rows) != 2 {
		t.Fatalf("按模型列表失败: len=%d err=%v", len(rows), err)
	}
	if rows[0].StepCode != "start" || rows[1].StepCode != "deptAudit" {
		t.Fatalf("按模型列表排序错误: %s,%s", rows[0].StepCode, rows[1].StepCode)
	}
	if _, err := svc.ListByModel(ctx, ""); err == nil {
		t.Fatal("空模型 ID 列表应报错")
	}

	// 编辑：不存在与代号改重
	clk.Advance(time.Hour)
	edit := &StepDesign{BaseColumns: BaseColumns{ID: added.ID, TenantID: "sys"}, Name: "部门审批改", StepCode: "deptAudit2", ModelID: "m1", Sort: 2, AllowReturn: "0"}
	edited, err := svc.Edit(ctx, edit)
	if err != nil {
		t.Fatalf("编辑失败: %v", err)
	}
	if edited.StepCode != "deptAudit2" || edited.AllowReturn != "0" {
		t.Fatalf("编辑结果错误: %+v", edited)
	}
	if _, err := svc.Edit(ctx, &StepDesign{BaseColumns: BaseColumns{ID: "nope"}}); err == nil || mmCodeOf(err) != "WORKFLOW-3006" {
		t.Fatalf("编辑不存在应报 WORKFLOW-3006: %v", err)
	}
	// 改成与 start 同号应拒绝（同模型内）
	conflict := &StepDesign{BaseColumns: BaseColumns{ID: added.ID}, Name: "冲突", StepCode: "start", ModelID: "m1"}
	if _, err := svc.Edit(ctx, conflict); err == nil || mmCodeOf(err) != "WORKFLOW-3005" {
		t.Fatalf("编辑改重应报 WORKFLOW-3005: %v", err)
	}

	// 删除
	if err := svc.Delete(ctx, added.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if rows, _ := svc.ListByModel(ctx, "m1"); len(rows) != 1 {
		t.Fatalf("删除后应剩 1 条: %d", len(rows))
	}
}

// TestModelMgmtTemplateService 验证设计器模板服务的增删改查。
// @param t 测试上下文
func TestModelMgmtTemplateService(t *testing.T) {
	ctx, clk := mmTestCtx()
	svc := NewTemplateService(NewMemTemplateStore(clk), clk)

	// 参数校验
	if _, err := svc.Add(ctx, nil); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("空请求应报参数错误: %v", err)
	}
	if _, err := svc.Add(ctx, &TemplateDef{BaseColumns: BaseColumns{TenantID: "sys"}}); err == nil || mmCodeOf(err) != "WORKFLOW-3007" {
		t.Fatalf("缺名称应报 WORKFLOW-3007: %v", err)
	}
	if _, err := svc.Add(ctx, &TemplateDef{Name: "t"}); err == nil || mmCodeOf(err) != "WORKFLOW-3007" {
		t.Fatalf("缺租户应报 WORKFLOW-3007: %v", err)
	}

	// 新增/列表（含分类过滤）
	tpl1 := &TemplateDef{BaseColumns: BaseColumns{TenantID: "sys"}, Name: "经典模板", Layout: "{}", FlowXML: "<xml/>", IsVisible: 1, IsDefault: 1, Source: 1, CategoryID: "cat1"}
	added, err := svc.Add(ctx, tpl1)
	if err != nil {
		t.Fatalf("新增失败: %v", err)
	}
	if added.ID == "" {
		t.Fatal("新增应生成主键")
	}
	if _, err := svc.Add(ctx, &TemplateDef{BaseColumns: BaseColumns{TenantID: "sys"}, Name: "备用模板", CategoryID: "cat1"}); err != nil {
		t.Fatalf("新增失败: %v", err)
	}
	if _, err := svc.Add(ctx, &TemplateDef{BaseColumns: BaseColumns{TenantID: "other"}, Name: "他租户模板", CategoryID: "cat1"}); err != nil {
		t.Fatalf("新增失败: %v", err)
	}
	rows, err := svc.List(ctx, "sys")
	if err != nil || len(rows) != 2 {
		t.Fatalf("按租户列表失败: len=%d err=%v", len(rows), err)
	}
	if rows[0].Name != "备用模板" || rows[1].Name != "经典模板" {
		t.Fatalf("按名称排序错误: %s,%s", rows[0].Name, rows[1].Name)
	}
	catRows, err := svc.ListByCategory(ctx, "cat1")
	if err != nil || len(catRows) != 3 {
		t.Fatalf("按分类列表失败: len=%d err=%v", len(catRows), err)
	}

	// 编辑与删除
	clk.Advance(time.Hour)
	edit := &TemplateDef{BaseColumns: BaseColumns{ID: added.ID, TenantID: "sys"}, Name: "经典模板改", IsDefault: 0}
	edited, err := svc.Edit(ctx, edit)
	if err != nil {
		t.Fatalf("编辑失败: %v", err)
	}
	if edited.Name != "经典模板改" || edited.IsDefault != 0 {
		t.Fatalf("编辑结果错误: %+v", edited)
	}
	if _, err := svc.Edit(ctx, &TemplateDef{BaseColumns: BaseColumns{ID: "nope"}}); err == nil || mmCodeOf(err) != "WORKFLOW-3008" {
		t.Fatalf("编辑不存在应报 WORKFLOW-3008: %v", err)
	}
	if err := svc.Delete(ctx, added.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if rows, _ := svc.List(ctx, "sys"); len(rows) != 1 {
		t.Fatalf("删除后应剩 1 条: %d", len(rows))
	}
}

// TestDeploySnapshotMemHisStepStore 验证快照内存存储：批量替换幂等、部署隔离、排序与克隆。
// @param t 测试上下文
func TestDeploySnapshotMemHisStepStore(t *testing.T) {
	ctx, _ := mmTestCtx()
	store := NewMemHisStepStore()

	// 首次批量写入
	r1 := &StepRule{DeploymentID: "dep-1", StepCode: "b", StepName: "B", Sort: 2, AllowReturn: "1"}
	r2 := &StepRule{DeploymentID: "dep-1", StepCode: "a", StepName: "A", Sort: 1, AllowReturn: "0"}
	if err := store.Put(ctx, "dep-1", []*StepRule{r1, r2}); err != nil {
		t.Fatalf("快照写入失败: %v", err)
	}
	rOther := &StepRule{DeploymentID: "dep-2", StepCode: "x", StepName: "X", Sort: 1}
	if err := store.Put(ctx, "dep-2", []*StepRule{rOther}); err != nil {
		t.Fatalf("快照写入失败: %v", err)
	}

	// 列表按部署隔离并按 sort 升序
	rows, err := store.ListByDeployment(ctx, "dep-1")
	if err != nil || len(rows) != 2 {
		t.Fatalf("按部署列表失败: len=%d err=%v", len(rows), err)
	}
	if rows[0].StepCode != "a" || rows[1].StepCode != "b" {
		t.Fatalf("快照排序错误: %s,%s", rows[0].StepCode, rows[1].StepCode)
	}

	// 按部署+节点定位
	got, found, err := store.GetByDeploymentAndActivity(ctx, "dep-1", "a")
	if err != nil || !found || got.StepName != "A" {
		t.Fatalf("按节点定位失败: found=%v err=%v", found, err)
	}
	if _, found, _ := store.GetByDeploymentAndActivity(ctx, "dep-1", "zzz"); found {
		t.Fatal("未写入节点不应命中")
	}

	// 克隆防外泄
	got.StepName = "被篡改"
	again, _, _ := store.GetByDeploymentAndActivity(ctx, "dep-1", "a")
	if again.StepName != "A" {
		t.Fatalf("返回副本被外泄修改: %s", again.StepName)
	}

	// 幂等：同部署重复 Put 替换而非追加
	if err := store.Put(ctx, "dep-1", []*StepRule{{DeploymentID: "dep-1", StepCode: "only", StepName: "O", Sort: 1}}); err != nil {
		t.Fatalf("重复写入失败: %v", err)
	}
	rows, _ = store.ListByDeployment(ctx, "dep-1")
	if len(rows) != 1 || rows[0].StepCode != "only" {
		t.Fatalf("重复部署应替换而非追加: %d", len(rows))
	}
	if rows2, _ := store.ListByDeployment(ctx, "dep-2"); len(rows2) != 1 {
		t.Fatal("其他部署不应受影响")
	}

	// 空批量替换清空部署快照
	if err := store.Put(ctx, "dep-2", nil); err != nil {
		t.Fatalf("空批量替换失败: %v", err)
	}
	if rows, _ := store.ListByDeployment(ctx, "dep-2"); len(rows) != 0 {
		t.Fatalf("空批量替换应清空: %d", len(rows))
	}

	// 删除与空参防御
	if err := store.Put(ctx, "", []*StepRule{r1}); err == nil {
		t.Fatal("空部署写入应报错")
	}
	if err := store.DeleteByDeployment(ctx, "dep-1"); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if rows, _ := store.ListByDeployment(ctx, "dep-1"); len(rows) != 0 {
		t.Fatal("删除后应为空")
	}
}

// TestDeploySnapshotService 验证部署快照服务：字段映射、部署时间回写与幂等替换。
// @param t 测试上下文
func TestDeploySnapshotService(t *testing.T) {
	ctx, clk := mmTestCtx()
	models := NewMemModelMgmtStore(clk)
	designs := NewMemStepDesignStore(clk)
	his := NewMemHisStepStore()
	svc := NewDeploySnapshotService(models, designs, his, clk)

	// 播种模型与两个设计步骤（含另一模型的干扰步骤）
	model := mmModelSample("m1", "leave", "sys")
	model.LastDeployTime = nil
	if err := models.Insert(ctx, model); err != nil {
		t.Fatalf("播种模型失败: %v", err)
	}
	d1 := &StepDesign{BaseColumns: BaseColumns{ID: "d1", TenantID: "sys"}, Name: "部门审批", StepCode: "deptAudit", ModelID: "m1", Sort: 2, AllowReturn: "1", BackStepCodes: "start,boss", ReturnApprovalStep: "3", CountersignPercentReturn: "1", ReturnPercent: "50", URL: "/audit"}
	d2 := &StepDesign{BaseColumns: BaseColumns{ID: "d2", TenantID: "sys"}, Name: "发起", StepCode: "start", ModelID: "m1", Sort: 1, AllowReturn: "0", URL: "/start"}
	if err := designs.Insert(ctx, d1); err != nil {
		t.Fatalf("播种步骤失败: %v", err)
	}
	if err := designs.Insert(ctx, d2); err != nil {
		t.Fatalf("播种步骤失败: %v", err)
	}
	if err := designs.Insert(ctx, &StepDesign{BaseColumns: BaseColumns{ID: "d3", TenantID: "sys"}, Name: "他模型", StepCode: "other", ModelID: "m2"}); err != nil {
		t.Fatalf("播种步骤失败: %v", err)
	}

	// 首次快照
	rules, err := svc.SnapshotForDeploy(ctx, "sys", "leave", "dep-1", "def-1")
	if err != nil {
		t.Fatalf("快照失败: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("快照步骤数错误: %d", len(rules))
	}
	// 字段映射断言（按 sort 定位）
	var deptRule *StepRule
	for _, r := range rules {
		if r.StepCode == "deptAudit" {
			deptRule = r
		}
	}
	if deptRule == nil {
		t.Fatal("deptAudit 未生成快照")
	}
	if deptRule.DeploymentID != "dep-1" || deptRule.StepName != "部门审批" ||
		deptRule.AllowReturn != "1" || deptRule.BackStepCodes != "start,boss" ||
		deptRule.ReturnApprovalStep != "3" || deptRule.CountersignPercentReturn != "1" ||
		deptRule.ReturnPercent != "50" || deptRule.Sort != 2 || deptRule.URL != "/audit" {
		t.Fatalf("快照字段映射错误: %+v", deptRule)
	}
	// 存储内可按部署+节点读到
	if _, found, _ := his.GetByDeploymentAndActivity(ctx, "dep-1", "start"); !found {
		t.Fatal("start 快照未落库")
	}
	// 部署时间已回写
	saved, _, _ := models.GetByPK(ctx, "m1")
	if saved.LastDeployTime == nil || !saved.LastDeployTime.Equal(mmTestStamp) {
		t.Fatalf("部署时间未回写: %v", saved.LastDeployTime)
	}

	// 幂等：同部署重复快照替换而非追加
	clk.Advance(time.Hour)
	if _, err := svc.SnapshotForDeploy(ctx, "sys", "leave", "dep-1", "def-2"); err != nil {
		t.Fatalf("重复快照失败: %v", err)
	}
	rows, _ := his.ListByDeployment(ctx, "dep-1")
	if len(rows) != 2 {
		t.Fatalf("重复快照应替换而非追加: %d", len(rows))
	}
	saved, _, _ = models.GetByPK(ctx, "m1")
	if !saved.LastDeployTime.Equal(mmTestStamp.Add(time.Hour)) {
		t.Fatalf("重复快照应刷新部署时间: %v", saved.LastDeployTime)
	}

	// 参数与存在性校验
	if _, err := svc.SnapshotForDeploy(ctx, "", "leave", "dep-1", "def-1"); err == nil || mmCodeOf(err) != "WORKFLOW-3009" {
		t.Fatalf("缺租户应报 WORKFLOW-3009: %v", err)
	}
	if _, err := svc.SnapshotForDeploy(ctx, "sys", "leave", "", "def-1"); err == nil || mmCodeOf(err) != "WORKFLOW-3009" {
		t.Fatalf("缺部署应报 WORKFLOW-3009: %v", err)
	}
	if _, err := svc.SnapshotForDeploy(ctx, "sys", "ghost", "dep-9", "def-9"); err == nil || mmCodeOf(err) != "WORKFLOW-3002" {
		t.Fatalf("模型不存在应报 WORKFLOW-3002: %v", err)
	}

	// 服务缺依赖防御
	nilSvc := NewDeploySnapshotService(nil, designs, his, clk)
	if _, err := nilSvc.SnapshotForDeploy(ctx, "sys", "leave", "dep-1", "def-1"); err == nil {
		t.Fatal("模型存储缺失应报错")
	}
	emptySvc := NewDeploySnapshotService(models, nil, his, clk)
	if _, err := emptySvc.SnapshotForDeploy(ctx, "sys", "leave", "dep-1", "def-1"); err == nil {
		t.Fatal("步骤存储缺失应报错")
	}
	noHisSvc := NewDeploySnapshotService(models, designs, nil, clk)
	if _, err := noHisSvc.SnapshotForDeploy(ctx, "sys", "leave", "dep-1", "def-1"); err == nil {
		t.Fatal("快照存储缺失应报错")
	}
}

// ─── 覆盖补强：缺省回退 / 错误注入 / 排序键边界 ─────────────────────────────────────────────

// mmFailingModelStore 是可注入错误的模型存储桩（内嵌默认实现，按需覆盖）。
type mmFailingModelStore struct {
	MemModelMgmtStore      // 内嵌默认实现
	failGetByCode     bool // GetByCode 注入错误
	failGetByPK       bool // GetByPK 注入错误
	failPutDeploy     bool // UpdateLastDeployTime 注入错误
}

// GetByCode 按需注入查询错误。
// @param ctx 操作上下文
// @param tenantID 租户
// @param modelCode 模型代号
// @return 模型实体
// @return 是否存在
// @return 注入的错误
func (f *mmFailingModelStore) GetByCode(ctx context.Context, tenantID, modelCode string) (*ModelCategory, bool, error) {
	// 注入开关命中时返回超时错误
	if f.failGetByCode {
		return nil, false, context.DeadlineExceeded
	}
	return f.MemModelMgmtStore.GetByCode(ctx, tenantID, modelCode)
}

// GetByPK 按需注入查询错误。
// @param ctx 操作上下文
// @param id 主键
// @return 模型实体
// @return 是否存在
// @return 注入的错误
func (f *mmFailingModelStore) GetByPK(ctx context.Context, id string) (*ModelCategory, bool, error) {
	// 注入开关命中时返回超时错误
	if f.failGetByPK {
		return nil, false, context.DeadlineExceeded
	}
	return f.MemModelMgmtStore.GetByPK(ctx, id)
}

// UpdateLastDeployTime 按需注入回写错误。
// @param ctx 操作上下文
// @param id 主键
// @param deployTime 部署时间
// @return 注入的错误
func (f *mmFailingModelStore) UpdateLastDeployTime(ctx context.Context, id string, deployTime time.Time) error {
	// 注入开关命中时返回超时错误
	if f.failPutDeploy {
		return context.DeadlineExceeded
	}
	return f.MemModelMgmtStore.UpdateLastDeployTime(ctx, id, deployTime)
}

// mmFailingStepStore 是可注入错误的步骤存储桩。
type mmFailingStepStore struct {
	MemStepDesignStore      // 内嵌默认实现
	failListByModel    bool // ListByModel 注入错误
	failGetByModelCode bool // GetByModelAndCode 注入错误
}

// GetByModelAndCode 按需注入查询错误。
// @param ctx 操作上下文
// @param modelID 模型 ID
// @param stepCode 步骤代号
// @return 步骤实体
// @return 是否存在
// @return 注入的错误
func (f *mmFailingStepStore) GetByModelAndCode(ctx context.Context, modelID, stepCode string) (*StepDesign, bool, error) {
	// 注入开关命中时返回超时错误
	if f.failGetByModelCode {
		return nil, false, context.DeadlineExceeded
	}
	return f.MemStepDesignStore.GetByModelAndCode(ctx, modelID, stepCode)
}

// ListByModel 按需注入查询错误。
// @param ctx 操作上下文
// @param modelID 模型 ID
// @return 步骤列表
// @return 注入的错误
func (f *mmFailingStepStore) ListByModel(ctx context.Context, modelID string) ([]*StepDesign, error) {
	// 注入开关命中时返回超时错误
	if f.failListByModel {
		return nil, context.DeadlineExceeded
	}
	return f.MemStepDesignStore.ListByModel(ctx, modelID)
}

// mmFailingHisStore 是可注入错误的快照存储桩。
type mmFailingHisStore struct {
	MemHisStepStore      // 内嵌默认实现
	failPut         bool // Put 注入错误
}

// Put 按需注入写入错误。
// @param ctx 操作上下文
// @param deploymentID 部署 ID
// @param rules 快照规则列表
// @return 注入的错误
func (f *mmFailingHisStore) Put(ctx context.Context, deploymentID string, rules []*StepRule) error {
	// 注入开关命中时返回超时错误
	if f.failPut {
		return context.DeadlineExceeded
	}
	return f.MemHisStepStore.Put(ctx, deploymentID, rules)
}

// TestModelMgmtDefaults 验证空时钟回退与 nil 实体克隆防御。
// @param t 测试上下文
func TestModelMgmtDefaults(t *testing.T) {
	ctx := context.Background()

	// 空时钟回退真实时钟（不 panic 即可）
	if NewMemModelMgmtStore(nil) == nil || NewMemStepDesignStore(nil) == nil || NewMemTemplateStore(nil) == nil {
		t.Fatal("空时钟构造应回退真实时钟")
	}
	his := NewMemHisStepStore()
	models := NewMemModelMgmtStore(nil)
	designs := NewMemStepDesignStore(nil)
	// 服务空时钟回退
	NewModelService(models, nil)
	NewStepDesignService(designs, nil)
	NewTemplateService(NewMemTemplateStore(nil), nil)
	NewDeploySnapshotService(models, designs, his, nil)

	// nil 克隆防御
	if mmCloneModel(nil) != nil || mmCloneStepDesign(nil) != nil || mmCloneTemplateDef(nil) != nil || mmCloneRule(nil) != nil {
		t.Fatal("nil 实体克隆应返回 nil")
	}

	// 排序键同值回退分支（同号按代号/同名按主键）
	if !mmStepLess(&StepDesign{Sort: 1, StepCode: "a"}, &StepDesign{Sort: 1, StepCode: "b"}) {
		t.Fatal("同号步骤应按代号排序")
	}
	if mmStepLess(&StepDesign{Sort: 1, StepCode: "b"}, &StepDesign{Sort: 1, StepCode: "a"}) {
		t.Fatal("同号步骤代号逆序应为否")
	}
	if !mmTemplateLess(&TemplateDef{Name: "x", ID: "1"}, &TemplateDef{Name: "x", ID: "2"}) {
		t.Fatal("同名模板应按主键排序")
	}
	if !mmRuleLess(&StepRule{Sort: 1, StepCode: "a"}, &StepRule{Sort: 1, StepCode: "b"}) {
		t.Fatal("同号快照应按代号排序")
	}

	// 模型服务空参分支
	svc := NewModelService(models, clock.NewFake(mmTestStamp))
	if _, err := svc.List(ctx, ""); err == nil {
		t.Fatal("空租户列表应报错")
	}
	if _, _, err := svc.GetByCode(ctx, "", "x"); err == nil {
		t.Fatal("空租户取模型应报错")
	}
	if _, _, err := svc.GetByCode(ctx, "sys", ""); err == nil {
		t.Fatal("空代号取模型应报错")
	}

	// 步骤/模板服务空删除分支
	stepSvc := NewStepDesignService(designs, clock.NewFake(mmTestStamp))
	if err := stepSvc.Delete(ctx); err == nil {
		t.Fatal("步骤空删除应报错")
	}
	tplSvc := NewTemplateService(NewMemTemplateStore(nil), clock.NewFake(mmTestStamp))
	if err := tplSvc.Delete(ctx); err == nil {
		t.Fatal("模板空删除应报错")
	}
	if _, err := tplSvc.List(ctx, ""); err == nil {
		t.Fatal("模板空租户列表应报错")
	}
	if _, err := tplSvc.ListByCategory(ctx, ""); err == nil {
		t.Fatal("模板空分类列表应报错")
	}

	// 服务查询错误透传（注入桩）
	failModel := &mmFailingModelStore{MemModelMgmtStore: *NewMemModelMgmtStore(nil), failGetByCode: true}
	if _, err := NewModelService(failModel, nil).Add(ctx, mmModelSample("", "x", "sys")); err == nil {
		t.Fatal("代号查重错误应透传")
	}
	failPK := &mmFailingModelStore{MemModelMgmtStore: *NewMemModelMgmtStore(nil), failGetByPK: true}
	if _, err := NewModelService(failPK, nil).Edit(ctx, mmModelSample("m1", "x", "sys")); err == nil {
		t.Fatal("主键查询错误应透传")
	}
	failStepCode := &mmFailingStepStore{MemStepDesignStore: *NewMemStepDesignStore(nil), failGetByModelCode: true}
	if _, err := NewStepDesignService(failStepCode, nil).Add(ctx, &StepDesign{BaseColumns: BaseColumns{TenantID: "sys"}, ModelID: "m1", StepCode: "s"}); err == nil {
		t.Fatal("步骤查重错误应透传")
	}
}

// TestModelMgmtTreeEdge 验证树排序键边界（同号兄弟按代号）。
// @param t 测试上下文
func TestModelMgmtTreeEdge(t *testing.T) {
	// 同号兄弟直接构造排序
	b := &ModelTreeNode{Model: &ModelCategory{ID: "b", Sort: 1, ModelCode: "b"}}
	b.Children = []*ModelTreeNode{{Model: &ModelCategory{ID: "b1", Sort: 1, ModelCode: "b1"}}}
	a := &ModelTreeNode{Model: &ModelCategory{ID: "a", Sort: 1, ModelCode: "a"}}
	roots := []*ModelTreeNode{b, a}
	mmSortModelNodes(roots)
	if roots[0].Model.ID != "a" || roots[1].Model.ID != "b" {
		t.Fatalf("同号兄弟排序错误: %s,%s", roots[0].Model.ID, roots[1].Model.ID)
	}
	if len(roots[1].Children) != 1 {
		t.Fatal("子树应保留")
	}
}

// TestDeploySnapshotErrors 验证部署快照服务的错误透传链。
// @param t 测试上下文
func TestDeploySnapshotErrors(t *testing.T) {
	ctx, clk := mmTestCtx()
	his := NewMemHisStepStore()

	// 代号查询错误
	failCode := &mmFailingModelStore{MemModelMgmtStore: *NewMemModelMgmtStore(nil), failGetByCode: true}
	if _, err := NewDeploySnapshotService(failCode, NewMemStepDesignStore(nil), his, clk).SnapshotForDeploy(ctx, "sys", "leave", "dep-1", "def-1"); err == nil {
		t.Fatal("模型查询错误应透传")
	}
	// 步骤列表错误
	failList := &mmFailingStepStore{MemStepDesignStore: *NewMemStepDesignStore(nil), failListByModel: true}
	if _, err := NewDeploySnapshotService(NewMemModelMgmtStore(nil), failList, his, clk).SnapshotForDeploy(ctx, "sys", "leave", "dep-1", "def-1"); err == nil {
		t.Fatal("步骤列表错误应透传")
	}
	// 快照写入错误
	failPut := &mmFailingHisStore{MemHisStepStore: *NewMemHisStepStore(), failPut: true}
	if _, err := NewDeploySnapshotService(NewMemModelMgmtStore(nil), NewMemStepDesignStore(nil), failPut, clk).SnapshotForDeploy(ctx, "sys", "leave", "dep-1", "def-1"); err == nil {
		t.Fatal("快照写入错误应透传")
	}
	// 部署时间回写错误
	failDeploy := &mmFailingModelStore{MemModelMgmtStore: *NewMemModelMgmtStore(nil), failPutDeploy: true}
	if _, err := NewDeploySnapshotService(failDeploy, NewMemStepDesignStore(nil), his, clk).SnapshotForDeploy(ctx, "sys", "leave", "dep-1", "def-1"); err == nil {
		t.Fatal("部署时间回写错误应透传")
	}
}
