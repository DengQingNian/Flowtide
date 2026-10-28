package flowable

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/flowable-go/flowable/api"
)

// ModelRecord 是流程模型元数据与源码。
type ModelRecord struct {
	Info        api.Model // 模型元数据
	Source      []byte    // 编辑源
	SourceExtra []byte    // 扩展源码
}

// ModelSourceName 返回模型源在 ACT_GE_BYTEARRAY 中的名称。
// @param modelID 模型 ID
// @param extra 是否扩展源
// @return 资源名称
func ModelSourceName(modelID string, extra bool) string {
	if extra {
		return "model/" + modelID + "/extra"
	}
	return "model/" + modelID + "/source"
}

// Snapshot 是从 ACT 行恢复的仓库数据。
type Snapshot struct {
	Deployments        []api.Deployment             // 部署
	Resources          map[string]map[string][]byte // 部署 ID 到资源
	Definitions        []api.ProcessDefinition      // 流程定义
	Models             []ModelRecord                // 流程模型
	Properties         map[string]string            // 自定义属性
	AttachmentContents map[string][]byte            // 附件内容
}

// DecodeRows 将 ACT 行恢复为部署、资源与流程定义。
// @param rows 行列表
// @return 仓库快照
// @return 解析错误
func DecodeRows(rows []Row) (Snapshot, error) {
	snapshot := Snapshot{
		Deployments:        make([]api.Deployment, 0),
		Resources:          make(map[string]map[string][]byte),
		Definitions:        make([]api.ProcessDefinition, 0),
		Models:             make([]ModelRecord, 0),
		Properties:         make(map[string]string),
		AttachmentContents: make(map[string][]byte),
	}
	modelSources := make(map[string][]byte)
	for _, row := range rows {
		switch row.Table {
		case "ACT_RE_DEPLOYMENT":
			deployment, err := decodeDeployment(row)
			if err != nil {
				return Snapshot{}, err
			}
			snapshot.Deployments = append(snapshot.Deployments, deployment)
		case "ACT_GE_BYTEARRAY":
			deploymentID, err := stringValue(row, "DEPLOYMENT_ID_")
			if err != nil {
				return Snapshot{}, err
			}
			name, err := stringValue(row, "NAME_")
			if err != nil {
				return Snapshot{}, err
			}
			data, err := bytesValue(row, "BYTES_")
			if err != nil {
				return Snapshot{}, err
			}
			if strings.HasPrefix(name, "flowable-go-") {
				continue
			}
			// 模型源使用 model/{id}/source 命名，与部署资源区分
			if strings.HasPrefix(name, "model/") {
				modelSources[name] = data
				continue
			}
			// 附件内容使用 attachment/{id} 命名
			if strings.HasPrefix(name, "attachment/") {
				snapshot.AttachmentContents[strings.TrimPrefix(name, "attachment/")] = data
				continue
			}
			if snapshot.Resources[deploymentID] == nil {
				snapshot.Resources[deploymentID] = make(map[string][]byte)
			}
			snapshot.Resources[deploymentID][name] = data
		case "ACT_GE_PROPERTY":
			name, err := stringValue(row, "NAME_")
			if err != nil {
				return Snapshot{}, err
			}
			value, err := stringValue(row, "VALUE_")
			if err != nil {
				return Snapshot{}, err
			}
			if !isSchemaProperty(name) {
				snapshot.Properties[name] = value
			}
		case "ACT_RE_MODEL":
			model, err := decodeModel(row)
			if err != nil {
				return Snapshot{}, err
			}
			snapshot.Models = append(snapshot.Models, ModelRecord{Info: model})
		case "ACT_RE_PROCDEF":
			definition, err := decodeDefinition(row)
			if err != nil {
				return Snapshot{}, err
			}
			snapshot.Definitions = append(snapshot.Definitions, definition)
		default:
			return Snapshot{}, api.NewError("ROW-0010", "不支持的表: "+row.Table, api.ErrBadRequest)
		}
	}
	sort.Slice(snapshot.Deployments, func(i, j int) bool { return snapshot.Deployments[i].ID < snapshot.Deployments[j].ID })
	sort.Slice(snapshot.Definitions, func(i, j int) bool { return snapshot.Definitions[i].ID < snapshot.Definitions[j].ID })
	sort.Slice(snapshot.Models, func(i, j int) bool { return snapshot.Models[i].Info.ID < snapshot.Models[j].Info.ID })
	for index := range snapshot.Models {
		modelID := snapshot.Models[index].Info.ID
		if source, ok := modelSources[ModelSourceName(modelID, false)]; ok {
			snapshot.Models[index].Source = source
		}
		if extra, ok := modelSources[ModelSourceName(modelID, true)]; ok {
			snapshot.Models[index].SourceExtra = extra
		}
	}
	for _, definition := range snapshot.Definitions {
		if _, ok := snapshot.Resources[definition.DeploymentID]; !ok {
			return Snapshot{}, api.NewError("ROW-0011", "流程定义缺少部署资源: "+definition.ID, api.ErrBadRequest)
		}
		if _, ok := snapshot.Resources[definition.DeploymentID][definition.ResourceName]; !ok {
			return Snapshot{}, api.NewError("ROW-0012", "流程定义资源不存在: "+definition.ResourceName, api.ErrBadRequest)
		}
	}
	return snapshot, nil
}

