package workflow

// 本文件承载 P6 设计器 XML 存取门面能力：模型创建（create/add/model）、模型 XML 读改
// （getModelJsonByType/export/xml、save/xml）、历史定义 XML（getHisModelJson、updateHisModelXml
// 回填）、复制流程（copyProcess）、模型+步骤联查（findModelAndStepPageList）、步骤详情
// （step/findById）、引擎委托（delegate/task）、权重清空（weight/delete）。

import (
	"context"
	"strings"

	"github.com/DengQingNian/Flowtide/api"
)

// CreatedProcessModel 是设计器建模结果（引擎模型与业务模型绑定信息）。
type CreatedProcessModel struct {
	EngineModelID string // 引擎模型 Id（model_id）
	BusinessID    string // 业务模型/分类主键（workflow_model.id）
	ModelCode     string // 模型代号（=流程定义 Key）
	ModelName     string // 模型名称
}

// CreateProcessModel 设计器建模型：创建引擎模型并写入编辑源，同步建立业务模型绑定（EngineModelID）。
// @param ctx 上下文
// @param tenantID 租户（业务系统代号）
// @param name 模型名称（必填）
// @param modelCode 模型代号 = 流程定义 Key（必填）
// @param xml BPMN XML 源（可空，空则仅建骨架）
// @return 统一返回（data 为 CreatedProcessModel）
// @return 错误
func (s *Service) CreateProcessModel(ctx context.Context, tenantID, name, modelCode, xml string) (R, error) {
	if err := requireMgmt(s.comp); err != nil {
		return R{}, err
	}
	if strings.TrimSpace(name) == "" || strings.TrimSpace(modelCode) == "" || strings.TrimSpace(tenantID) == "" {
		return R{}, api.NewError("WORKFLOW-0400", "建模缺少名称、代号或租户", api.ErrBadRequest)
	}
	// 引擎模型（代号唯一性由业务模型查重保证，引擎侧允许跨版本同名）
	engineModel, err := s.engine.RepositoryService().CreateModel(ctx, api.CreateModelRequest{
		Name: name, Key: modelCode, TenantID: tenantID,
	})
	if err != nil {
		return R{}, err
	}
	// 编辑源非空时写入（空源骨架在保存 XML 后补齐）
	if strings.TrimSpace(xml) != "" {
		if err := s.engine.RepositoryService().SetModelSource(ctx, engineModel.ID, []byte(xml)); err != nil {
			return R{}, err
		}
	}
	// 业务模型绑定（代号唯一校验在 ModelService.Add）
	saved, err := s.comp.modelMgmt.Add(ctx, &ModelCategory{
		ModelName: name, ModelCode: modelCode, TenantID: tenantID, EngineModelID: engineModel.ID,
	})
	if err != nil {
		return R{}, err
	}
	return ROkMsg("创建成功", CreatedProcessModel{
		EngineModelID: engineModel.ID, BusinessID: saved.ID, ModelCode: modelCode, ModelName: name,
	}), nil
}

// GetModelXML 读取业务模型绑定的引擎模型编辑源（getModelJsonByType type=model 与 export/xml 数据面）。
// @param ctx 上下文
// @param modelID 业务模型主键（workflow_model.id）
// @return 统一返回（data 为 {modelId, engineModelId, modelCode, xml}）
// @return 错误
func (s *Service) GetModelXML(ctx context.Context, modelID string) (R, error) {
	m, xml, err := s.modelXML(ctx, modelID)
	if err != nil {
		return R{}, err
	}
	return ROk(map[string]any{
		"modelId": m.ID, "engineModelId": m.EngineModelID, "modelCode": m.ModelCode, "xml": string(xml),
	}), nil
}

// modelXML 定位业务模型并读取引擎模型编辑源。
// @param ctx 上下文
// @param modelID 业务模型主键
// @return 业务模型
// @return XML 源
// @return 错误
func (s *Service) modelXML(ctx context.Context, modelID string) (*ModelCategory, []byte, error) {
	m, err := s.boundModel(ctx, modelID)
	if err != nil {
		return nil, nil, err
	}
	source, err := s.engine.RepositoryService().GetModelSource(ctx, m.EngineModelID)
	if err != nil {
		return nil, nil, err
	}
	return m, source, nil
}

// boundModel 定位业务模型并校验引擎模型绑定（不读源：空源骨架模型也可定位）。
// @param ctx 上下文
// @param modelID 业务模型主键
// @return 业务模型
// @return 错误
func (s *Service) boundModel(ctx context.Context, modelID string) (*ModelCategory, error) {
	if err := requireMgmt(s.comp); err != nil {
		return nil, err
	}
	if strings.TrimSpace(modelID) == "" {
		return nil, api.NewError("WORKFLOW-0400", "参数 modelId 为空", api.ErrBadRequest)
	}
	m, found, err := s.comp.modelMgmt.GetByID(ctx, modelID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, api.NewError("WORKFLOW-1404", "流程模型不存在", api.ErrNotFound)
	}
	if m.EngineModelID == "" {
		return nil, api.NewError("WORKFLOW-0400", "模型未绑定引擎侧模型", api.ErrBadRequest)
	}
	return m, nil
}

