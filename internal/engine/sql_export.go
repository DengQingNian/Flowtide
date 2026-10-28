package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"

	flowablerow "github.com/flowable-go/flowable/internal/persistence/flowable"
)

// ExportRows 导出部署、流程定义与字节数组行，供按表仓储写入。
// @param ctx 上下文
// @return 行列表
// @return 导出错误
func (e *Engine) ExportRows(ctx context.Context) ([]flowablerow.Row, error) {
	return e.readRows(e.repositoryRowsLocked)
}

// repositoryRowsLocked 在锁内导出仓库行。
// @return 行列表
// @return 导出错误
func (e *Engine) repositoryRowsLocked() ([]flowablerow.Row, error) {

	rows := make([]flowablerow.Row, 0)
	deployments := make([]*deploymentState, 0, len(e.state.Deployments))
	for _, deployment := range e.state.Deployments {
		deployments = append(deployments, deployment)
	}
	sort.Slice(deployments, func(i, j int) bool { return deployments[i].Info.ID < deployments[j].Info.ID })
	for _, deployment := range deployments {
		deploymentRow, rowErr := flowablerow.NewRow("ACT_RE_DEPLOYMENT")
		if rowErr != nil {
			return nil, rowErr
		}
		_ = deploymentRow.Set("ID_", deployment.Info.ID)
		_ = deploymentRow.Set("NAME_", deployment.Info.Name)
		_ = deploymentRow.Set("CATEGORY_", "")
		_ = deploymentRow.Set("KEY_", "")
		_ = deploymentRow.Set("TENANT_ID_", deployment.Info.TenantID)
		_ = deploymentRow.Set("DEPLOY_TIME_", deployment.Info.DeployTime)
		_ = deploymentRow.Set("DERIVED_FROM_", "")
		_ = deploymentRow.Set("DERIVED_FROM_ROOT_", "")
		_ = deploymentRow.Set("PARENT_DEPLOYMENT_ID_", "")
		_ = deploymentRow.Set("ENGINE_VERSION_", "8.1.0.3")
		rows = append(rows, *deploymentRow)

		names := make([]string, 0, len(deployment.Resources))
		for name := range deployment.Resources {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			byteRow, byteErr := flowablerow.NewRow("ACT_GE_BYTEARRAY")
			if byteErr != nil {
				return nil, byteErr
			}
			_ = byteRow.Set("ID_", byteArrayID(deployment.Info.ID, name))
			_ = byteRow.Set("REV_", 1)
			_ = byteRow.Set("NAME_", name)
			_ = byteRow.Set("DEPLOYMENT_ID_", deployment.Info.ID)
			_ = byteRow.Set("BYTES_", deployment.Resources[name])
			_ = byteRow.Set("GENERATED_", false)
			rows = append(rows, *byteRow)
		}
	}
	definitions := make([]*definitionState, 0, len(e.state.Definitions))
	for _, definition := range e.state.Definitions {
		definitions = append(definitions, definition)
	}
	sort.Slice(definitions, func(i, j int) bool { return definitions[i].Info.ID < definitions[j].Info.ID })
	for _, definition := range definitions {
		procRow, rowErr := flowablerow.NewRow("ACT_RE_PROCDEF")
		if rowErr != nil {
			return nil, rowErr
		}
		suspension := 1
		if definition.Info.Suspended {
			suspension = 2
		}
		_ = procRow.Set("ID_", definition.Info.ID)
		_ = procRow.Set("REV_", 1)
		_ = procRow.Set("CATEGORY_", "")
		_ = procRow.Set("NAME_", definition.Info.Name)
		_ = procRow.Set("KEY_", definition.Info.Key)
		_ = procRow.Set("VERSION_", definition.Info.Version)
		_ = procRow.Set("DEPLOYMENT_ID_", definition.Info.DeploymentID)
		_ = procRow.Set("RESOURCE_NAME_", definition.Info.ResourceName)
		_ = procRow.Set("DGRM_RESOURCE_NAME_", "")
		_ = procRow.Set("DESCRIPTION_", "")
		_ = procRow.Set("HAS_START_FORM_KEY_", false)
		_ = procRow.Set("HAS_GRAPHICAL_NOTATION_", false)
		_ = procRow.Set("SUSPENSION_STATE_", suspension)
		_ = procRow.Set("TENANT_ID_", definition.Info.TenantID)
		_ = procRow.Set("ENGINE_VERSION_", "8.1.0.3")
		_ = procRow.Set("DERIVED_FROM_", "")
		_ = procRow.Set("DERIVED_FROM_ROOT_", "")
		_ = procRow.Set("DERIVED_VERSION_", 0)
		rows = append(rows, *procRow)
	}
	models := make([]*modelState, 0, len(e.state.Models))
	for _, model := range e.state.Models {
		models = append(models, model)
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Info.ID < models[j].Info.ID })
	for _, model := range models {
		sourceID, extraID := "", ""
		if model.Source != nil {
			name := flowablerow.ModelSourceName(model.Info.ID, false)
			sourceID = byteArrayID("", name)
			byteRow, byteErr := flowablerow.NewRow("ACT_GE_BYTEARRAY")
			if byteErr != nil {
				return nil, byteErr
			}
			_ = byteRow.Set("ID_", sourceID)
			_ = byteRow.Set("REV_", 1)
			_ = byteRow.Set("NAME_", name)
			_ = byteRow.Set("DEPLOYMENT_ID_", "")
			_ = byteRow.Set("BYTES_", model.Source)
			_ = byteRow.Set("GENERATED_", false)
			rows = append(rows, *byteRow)
		}
		if model.SourceExtra != nil {
			name := flowablerow.ModelSourceName(model.Info.ID, true)
			extraID = byteArrayID("", name)
			byteRow, byteErr := flowablerow.NewRow("ACT_GE_BYTEARRAY")
			if byteErr != nil {
				return nil, byteErr
			}
			_ = byteRow.Set("ID_", extraID)
			_ = byteRow.Set("REV_", 1)
			_ = byteRow.Set("NAME_", name)
			_ = byteRow.Set("DEPLOYMENT_ID_", "")
			_ = byteRow.Set("BYTES_", model.SourceExtra)
			_ = byteRow.Set("GENERATED_", false)
			rows = append(rows, *byteRow)
		}
		modelRow, rowErr := flowablerow.NewRow("ACT_RE_MODEL")
		if rowErr != nil {
			return nil, rowErr
		}
		_ = modelRow.Set("ID_", model.Info.ID)
		_ = modelRow.Set("REV_", 1)
		_ = modelRow.Set("NAME_", model.Info.Name)
		_ = modelRow.Set("KEY_", model.Info.Key)
		_ = modelRow.Set("CATEGORY_", model.Info.Category)
		_ = modelRow.Set("CREATE_TIME_", model.Info.CreateTime)
		_ = modelRow.Set("LAST_UPDATE_TIME_", model.Info.LastUpdateTime)
		_ = modelRow.Set("VERSION_", model.Info.Version)
		_ = modelRow.Set("META_INFO_", model.Info.MetaInfo)
		_ = modelRow.Set("DEPLOYMENT_ID_", model.Info.DeploymentID)
		_ = modelRow.Set("EDITOR_SOURCE_VALUE_ID_", sourceID)
		_ = modelRow.Set("EDITOR_SOURCE_EXTRA_VALUE_ID_", extraID)
		_ = modelRow.Set("TENANT_ID_", model.Info.TenantID)
		rows = append(rows, *modelRow)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Table == rows[j].Table {
			return rowID(rows[i]) < rowID(rows[j])
		}
		return rows[i].Table < rows[j].Table
	})
	return rows, nil
}

// byteArrayID 生成部署资源的稳定字节数组 ID。
// @param deploymentID 部署 ID
// @param name 资源名
// @return 字节数组 ID
func byteArrayID(deploymentID, name string) string {
	sum := sha256.Sum256([]byte(deploymentID + "|" + name))
	return "ba-" + hex.EncodeToString(sum[:])[:32]
}

// rowID 返回行的 ID_ 字符串。
// @param row 行
// @return ID
func rowID(row flowablerow.Row) string {
	if value, ok := row.Values["ID_"].(string); ok {
		return value
	}
	return ""
}