// decodeDeployment 解析部署行。
// @param row 行
// @return 部署
// @return 错误
func decodeDeployment(row Row) (api.Deployment, error) {
	id, err := stringValue(row, "ID_")
	if err != nil {
		return api.Deployment{}, err
	}
	name, err := stringValue(row, "NAME_")
	if err != nil {
		return api.Deployment{}, err
	}
	tenant, err := stringValue(row, "TENANT_ID_")
	if err != nil {
		return api.Deployment{}, err
	}
	deployTime, err := timeValue(row, "DEPLOY_TIME_")
	if err != nil {
		return api.Deployment{}, err
	}
	return api.Deployment{ID: id, Name: name, TenantID: tenant, DeployTime: deployTime}, nil
}

// isSchemaProperty 判断是否为 Schema 内置属性。
// @param name 属性名
// @return 是否内置
func isSchemaProperty(name string) bool {
	if strings.HasPrefix(name, "flowable-go.schema.") {
		return true
	}
	switch name {
	case "common.schema.version", "schema.version", "schema.history", "next.dbid":
		return true
	}
	return false
}

// decodeModel 解析流程模型行。
// @param row 行
// @return 模型
// @return 错误
func decodeModel(row Row) (api.Model, error) {
	id, err := stringValue(row, "ID_")
	if err != nil {
		return api.Model{}, err
	}
	name, err := stringValue(row, "NAME_")
	if err != nil {
		return api.Model{}, err
	}
	key, err := stringValue(row, "KEY_")
	if err != nil {
		return api.Model{}, err
	}
	category, err := stringValue(row, "CATEGORY_")
	if err != nil {
		return api.Model{}, err
	}
	version, err := intValue(row, "VERSION_")
	if err != nil {
		return api.Model{}, err
	}
	metaInfo, err := stringValue(row, "META_INFO_")
	if err != nil {
		return api.Model{}, err
	}
	deploymentID, err := stringValue(row, "DEPLOYMENT_ID_")
	if err != nil {
		return api.Model{}, err
	}
	tenant, err := stringValue(row, "TENANT_ID_")
	if err != nil {
		return api.Model{}, err
	}
	createTime, err := timeValue(row, "CREATE_TIME_")
	if err != nil {
		return api.Model{}, err
	}
	lastUpdate, err := timeValue(row, "LAST_UPDATE_TIME_")
	if err != nil {
		return api.Model{}, err
	}
	return api.Model{
		ID: id, Name: name, Key: key, Category: category, Version: version,
		MetaInfo: metaInfo, DeploymentID: deploymentID, TenantID: tenant,
		CreateTime: createTime, LastUpdateTime: lastUpdate,
	}, nil
}