// SaveModelXML 保存业务模型的 BPMN XML 编辑源（save/xml；空源骨架模型可直接首存）。
// @param ctx 上下文
// @param modelID 业务模型主键
// @param xml BPMN XML 源
// @return 统一返回
// @return 错误
func (s *Service) SaveModelXML(ctx context.Context, modelID, xml string) (R, error) {
	m, err := s.boundModel(ctx, modelID)
	if err != nil {
		return R{}, err
	}
	if strings.TrimSpace(xml) == "" {
		return R{}, api.NewError("WORKFLOW-0400", "XML 源为空", api.ErrBadRequest)
	}
	if err := s.engine.RepositoryService().SetModelSource(ctx, m.EngineModelID, []byte(xml)); err != nil {
		return R{}, err
	}
	return ROkMsg("保存成功", nil), nil
}

// GetDefinitionXML 读取流程定义的 BPMN XML（getHisModelJson / getProcessDefinitionJsonInfo 数据面）。
// @param ctx 上下文
// @param definitionID 流程定义 Id
// @return 统一返回（data 为 {definitionId, definitionKey, xml}）
// @return 错误
func (s *Service) GetDefinitionXML(ctx context.Context, definitionID string) (R, error) {
	if err := requireMgmt(s.comp); err != nil {
		return R{}, err
	}
	if strings.TrimSpace(definitionID) == "" {
		return R{}, api.NewError("WORKFLOW-0400", "参数 definitionId 为空", api.ErrBadRequest)
	}
	definition, err := s.engine.RepositoryService().GetProcessDefinition(ctx, definitionID)
	if err != nil {
		return R{}, err
	}
	// 定义资源即其 BPMN XML（ResourceName 为部署时资源名）
	xml, err := s.engine.RepositoryService().GetDeploymentResource(ctx, definition.DeploymentID, definition.ResourceName)
	if err != nil {
		return R{}, err
	}
	return ROk(map[string]any{
		"definitionId": definition.ID, "definitionKey": definition.Key, "xml": string(xml),
	}), nil
}

// UpdateModelFromDefinition 将历史定义 XML 回填到同 Key 业务模型的引擎模型源
// （updateHisModelXml：复制历史版本到设计器再编辑保存的入口）。
// @param ctx 上下文
// @param tenantID 租户（业务系统代号）
// @param definitionID 历史流程定义 Id
// @return 统一返回
// @return 错误
func (s *Service) UpdateModelFromDefinition(ctx context.Context, tenantID, definitionID string) (R, error) {
	defR, err := s.GetDefinitionXML(ctx, definitionID)
	if err != nil {
		return R{}, err
	}
	data := defR.Data.(map[string]any)
	key := data["definitionKey"].(string)
	m, found, err := s.comp.modelMgmt.GetByCode(ctx, tenantID, key)
	if err != nil {
		return R{}, err
	}
	if !found {
		return R{}, api.NewError("WORKFLOW-1404", "流程模型不存在: "+key, api.ErrNotFound)
	}
	return s.SaveModelXML(ctx, m.ID, data["xml"].(string))
}

// CopyProcess 复制源模型 XML 到目标模型列表（copyProcess：resourceId=源业务模型，modelIdList=目标业务模型）。
// 目标未绑定引擎模型时自动创建引擎模型再回填绑定。
// @param ctx 上下文
// @param sourceModelID 源业务模型主键
// @param targetModelIDs 目标业务模型主键列表
// @return 统一返回（data 含 copied 数）
// @return 错误
func (s *Service) CopyProcess(ctx context.Context, sourceModelID string, targetModelIDs []string) (R, error) {
	if err := requireMgmt(s.comp); err != nil {
		return R{}, err
	}
	_, sourceXML, err := s.modelXML(ctx, sourceModelID)
	if err != nil {
		return R{}, err
	}
	copied := 0
	for _, targetID := range targetModelIDs {
		if targetID == "" || targetID == sourceModelID {
			continue
		}
		target, found, err := s.comp.modelMgmt.GetByID(ctx, targetID)
		if err != nil {
			return R{}, err
		}
		if !found {
			return R{}, api.NewError("WORKFLOW-1404", "目标流程模型不存在: "+targetID, api.ErrNotFound)
		}
		engineModelID := target.EngineModelID
		if engineModelID == "" {
			// 目标未建引擎模型：按目标名称/代号创建后回填绑定
			engineModel, err := s.engine.RepositoryService().CreateModel(ctx, api.CreateModelRequest{
				Name: target.ModelName, Key: target.ModelCode, TenantID: target.TenantID,
			})
			if err != nil {
				return R{}, err
			}
			engineModelID = engineModel.ID
			target.EngineModelID = engineModelID
			if _, err := s.comp.modelMgmt.Edit(ctx, target); err != nil {
				return R{}, err
			}
		}
		if err := s.engine.RepositoryService().SetModelSource(ctx, engineModelID, sourceXML); err != nil {
			return R{}, err
		}
		copied++
	}
	return ROkMsg("复制成功", map[string]any{"copied": copied}), nil
}

