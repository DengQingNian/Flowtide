package engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/tenant"
)

// dmnAlphaXML 是包含单个决策与输入输出列的 DMN 文档。
const dmnAlphaXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20191111/MODEL/" id="alphaDefinitions" name="Alpha" namespace="http://example.com/dmn">
  <decision id="decisionA" name="Decision A">
    <decisionTable id="tableA" hitPolicy="UNIQUE">
      <input id="inputA" label="金额">
        <inputExpression id="exprA" typeRef="number"><text> amount </text></inputExpression>
      </input>
      <output id="outputA" name="结果" typeRef="string"/>
      <rule id="ruleA1">
        <inputEntry><text>&gt; 100</text></inputEntry>
        <outputEntry><text>"high"</text></outputEntry>
      </rule>
    </decisionTable>
  </decision>
</definitions>`

// dmnZetaXML 是包含两个输入、两个输出与两条规则的 DMN 文档。
const dmnZetaXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20191111/MODEL/" id="zetaDefinitions" name="Zeta" namespace="http://example.com/dmn">
  <decision id="decisionB" name="Decision B">
    <decisionTable id="tableB" hitPolicy="FIRST">
      <input id="inputB1" label="类型">
        <inputExpression id="exprB1" typeRef="string"><text>type</text></inputExpression>
      </input>
      <input id="inputB2">
        <inputExpression id="exprB2" text="level * 2"/>
      </input>
      <output id="outputB1" name="等级" type="string"/>
      <output id="outputB2" name="备注" typeRef="string"/>
      <rule id="ruleB1"/>
      <rule id="ruleB2"/>
    </decisionTable>
  </decision>
</definitions>`

// dmnNestedXML 是包含嵌套决策、无决策表决策与缺失表 ID 的 DMN 文档。
const dmnNestedXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20191111/MODEL/">
  <knowledgeModel id="model">
    <decision id="nested" name="Nested">
      <decisionTable hitPolicy="COLLECT">
        <input id="nestedInput"><inputExpression text="age"/></input>
        <output id="nestedOutput" name="Allowed" type="boolean"/>
        <rule id="nestedRule"/>
      </decisionTable>
    </decision>
  </knowledgeModel>
  <decision id="ignored" name="Ignored"/>
  <decision>
    <decisionTable id="tableOnly" hitPolicy="ANY"/>
  </decision>