// decodeDefinition 解析流程定义行。
// @param row 行
// @return 定义
// @return 错误
func decodeDefinition(row Row) (api.ProcessDefinition, error) {
	id, err := stringValue(row, "ID_")
	if err != nil {
		return api.ProcessDefinition{}, err
	}
	key, err := stringValue(row, "KEY_")
	if err != nil {
		return api.ProcessDefinition{}, err
	}
	name, err := stringValue(row, "NAME_")
	if err != nil {
		return api.ProcessDefinition{}, err
	}
	version, err := intValue(row, "VERSION_")
	if err != nil {
		return api.ProcessDefinition{}, err
	}
	deploymentID, err := stringValue(row, "DEPLOYMENT_ID_")
	if err != nil {
		return api.ProcessDefinition{}, err
	}
	resourceName, err := stringValue(row, "RESOURCE_NAME_")
	if err != nil {
		return api.ProcessDefinition{}, err
	}
	tenant, err := stringValue(row, "TENANT_ID_")
	if err != nil {
		return api.ProcessDefinition{}, err
	}
	suspension, err := intValue(row, "SUSPENSION_STATE_")
	if err != nil {
		return api.ProcessDefinition{}, err
	}
	return api.ProcessDefinition{
		ID:           id,
		Key:          key,
		Name:         name,
		Version:      version,
		DeploymentID: deploymentID,
		ResourceName: resourceName,
		TenantID:     tenant,
		Suspended:    suspension == 2,
	}, nil
}

// stringValue 读取字符串列。
// @param row 行
// @param column 列名
// @return 字符串
// @return 错误
func stringValue(row Row, column string) (string, error) {
	value, ok := row.Values[column]
	if !ok {
		return "", api.NewError("ROW-0013", "缺少列: "+column, api.ErrBadRequest)
	}
	if value == nil {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", api.NewError("ROW-0014", fmt.Sprintf("列 %s 不是字符串: %T", column, value), api.ErrBadRequest)
	}
	return text, nil
}

// bytesValue 读取字节列。
// @param row 行
// @param column 列名
// @return 字节
// @return 错误
func bytesValue(row Row, column string) ([]byte, error) {
	value, ok := row.Values[column]
	if !ok {
		return nil, api.NewError("ROW-0013", "缺少列: "+column, api.ErrBadRequest)
	}
	if value == nil {
		return nil, nil
	}
	data, ok := value.([]byte)
	if !ok {
		return nil, api.NewError("ROW-0014", fmt.Sprintf("列 %s 不是字节数组: %T", column, value), api.ErrBadRequest)
	}
	return data, nil
}

// intValue 读取整数列。
// @param row 行
// @param column 列名
// @return 整数
// @return 错误
func intValue(row Row, column string) (int, error) {
	value, ok := row.Values[column]
	if !ok {
		return 0, api.NewError("ROW-0013", "缺少列: "+column, api.ErrBadRequest)
	}
	switch typed := value.(type) {
	case int:
		return typed, nil
	case int64:
		return int(typed), nil
	case float64:
		return int(typed), nil
	default:
		return 0, api.NewError("ROW-0014", fmt.Sprintf("列 %s 不是整数: %T", column, value), api.ErrBadRequest)
	}
}

// timeValue 读取时间列。
// @param row 行
// @param column 列名
// @return 时间
// @return 错误
func timeValue(row Row, column string) (time.Time, error) {
	value, ok := row.Values[column]
	if !ok {
		return time.Time{}, api.NewError("ROW-0013", "缺少列: "+column, api.ErrBadRequest)
	}
	switch typed := value.(type) {
	case time.Time:
		return typed, nil
	case string:
		parsed, err := time.Parse(time.RFC3339, typed)
		if err != nil {
			return time.Time{}, api.NewError("ROW-0014", "时间格式错误: "+column, err)
		}
		return parsed, nil
	default:
		return time.Time{}, api.NewError("ROW-0014", fmt.Sprintf("列 %s 不是时间: %T", column, value), api.ErrBadRequest)
	}
}
