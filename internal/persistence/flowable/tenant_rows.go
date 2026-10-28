package flowable

import "strings"

// FilterRowsByTenant 对全量或预过滤行按资源归属过滤，包括无租户列的关联表。
// @param rows 待过滤行
// @param tenantID 目标租户，空字符串只表示默认租户
// @return 当前租户可见行
func FilterRowsByTenant(rows []Row, tenantID string) []Row {
	selected := make(map[string]map[string]bool)
	instances := make(map[string]bool)
	for _, row := range rows {
		if !tableHasColumn(row.Table, "TENANT_ID_") {
			continue
		}
		tenant, _ := row.Values["TENANT_ID_"].(string)
		if tenant != tenantID {
			continue
		}
		if selected[row.Table] == nil {
			selected[row.Table] = make(map[string]bool)
		}
		id, _ := rowIDValue(row)
		selected[row.Table][id] = true
		if row.Table == "ACT_RU_EXECUTION" || row.Table == "ACT_HI_PROCINST" {
			instance, _ := row.Values["PROC_INST_ID_"].(string)
			instances[instance] = true
		}
	}
	visible := func(row Row) bool {
		if tableHasColumn(row.Table, "TENANT_ID_") {
			id, _ := rowIDValue(row)
			return selected[row.Table][id]
		}
		instance, _ := row.Values["PROC_INST_ID_"].(string)
		task, _ := row.Values["TASK_ID_"].(string)
		definition, _ := row.Values["PROC_DEF_ID_"].(string)
		execution, _ := row.Values["EXECUTION_ID_"].(string)
		// 无租户列必须从实际归属推断，缺少归属时不能默认为跨租户可见。
		switch row.Table {
		case "ACT_RU_VARIABLE":
			return selected["ACT_RU_EXECUTION"][execution] || selected["ACT_RU_TASK"][task]
		case "ACT_RU_IDENTITYLINK":
			return selected["ACT_RU_TASK"][task] || instances[instance] || selected["ACT_RE_PROCDEF"][definition]
		case "ACT_HI_IDENTITYLINK", "ACT_HI_DETAIL", "ACT_HI_VARINST", "ACT_HI_COMMENT", "ACT_HI_ATTACHMENT":
			return instances[instance] || selected["ACT_HI_TASKINST"][task]
		case "ACT_GE_PROPERTY":
			return true
		default:
			return false
		}
	}
	out := make([]Row, 0, len(rows))
	byteIDs := make(map[string]bool)
	for _, row := range rows {
		if !visible(row) {
			continue
		}
		out = append(out, row)
		for _, column := range []string{"BYTEARRAY_ID_", "EDITOR_SOURCE_VALUE_ID_", "EDITOR_SOURCE_EXTRA_VALUE_ID_", "CONTENT_ID_", "EXCEPTION_STACK_ID_", "CUSTOM_VALUES_ID_"} {
			if id, ok := row.Values[column].(string); ok && id != "" {
				byteIDs[id] = true
			}
		}
	}
	for _, row := range rows {
		if row.Table != "ACT_GE_BYTEARRAY" {
			continue
		}
		deployment, _ := row.Values["DEPLOYMENT_ID_"].(string)
		id, _ := rowIDValue(row)
		name, _ := row.Values["NAME_"].(string)
		if selected["ACT_RE_DEPLOYMENT"][deployment] || byteIDs[id] || instances[strings.TrimPrefix(name, "flowable-go-control/")] {
			out = append(out, row)
		}
	}
	return out
}