// GetModelAndStepPage 查询租户下模型列表并附带各自设计时步骤（findModelAndStepPageList 数据面）。
// @param ctx 上下文
// @param tenantID 租户
// @return 统一返回（data 为 {model, steps} 列表）
// @return 错误
func (s *Service) GetModelAndStepPage(ctx context.Context, tenantID string) (R, error) {
	if err := requireMgmt(s.comp); err != nil {
		return R{}, err
	}
	models, err := s.comp.modelMgmt.List(ctx, tenantID)
	if err != nil {
		return R{}, err
	}
	rows := make([]map[string]any, 0, len(models))
	for _, model := range models {
		steps, err := s.comp.stepDesign.ListByModel(ctx, model.ID)
		if err != nil {
			return R{}, err
		}
		rows = append(rows, map[string]any{"model": model, "steps": steps})
	}
	return ROk(rows), nil
}

// GetStepDesign 按主键查询设计时步骤（step/findById 数据面）。
// @param ctx 上下文
// @param id 步骤主键
// @return 统一返回（data 为步骤实体，未命中 data 为 null）
// @return 错误
func (s *Service) GetStepDesign(ctx context.Context, id string) (R, error) {
	if err := requireMgmt(s.comp); err != nil {
		return R{}, err
	}
	if strings.TrimSpace(id) == "" {
		return R{}, api.NewError("WORKFLOW-0400", "参数 id 为空", api.ErrBadRequest)
	}
	step, found, err := s.comp.stepDesign.GetByID(ctx, id)
	if err != nil {
		return R{}, err
	}
	if !found {
		return ROk(nil), nil
	}
	return ROk(step), nil
}

// DelegateTask 委派任务（delegate/task：处理人转他人办理，resolve 归还）。
// @param ctx 上下文
// @param taskID 任务 ID
// @param userID 被委派人身份 Id
// @return 统一返回
// @return 错误
func (s *Service) DelegateTask(ctx context.Context, taskID, userID string) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	if taskID == "" || userID == "" {
		return R{}, api.NewError("WORKFLOW-0400", "委派缺少任务或被委派人", api.ErrBadRequest)
	}
	if err := s.engine.TaskService().Delegate(ctx, taskID, userID); err != nil {
		return R{}, err
	}
	return ROkMsg("委派成功", nil), nil
}

// DeleteCountersignWeights 清空节点会签权重配置（weight/delete：SaveByNode 先删后插空集）。
// @param ctx 上下文
// @param processDefinitionKey 流程定义 Key
// @param activityID 节点 ID
// @param tenantID 租户（空不隔离）
// @return 统一返回
// @return 错误
func (s *Service) DeleteCountersignWeights(ctx context.Context, processDefinitionKey, activityID, tenantID string) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	if processDefinitionKey == "" || activityID == "" {
		return R{}, api.NewError("WORKFLOW-0400", "权重删除缺少定义 Key 或节点 ID", api.ErrBadRequest)
	}
	key := CountersignWeightKey{ProcessDefinitionKey: processDefinitionKey, ActivityID: activityID, TenantID: tenantID}
	if err := s.comp.counter.SaveNodeWeights(ctx, key, nil); err != nil {
		return R{}, err
	}
	return ROkMsg("删除成功", nil), nil
}

// GetUserTaskMetas 查询定义上全部用户任务节点元数据（getUserTaskJsonInfo 数据面：
// 设计器属性面板按定义读取各节点的执行人/步骤/权重扩展配置）。
// @param ctx 上下文
// @param definitionID 流程定义 Id
// @return 统一返回（data 为节点元数据列表）
// @return 错误
func (s *Service) GetUserTaskMetas(ctx context.Context, definitionID string) (R, error) {
	if err := requireMgmt(s.comp); err != nil {
		return R{}, err
	}
	if strings.TrimSpace(definitionID) == "" {
		return R{}, api.NewError("WORKFLOW-0400", "参数 definitionId 为空", api.ErrBadRequest)
	}
	metas, err := s.comp.meta.ListUserTaskMetas(ctx, definitionID)
	if err != nil {
		return R{}, err
	}
	return ROk(metas), nil
}
