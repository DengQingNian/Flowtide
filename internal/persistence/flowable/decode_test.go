package flowable

import (
	"testing"
	"time"
)

// deploymentRow 构造部署行。
// @param id 部署 ID
// @return 行
func deploymentRow(t *testing.T, id string) Row {
	t.Helper()
	row, _ := NewRow("ACT_RE_DEPLOYMENT")
	_ = row.Set("ID_", id)
	_ = row.Set("NAME_", "demo")
	_ = row.Set("TENANT_ID_", "t1")
	_ = row.Set("DEPLOY_TIME_", time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC))
	return *row
}

// byteRow 构造字节数组行。
// @param deploymentID 部署 ID
// @param name 资源名
// @param data 内容
// @return 行
func byteRow(t *testing.T, deploymentID, name string, data []byte) Row {
	t.Helper()
	row, _ := NewRow("ACT_GE_BYTEARRAY")
	_ = row.Set("ID_", byteArrayTestID(deploymentID, name))
	_ = row.Set("REV_", 1)
	_ = row.Set("NAME_", name)
	_ = row.Set("DEPLOYMENT_ID_", deploymentID)
	_ = row.Set("BYTES_", data)
	_ = row.Set("GENERATED_", false)
	return *row
}

// procRow 构造流程定义行。
// @param id 定义 ID
// @param deploymentID 部署 ID
// @param resource 资源名
// @return 行
func procRow(t *testing.T, id, deploymentID, resource string) Row {
	t.Helper()
	row, _ := NewRow("ACT_RE_PROCDEF")
	_ = row.Set("ID_", id)
	_ = row.Set("REV_", 1)
	_ = row.Set("CATEGORY_", "")
	_ = row.Set("NAME_", "Demo")
	_ = row.Set("KEY_", "demo")
	_ = row.Set("VERSION_", 1)
	_ = row.Set("DEPLOYMENT_ID_", deploymentID)
	_ = row.Set("RESOURCE_NAME_", resource)
	_ = row.Set("DGRM_RESOURCE_NAME_", "")
	_ = row.Set("DESCRIPTION_", "")
	_ = row.Set("HAS_START_FORM_KEY_", false)
	_ = row.Set("HAS_GRAPHICAL_NOTATION_", false)
	_ = row.Set("SUSPENSION_STATE_", 1)
	_ = row.Set("TENANT_ID_", "t1")
	_ = row.Set("ENGINE_VERSION_", "8.1.0.3")
	_ = row.Set("DERIVED_FROM_", "")
	_ = row.Set("DERIVED_FROM_ROOT_", "")
	_ = row.Set("DERIVED_VERSION_", 0)
	return *row
}

// byteArrayTestID 生成测试字节数组 ID。
// @param deploymentID 部署 ID
// @param name 资源名
// @return ID
func byteArrayTestID(deploymentID, name string) string {
	return "ba-" + deploymentID + "-" + name
}

// modelRow 构造模型行。
// @param t 测试上下文
// @param id 模型 ID
// @return 行
func modelRow(t *testing.T, id string) Row {
	t.Helper()
	row, _ := NewRow("ACT_RE_MODEL")
	_ = row.Set("ID_", id)
	_ = row.Set("REV_", 1)
	_ = row.Set("NAME_", "Model")
	_ = row.Set("KEY_", "model")
	_ = row.Set("CATEGORY_", "cat")
	_ = row.Set("CREATE_TIME_", time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC))
	_ = row.Set("LAST_UPDATE_TIME_", time.Date(2026, 10, 28, 1, 0, 0, 0, time.UTC))
	_ = row.Set("VERSION_", 2)
	_ = row.Set("META_INFO_", "{}")
	_ = row.Set("DEPLOYMENT_ID_", "")
	_ = row.Set("EDITOR_SOURCE_VALUE_ID_", "")
	_ = row.Set("EDITOR_SOURCE_EXTRA_VALUE_ID_", "")
	_ = row.Set("TENANT_ID_", "t1")
	return *row
}

// propertyRow 构造属性行。
// @param name 名称
// @param value 值
// @return 行
func propertyRow(t *testing.T, name, value string) Row {
	t.Helper()
	row, _ := NewRow("ACT_GE_PROPERTY")
	_ = row.Set("NAME_", name)
	_ = row.Set("VALUE_", value)
	_ = row.Set("REV_", 1)
	return *row
}