</definitions>`

// deployDMNBundle 部署 BPMN 与 DMN 压缩包，并返回流程定义。
// @param t 测试上下文
// @param engine 引擎
// @param resources 额外资源
// @return 流程定义
func deployDMNBundle(t *testing.T, engine *Engine, resources map[string][]byte) api.ProcessDefinition {
	t.Helper()
	entries := map[string][]byte{
		"processes/simple.bpmn20.xml": []byte(simpleProcess),
	}
	for name, content := range resources {
		entries[name] = content
	}
	if _, err := engine.Deploy(context.Background(), api.DeploymentRequest{
		Name:      "dmn-bundle",
		Resources: map[string][]byte{"bundle.zip": makeZip(t, entries)},
	}); err != nil {
		t.Fatalf("部署 DMN 资源失败: %v", err)
	}
	definition, err := engine.GetProcessDefinitionByKey(context.Background(), "simple")
	if err != nil {
		t.Fatalf("查询流程定义失败: %v", err)
	}
	return definition
}

// TestDMNDefinitions_List 验证从 ZIP 部署读取决策与决策表元数据。
// @param t 测试上下文
func TestDMNDefinitions_List(t *testing.T) {
	engine := newTestEngine(t)
	definition := deployDMNBundle(t, engine, map[string][]byte{
		"decisions/zeta.DMN":   []byte(dmnZetaXML),
		"decisions/alpha.dmn":  []byte(dmnAlphaXML),
		"decisions/readme.txt": []byte("not a dmn"),
	})
	ctx := context.Background()
	decisions, err := engine.ListDecisionDefinitions(ctx, definition.ID)
	if err != nil {
		t.Fatalf("查询决策定义失败: %v", err)
	}
	if len(decisions) != 2 {
		t.Fatalf("决策定义数量错误: %+v", decisions)
	}
	// 按资源名排序，扩展名大小写不影响读取
	first := decisions[0]
	if first.ID != "decisionA" || first.Key != "decisionA" || first.Name != "Decision A" {
		t.Fatalf("第一个决策定义错误: %+v", first)
	}
	if first.DeploymentID != definition.DeploymentID || first.ResourceName != "decisions/alpha.dmn" {
		t.Fatalf("第一个决策定义归属错误: %+v", first)
	}
	if first.TenantID != definition.TenantID || first.Version != definition.Version {
		t.Fatalf("第一个决策定义租户或版本错误: %+v", first)
	}
	if second := decisions[1]; second.ID != "decisionB" || second.ResourceName != "decisions/zeta.DMN" {
		t.Fatalf("第二个决策定义错误: %+v", second)
	}

	tables, err := engine.ListDecisionTables(ctx, definition.ID)
	if err != nil {
		t.Fatalf("查询决策表失败: %v", err)
	}
	if len(tables) != 2 {
		t.Fatalf("决策表数量错误: %+v", tables)
	}
	tableA := tables[0]
	if tableA.ID != "tableA" || tableA.Key != "decisionA" || tableA.Name != "Decision A" {
		t.Fatalf("第一个决策表基础字段错误: %+v", tableA)
	}
	if tableA.DeploymentID != definition.DeploymentID || tableA.ResourceName != "decisions/alpha.dmn" || tableA.TenantID != definition.TenantID {
		t.Fatalf("第一个决策表归属错误: %+v", tableA)
	}
	if tableA.HitPolicy != "UNIQUE" || tableA.RuleCount != 1 {
		t.Fatalf("第一个决策表策略或规则数错误: %+v", tableA)
	}
	if tableA.Inputs == nil || len(tableA.Inputs) != 1 {
		t.Fatalf("第一个决策表输入数量错误: %+v", tableA.Inputs)
	}
	if input := tableA.Inputs[0]; input.ID != "inputA" || input.Label != "金额" || input.Expression != "amount" {
		t.Fatalf("第一个决策表输入错误: %+v", tableA.Inputs[0])
	}
	if tableA.Outputs == nil || len(tableA.Outputs) != 1 {
		t.Fatalf("第一个决策表输出数量错误: %+v", tableA.Outputs)
	}
	if output := tableA.Outputs[0]; output.ID != "outputA" || output.Name != "结果" || output.Type != "string" {
		t.Fatalf("第一个决策表输出错误: %+v", tableA.Outputs[0])
	}

	tableB := tables[1]
	if tableB.ID != "tableB" || tableB.HitPolicy != "FIRST" || tableB.RuleCount != 2 {
		t.Fatalf("第二个决策表基础字段错误: %+v", tableB)
	}
	if len(tableB.Inputs) != 2 {
		t.Fatalf("第二个决策表输入数量错误: %+v", tableB.Inputs)
	}
	// 表达式放在 text 属性时同样可读取
	if tableB.Inputs[1].ID != "inputB2" || tableB.Inputs[1].Label != "" || tableB.Inputs[1].Expression != "level * 2" {
		t.Fatalf("第二个决策表属性表达式错误: %+v", tableB.Inputs[1])
	}
	if len(tableB.Outputs) != 2 {
		t.Fatalf("第二个决策表输出数量错误: %+v", tableB.Outputs)
	}
	// type 缺失时回退 typeRef
	if tableB.Outputs[1].ID != "outputB2" || tableB.Outputs[1].Type != "string" {
		t.Fatalf("第二个决策表 typeRef 回退错误: %+v", tableB.Outputs[1])
	}
}

// TestDMNDefinitions_NestedAndIgnored 验证嵌套决策、无决策表决策与表 ID 回退。
// @param t 测试上下文
func TestDMNDefinitions_NestedAndIgnored(t *testing.T) {
	engine := newTestEngine(t)
	definition := deployDMNBundle(t, engine, map[string][]byte{"nested.dmn": []byte(dmnNestedXML)})
	ctx := context.Background()
	tables, err := engine.ListDecisionTables(ctx, definition.ID)
	if err != nil {
		t.Fatalf("查询决策表失败: %v", err)
	}
	if len(tables) != 2 {
		t.Fatalf("决策表数量错误: %+v", tables)
	}
	// 嵌套 decision 被识别，缺失 decisionTable id 时回退 decision id
	if table := tables[0]; table.ID != "nested" || table.Key != "nested" || table.HitPolicy != "COLLECT" || table.RuleCount != 1 {
		t.Fatalf("嵌套决策表错误: %+v", table)
	}
	if nestedInput := tables[0].Inputs[0]; nestedInput.ID != "nestedInput" || nestedInput.Expression != "age" {
		t.Fatalf("嵌套输入表达式错误: %+v", tables[0].Inputs[0])
	}
	if nestedOutput := tables[0].Outputs[0]; nestedOutput.ID != "nestedOutput" || nestedOutput.Type != "boolean" {
		t.Fatalf("嵌套输出类型错误: %+v", tables[0].Outputs[0])
	}
	// decision 缺少 id 时使用 decisionTable id 作为定义 ID 与 Key
	if table := tables[1]; table.ID != "tableOnly" || table.Key != "tableOnly" || table.Name != "" {
		t.Fatalf("缺失 decision id 回退错误: %+v", table)
	}
	if tables[1].Inputs == nil || tables[1].Outputs == nil {
		t.Fatalf("空输入输出应为非 nil 切片: %+v", tables[1])
	}
	decisions, err := engine.ListDecisionDefinitions(ctx, definition.ID)
	if err != nil || len(decisions) != 2 {
		t.Fatalf("决策定义数量错误: %v %+v", err, decisions)
	}
	// 无决策表的 decision 被忽略
	for _, decision := range decisions {
		if decision.ID == "ignored" {
			t.Fatalf("无决策表的 decision 不应出现: %+v", decisions)
		}
	}
}

// TestDMNDefinitions_EmptyAndMissing 验证无 DMN、定义不存在与部署缺失。
// @param t 测试上下文
func TestDMNDefinitions_EmptyAndMissing(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	definition, err := engine.GetProcessDefinitionByKey(context.Background(), "simple")
	if err != nil {
		t.Fatalf("查询流程定义失败: %v", err)
	}
	ctx := context.Background()
	decisions, err := engine.ListDecisionDefinitions(ctx, definition.ID)
	if err != nil || decisions == nil || len(decisions) != 0 {
		t.Fatalf("无 DMN 部署应返回空决策列表: %v %+v", err, decisions)
	}
	tables, err := engine.ListDecisionTables(ctx, definition.ID)
	if err != nil || tables == nil || len(tables) != 0 {
		t.Fatalf("无 DMN 部署应返回空决策表列表: %v %+v", err, tables)
	}
	if _, err := engine.ListDecisionDefinitions(ctx, "missing"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失流程定义应返回 404: %v", err)
	}
	if _, err := engine.ListDecisionTables(ctx, "missing"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失流程定义应返回 404: %v", err)
	}
	// 定义指向不存在的部署时返回空列表而非 panic
	engine.state.Definitions["orphan"] = &definitionState{Info: api.ProcessDefinition{ID: "orphan", DeploymentID: "missing-deployment"}}
	if decisions, err := engine.ListDecisionDefinitions(ctx, "orphan"); err != nil || decisions == nil || len(decisions) != 0 {
		t.Fatalf("缺失部署应返回空列表: %v %+v", err, decisions)
	}
	if tables, err := engine.ListDecisionTables(ctx, "orphan"); err != nil || tables == nil || len(tables) != 0 {
		t.Fatalf("缺失部署应返回空列表: %v %+v", err, tables)
	}
	var service api.DMNService = engine
	if service == nil || engine.DMNService() == nil {
		t.Fatalf("DMNService 应返回引擎自身")
	}
}

// TestDMNDefinitions_Tenant 验证决策元数据沿用流程定义租户。
// @param t 测试上下文
func TestDMNDefinitions_Tenant(t *testing.T) {
	engine, err := New(Config{HistoryLevel: HistoryAudit, TenantID: "tenant-a"})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	t.Cleanup(func() { _ = engine.Close(context.Background()) })
	definition := deployDMNBundle(t, engine, map[string][]byte{"decisions/alpha.dmn": []byte(dmnAlphaXML)})
	if definition.TenantID != "tenant-a" {
		t.Fatalf("流程定义租户错误: %+v", definition)
	}
	if _, err := engine.ListDecisionDefinitions(tenant.With(context.Background(), "tenant-b"), definition.ID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("跨租户查询决策应返回 404: %v", err)
	}
	if _, err := engine.ListDecisionTables(tenant.With(context.Background(), "tenant-b"), definition.ID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("跨租户查询决策表应返回 404: %v", err)
	}
	tables, err := engine.ListDecisionTables(tenant.With(context.Background(), "tenant-a"), definition.ID)
	if err != nil || len(tables) != 1 || tables[0].TenantID != "tenant-a" {
		t.Fatalf("同租户查询失败: %v %+v", err, tables)
	}
}

// TestDMNDefinitions_ParseErrors 验证非法 DMN XML 返回 400 错误。
// @param t 测试上下文
func TestDMNDefinitions_ParseErrors(t *testing.T) {
	cases := []struct {
		name string
		data string
	}{
		{name: "空内容", data: ""},
		{name: "未闭合", data: "<definitions>"},
		{name: "错误根元素", data: "<notdefinitions/>"},
		{name: "DTD", data: "<!DOCTYPE definitions><definitions/>"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			engine := newTestEngine(t)
			definition := deployDMNBundle(t, engine, map[string][]byte{"decisions/bad.dmn": []byte(testCase.data)})
			for _, list := range []func(context.Context, string) error{
				func(ctx context.Context, id string) error {
					_, err := engine.ListDecisionDefinitions(ctx, id)
					return err
				},
				func(ctx context.Context, id string) error {
					_, err := engine.ListDecisionTables(ctx, id)
					return err
				},
			} {
				err := list(context.Background(), definition.ID)
				if !errors.Is(err, api.ErrBadRequest) || !strings.Contains(err.Error(), "ENGINE-1207") {
					t.Fatalf("非法 DMN 应返回 400 ENGINE-1207: %v", err)
				}
			}
		})
	}
}

// TestDMNDefinitions_SortWithinResource 验证同一资源内按决策 ID 排序。
// @param t 测试上下文
func TestDMNDefinitions_SortWithinResource(t *testing.T) {
	engine := newTestEngine(t)
	const content = `<definitions>
  <decision id="second" name="Second"><decisionTable id="tableSecond"/></decision>
  <decision id="first" name="First"><decisionTable id="tableFirst"/></decision>
</definitions>`
	definition := deployDMNBundle(t, engine, map[string][]byte{"multi.dmn": []byte(content)})
	decisions, err := engine.ListDecisionDefinitions(context.Background(), definition.ID)
	if err != nil {
		t.Fatalf("查询决策定义失败: %v", err)
	}
	if len(decisions) != 2 || decisions[0].ID != "first" || decisions[1].ID != "second" {
		t.Fatalf("同一资源内排序错误: %+v", decisions)
	}
}