// TestDecodePropertyRows 验证自定义属性恢复。
// @param t 测试上下文
func TestDecodePropertyRows(t *testing.T) {
	rows := []Row{
		propertyRow(t, "feature.x", "1"),
		propertyRow(t, "schema.version", "8.1.0.3"),
	}
	snapshot, err := DecodeRows(rows)
	if err != nil {
		t.Fatalf("恢复属性失败: %v", err)
	}
	if snapshot.Properties["feature.x"] != "1" {
		t.Fatalf("自定义属性恢复错误: %+v", snapshot.Properties)
	}
	if _, ok := snapshot.Properties["schema.version"]; ok {
		t.Fatalf("内置属性不应导入: %+v", snapshot.Properties)
	}
}

// TestDecodeModelRows 验证模型与源码恢复。
// @param t 测试上下文
func TestDecodeModelRows(t *testing.T) {
	rows := []Row{
		modelRow(t, "m1"),
		byteRow(t, "", ModelSourceName("m1", false), []byte("<xml/>")),
		byteRow(t, "", ModelSourceName("m1", true), []byte("{}")),
		modelRow(t, "m2"),
	}
	snapshot, err := DecodeRows(rows)
	if err != nil {
		t.Fatalf("恢复模型失败: %v", err)
	}
	if len(snapshot.Models) != 2 || snapshot.Models[0].Info.ID != "m1" || snapshot.Models[0].Info.Version != 2 {
		t.Fatalf("模型恢复错误: %+v", snapshot.Models)
	}
	if string(snapshot.Models[0].Source) != "<xml/>" || string(snapshot.Models[0].SourceExtra) != "{}" {
		t.Fatalf("模型源码恢复错误: %+v", snapshot.Models[0])
	}
	if snapshot.Models[1].Source != nil {
		t.Fatalf("无源码模型应保持 nil")
	}
	if ModelSourceName("m", false) != "model/m/source" || ModelSourceName("m", true) != "model/m/extra" {
		t.Fatalf("源码名称错误")
	}
}

// TestDecodeRows 验证行恢复。
// @param t 测试上下文
func TestDecodeRows(t *testing.T) {
	rows := []Row{
		deploymentRow(t, "d1"),
		byteRow(t, "d1", "demo.bpmn20.xml", []byte("xml")),
		procRow(t, "p1", "d1", "demo.bpmn20.xml"),
	}
	snapshot, err := DecodeRows(rows)
	if err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	if len(snapshot.Deployments) != 1 || snapshot.Deployments[0].ID != "d1" || snapshot.Deployments[0].TenantID != "t1" {
		t.Fatalf("部署恢复错误: %+v", snapshot.Deployments)
	}
	if len(snapshot.Definitions) != 1 || snapshot.Definitions[0].Key != "demo" || snapshot.Definitions[0].Version != 1 {
		t.Fatalf("定义恢复错误: %+v", snapshot.Definitions)
	}
	if string(snapshot.Resources["d1"]["demo.bpmn20.xml"]) != "xml" {
		t.Fatalf("资源恢复错误")
	}
}

// TestDecodeRows_Errors 验证行恢复错误。
// @param t 测试上下文
func TestDecodeRows_Errors(t *testing.T) {
	cases := [][]Row{
		{{Table: "ACT_UNKNOWN"}},
		{func() Row { row := deploymentRow(t, "d1"); delete(row.Values, "NAME_"); return row }()},
		{func() Row { row := deploymentRow(t, "d1"); row.Values["DEPLOY_TIME_"] = "bad"; return row }()},
		{func() Row { row := procRow(t, "p1", "d1", "x"); return row }()},
		{func() Row { row := byteRow(t, "d1", "x", []byte("x")); row.Values["BYTES_"] = "bad"; return row }()},
		{procRow(t, "p1", "d1", "missing")},
		{func() Row { row := modelRow(t, "m1"); delete(row.Values, "NAME_"); return row }()},
		{func() Row { row := modelRow(t, "m1"); row.Values["VERSION_"] = "bad"; return row }()},
		{func() Row { row := propertyRow(t, "x", "1"); delete(row.Values, "NAME_"); return row }()},
		{func() Row { row := propertyRow(t, "x", "1"); delete(row.Values, "VALUE_"); return row }()},
	}
	for _, rows := range cases {
		if _, err := DecodeRows(rows); err == nil {
			t.Fatalf("期望解析失败: %+v", rows)
		}
	}
}
