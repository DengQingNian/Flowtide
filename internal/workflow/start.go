// start.go 实现流程发起域服务（P1-D 核心闭环前半）。
// 对位 Java ProcessesUtil#publishAndGetFirstTaskByKey:987 与 saveFirstTaskUserNextActivity:1142：
// 五参校验 → 部署校验 → 协议变量注入 → 引擎发起 → 首任务执行者解析 →
// 首步预生成（used=1 + taskId）→ 提交意见(TJ)/操作记录/待办/已办镜像/表单状态落位。
package workflow

import (
	"context"
	"fmt"
	"strings"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/tenant"
)

// 发起域服务错误码（WORKFLOW-165x 段为本文件专用段，勿挪用）。
const (
	// errCodeStartParam 发起参数缺失。
	errCodeStartParam = "WORKFLOW-1650"
	// errCodeStartNotDeployed 流程未部署。
	errCodeStartNotDeployed = "WORKFLOW-1651"
	// errCodeStartNoTask 发起后未找到首个待办任务。
	errCodeStartNoTask = "WORKFLOW-1652"
	// errCodeStartNoExecutor 首步未设置执行者。
	errCodeStartNoExecutor = "WORKFLOW-1653"
	// errCodeStartNoIdentity 组展开后执行人身份不存在。
	errCodeStartNoIdentity = "WORKFLOW-1654"
)

// StartResult 是发起流程结果（对位 Java publishAndGetFirstTaskByKey 返回 Map）。
type StartResult struct {
	ProcessInstanceID   string // 流程实例 Id
	ProcessDefinitionID string // 流程定义 Id
	TaskID              string // 首个用户任务 Id
	TaskName            string // 首个用户任务名称
	TaskKey             string // 首个用户任务节点 Key（taskDefinitionKey）
}

// StartService 是流程发起域服务。
// 依赖注入风格与既有域服务一致：引擎门面 + 各域服务（意见/操作记录/待办/已办/表单/预生成）。
type StartService struct {
	engine     api.Engine              // 引擎门面（七服务）
	meta       *ModelMetaService       // 模型元数据服务（首任务执行者配置）
	next       *NextActivityService    // 下一步预生成域服务（首步记账）
	comments   *CommentService         // 审批意见服务（提交 TJ 意见）
	operations *OperationRecordService // 操作记录服务（TJ 留痕）
	messages   *MessageExtendService   // 待办扩展服务（待办中心数据源）
	histories  *HistoryExtendService   // 已办扩展服务（待办镜像）
	documents  *DocumentService        // 业务表单状态服务（document 落位）
}

// NewStartService 构造流程发起域服务。
// @param engine 引擎门面（七服务）
// @param meta 模型元数据服务
// @param next 下一步预生成域服务
// @param comments 审批意见服务
// @param operations 操作记录服务
// @param messages 待办扩展服务
// @param histories 已办扩展服务
// @param documents 业务表单状态服务
// @return 发起域服务
func NewStartService(engine api.Engine, meta *ModelMetaService, next *NextActivityService, comments *CommentService, operations *OperationRecordService, messages *MessageExtendService, histories *HistoryExtendService, documents *DocumentService) *StartService {
	return &StartService{
		engine:     engine,
		meta:       meta,
		next:       next,
		comments:   comments,
		operations: operations,
		messages:   messages,
		histories:  histories,
		documents:  documents,
	}
}

// Start 发起流程实例并落位首任务全部扩展数据（对位 Java publishAndGetFirstTaskByKey）。
// 步骤：五参校验 → 部署校验（租户+Key）→ 协议变量注入 → 引擎发起 → 首任务定位 →
// 首步执行者解析与预生成（used=1+taskId）→ TJ 意见/操作记录/待办/已办镜像/表单状态。
// @param ctx 上下文（租户信息由 vo.TenantID 注入引擎上下文，引擎多租户按上下文隔离）
// @param vo 发起请求（必填 ProcessDefinitionKey/Title/Operator/TenantID/BusinessKey）
// @return 发起结果（实例/定义/首任务三元组）
// @return 错误（参数缺失、流程未部署、发起失败、步骤未设置执行者等）
func (s *StartService) Start(ctx context.Context, vo *StartProcessInstanceVo) (StartResult, error) {
	// 五参非空校验（对位 Java 逐参 BusinessException）
	if vo == nil {
		return StartResult{}, apiError(errCodeStartParam, "发起请求不能为空")
	}
	if vo.ProcessDefinitionKey == "" {
		return StartResult{}, apiError(errCodeStartParam, "参数 processDefinitionKey 为空")
	}
	if vo.Title == "" {
		return StartResult{}, apiError(errCodeStartParam, "参数 title 为空")
	}
	if vo.Operator == "" {
		return StartResult{}, apiError(errCodeStartParam, "参数 operator 为空")
	}
	if vo.TenantID == "" {
		return StartResult{}, apiError(errCodeStartParam, "参数 tenantId 为空")
	}
	if vo.BusinessKey == "" {
		return StartResult{}, apiError(errCodeStartParam, "参数 businessKey 为空")
	}
	// 引擎多租户语义：定义/任务/实例的可见性按上下文租户隔离，整个发起链路共用注入租户的上下文
	startCtx := tenant.With(ctx, vo.TenantID)
	// 部署校验：按租户+Key 定位最新定义，同时留存定义供待办/表单分类列使用
	definition, err := s.IsProcessDeployed(startCtx, vo.TenantID, vo.ProcessDefinitionKey)
	if err != nil {
		return StartResult{}, err
	}
	// 协议变量注入（Java 语义：vo.Variables 先放入，协议键覆盖同名业务变量）
	variables := startVariablesOf(vo)
	// 引擎发起：名称=标题（实例名）、businessKey、租户、发起人参与者
	instance, err := s.engine.RuntimeService().StartProcessInstance(startCtx, api.StartRequest{
		ProcessDefinitionKey: vo.ProcessDefinitionKey,
		BusinessKey:          vo.BusinessKey,
		Name:                 vo.Title,
		TenantID:             vo.TenantID,
		UserID:               vo.Operator,
		Variables:            variableListOf(variables),
	})
	if err != nil {
		return StartResult{}, err
	}
	// 发起后的落位步骤任一失败即整体报错阻断，并补偿回滚已产生的运行态数据
	//（近似 Java 同事务回滚：删实例 + 清预生成/待办/已办；操作记录与表单状态残留可接受）
	result, finishErr := s.finishStart(startCtx, vo, instance, variables, definition)
	if finishErr != nil {
		s.compensateStart(startCtx, vo, instance, finishErr)
		return StartResult{}, finishErr
	}
	return result, nil
}

// finishStart 执行发起后的全部落位步骤（首任务定位/预生成/意见/操作记录/待办/镜像/表单）。
// @param ctx 操作上下文（已注入租户）
// @param vo 发起请求
// @param instance 已创建的流程实例
// @param variables 发起变量
// @param definition 流程定义
// @return 发起结果
// @return 错误
func (s *StartService) finishStart(ctx context.Context, vo *StartProcessInstanceVo, instance api.ProcessInstance, variables map[string]any, definition api.ProcessDefinition) (StartResult, error) {
	// 首任务定位：发起后第一个用户任务（对位 Java taskQuery().processInstanceId().active().singleResult()）
	task, err := s.firstTaskOf(ctx, instance.ID)
	if err != nil {
		return StartResult{}, err
	}
	// 首步执行者解析与预生成（对位 saveFirstTaskUserNextActivity：used=1+taskId）
	identities, _, err := s.saveFirstTaskUser(ctx, vo, task, variables)
	if err != nil {
		return StartResult{}, err
	}
	// 提交意见（TJ）：Java 允许空意见落库，引擎侧统一拒绝空白评论，备注为空时跳过意见写入
	if strings.TrimSpace(vo.Remark) != "" {
		if _, err := s.comments.Add(ctx, CommentRequest{
			TaskID:            task.ID,
			ProcessInstanceID: instance.ID,
			UserID:            vo.Operator,
			Message:           vo.Remark,
			Type:              CommentTypeTJ,
		}); err != nil {
			return StartResult{}, err
		}
	}
	// 操作记录（TJ 留痕，任务步骤/定义/租户由服务经引擎补全）
	if _, err := s.operations.Save(ctx, OperationRequest{
		TaskID:            task.ID,
		ProcessInstanceID: instance.ID,
		OwnerID:           vo.Operator,
		Message:           vo.Remark,
		Type:              CommentTypeTJ,
	}); err != nil {
		return StartResult{}, err
	}
	// 待办扩展：owner=首任务实际执行人（发起后办理人已按模型/变量解析）
	message, err := s.createFirstTodo(ctx, vo, instance, task, definition, identities)
	if err != nil {
		return StartResult{}, err
	}
	// 已办扩展镜像（Java BeanUtils.copyProperties 整行复制语义）
	if _, err := s.histories.CreateForTask(ctx, &HistoryExtend{MessageExtend: *message}); err != nil {
		return StartResult{}, err
	}
	// 业务表单状态落位（Draft 步骤提交记流程开始时间的语义由 DocumentService.Save 实现）
	if _, err := s.documents.Save(ctx, DocumentSaveRequest{
		Document: startDocumentOf(ctx, vo, instance, definition, variables, s),
		TaskKey:  task.TaskDefinitionKey,
	}); err != nil {
		return StartResult{}, err
	}
	return StartResult{
		ProcessInstanceID:   instance.ID,
		ProcessDefinitionID: instance.ProcessDefinitionID,
		TaskID:              task.ID,
		TaskName:            task.Name,
		TaskKey:             task.TaskDefinitionKey,
	}, nil
}

// compensateStart 补偿回滚发起后的运行态数据（对位 Java 同事务回滚的最小等价形态）。
// 覆盖：引擎实例删除（级联任务）+ 预生成台账 + 待办/已办扩展；操作记录无删除端口、
// 表单状态可能承接既有业务数据，均不回滚（残留注释说明）。
// @param ctx 操作上下文
// @param vo 发起请求
// @param instance 已创建的流程实例
// @param cause 触发补偿的错误
func (s *StartService) compensateStart(ctx context.Context, vo *StartProcessInstanceVo, instance api.ProcessInstance, cause error) {
	// 删除引擎实例（级联运行态任务与执行树）
	_ = s.engine.RuntimeService().DeleteProcessInstance(ctx, instance.ID, "发起失败回滚: "+cause.Error())
	// 清理预生成台账运行态记录
	_ = s.next.DeleteByProcessInstance(ctx, instance.ID)
	// 清理待办与已办扩展镜像
	_ = s.messages.DeleteByInstance(ctx, instance.ID)
	_ = s.histories.DeleteByInstance(ctx, instance.ID)
}

// IsProcessDeployed 判断流程定义是否已部署（按租户+Key 定位最新定义）。
// 引擎多租户语义：GetProcessDefinitionByKey 按上下文租户解析最新版本，故先注入租户上下文。
// @param ctx 操作上下文
// @param tenantID 租户（业务系统代号）
// @param processDefinitionKey 流程定义 Key
// @return 最新流程定义（已部署时）
// @return 错误（未部署返回 WORKFLOW-1651）
func (s *StartService) IsProcessDeployed(ctx context.Context, tenantID, processDefinitionKey string) (api.ProcessDefinition, error) {
	definition, err := s.engine.RepositoryService().GetProcessDefinitionByKey(tenant.With(ctx, tenantID), processDefinitionKey)
	// 引擎未命中返回 ENGINE-0404：对外统一翻译为"流程未部署"领域错误
	if err != nil {
		return api.ProcessDefinition{}, apiError(errCodeStartNotDeployed, "【"+processDefinitionKey+"】流程尚未部署，请检查后重试。")
	}
	return definition, nil
}

// firstTaskOf 定位发起后的首个用户任务。
// 引擎 ListTasks 按创建时间升序返回，首个即首任务（对位 Java singleResult 的 active 语义）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 首个任务
// @return 错误（无任务返回 WORKFLOW-1652）
func (s *StartService) firstTaskOf(ctx context.Context, processInstanceID string) (api.Task, error) {
	page, err := s.engine.TaskService().ListTasks(ctx, api.TaskFilter{ProcessInstanceID: processInstanceID, Size: 1})
	if err != nil {
		return api.Task{}, err
	}
	// 发起后必然存在首个用户任务；查不到说明定义不含用户任务（Java 同语义阻断）
	if page.Total == 0 || len(page.Data) == 0 {
		return api.Task{}, apiError(errCodeStartNoTask, "发起后未找到首个待办任务，请检查流程定义")
	}
	return page.Data[0], nil
}

// startVariablesOf 构造发起变量：vo.Variables 透传（含 entity/tableName）+ 协议键覆盖。
// 对位 Java：remark/initiator/businessKey/title/processDefinitionKey 五个协议键最后 put。
// @param vo 发起请求
// @return 变量映射
func startVariablesOf(vo *StartProcessInstanceVo) map[string]any {
	variables := make(map[string]any, len(vo.Variables)+5)
	// 业务变量先放入，协议键随后覆盖同名键（Java HashMap put 顺序语义）
	for name, value := range vo.Variables {
		variables[name] = value
	}
	variables[VarRemark] = vo.Remark
	// BPMN 中 flowable:assignee="${initiator}" 即发起人节点，依赖该变量
	variables[VarInitiator] = vo.Operator
	variables[VarBusinessKey] = vo.BusinessKey
	variables[VarTitle] = vo.Title
	variables[VarProcessDefinitionKey] = vo.ProcessDefinitionKey
	return variables
}

// variableListOf 将变量映射转为引擎变量列表。
// @param variables 变量映射
// @return 引擎变量列表
func variableListOf(variables map[string]any) []api.Variable {
	out := make([]api.Variable, 0, len(variables))
	for name, value := range variables {
		out = append(out, api.Variable{Name: name, Value: value})
	}
	return out
}

// startDocumentOf 构造发起时的表单状态初始数据。
// @param ctx 操作上下文
// @param vo 发起请求
// @param instance 流程实例
// @param definition 流程定义
// @param variables 发起变量（tableName 提取业务表名）
// @param s 发起域服务（创建人姓名补全）
// @return 表单状态实体
func startDocumentOf(ctx context.Context, vo *StartProcessInstanceVo, instance api.ProcessInstance, definition api.ProcessDefinition, variables map[string]any, s *StartService) *Document {
	tableName := ""
	// 业务表名由发起变量 tableName 透传（document.document_table）
	if raw, ok := variables[VarTableName].(string); ok {
		tableName = raw
	}
	return &Document{
		BaseColumns:    BaseColumns{Creator: vo.Operator, CreatorName: s.userDisplayName(ctx, vo.Operator), TenantID: vo.TenantID},
		BusinessID:     vo.BusinessKey,
		DocumentID:     vo.BusinessKey,
		DocumentTable:  tableName,
		FlowTitle:      vo.Title,
		FlowInstanceID: instance.ID,
		FlowCategoryID: definition.Key,
		FlowCategory:   definition.Name,
	}
}

// saveFirstTaskUser 解析首步执行者并预生成首步记录（对位 saveFirstTaskUserNextActivity:1142）。
// 解析优先级：executorList（模型扩展属性 → vo.Variables 透传）→ assignee 单人 →
// candidateUsers 多人 → candidateGroups 经引擎身份库展开组内用户。
// @param ctx 操作上下文
// @param vo 发起请求
// @param task 首个任务
// @param variables 发起变量（表达式解析用）
// @return 执行人 Id 列表
// @return 执行人候选组 Id 列表（与执行人等长配对）
// @return 错误（步骤未设置执行者/执行人身份不存在）
func (s *StartService) saveFirstTaskUser(ctx context.Context, vo *StartProcessInstanceVo, task api.Task, variables map[string]any) ([]string, []string, error) {
	// 模型元数据以实际创建的任务为准（Java getUserTaskJsonInfo(taskDefinitionKey, processDefinitionId)）
	meta, err := s.meta.UserTaskMeta(ctx, task.ProcessDefinitionID, task.TaskDefinitionKey)
	if err != nil {
		return nil, nil, err
	}
	identities, groupIDs, err := s.resolveFirstStepExecutors(ctx, meta, vo, variables, task.TaskDefinitionKey)
	if err != nil {
		return nil, nil, err
	}
	// 发送人姓名补全（Java accountService 取账号姓名；引擎身份库查不到给空）
	sendUserName := s.userDisplayName(ctx, vo.Operator)
	// 首步预生成：used=1、taskId=首任务、版本号=已消耗最大版本+1（服务内推算）
	err = s.next.SaveFirstTaskUser(ctx, FirstStepRequest{
		ProcessInstanceID: task.ProcessInstanceID,
		ActivityID:        task.TaskDefinitionKey,
		IdentityIDs:       identities,
		GroupIDs:          groupIDs,
		SendIdentityID:    vo.Operator,
		SendUserName:      sendUserName,
		TaskID:            task.ID,
		TenantID:          vo.TenantID,
		Creator:           vo.Operator,
	})
	if err != nil {
		return nil, nil, err
	}
	return identities, groupIDs, nil
}

// resolveFirstStepExecutors 解析首步执行者（对位 Java assignee→candidateUsers→candidateGroups
// 的 if/else 优先级链，并新增 executorList 优先级最高）。
// @param ctx 操作上下文
// @param meta 首任务模型元数据
// @param vo 发起请求
// @param variables 发起变量
// @param activityID 节点 ID（报错信息用）
// @return 执行人 Id 列表
// @return 候选组 Id 列表（与执行人等长配对，无组为空）
// @return 错误
func (s *StartService) resolveFirstStepExecutors(ctx context.Context, meta *UserTaskMeta, vo *StartProcessInstanceVo, variables map[string]any, activityID string) ([]string, []string, error) {
	// 优先级 0：executorList（模型扩展属性优先，其次 vo.Variables 透传的自定义执行人集合）
	configs := ParseExecutorConfigs(meta.ExecutorListRaw)
	if len(configs) == 0 {
		configs = executorConfigsFromVariable(vo.Variables[VarExecutorList])
	}
	if len(configs) > 0 {
		identities, groupIDs := s.expandExecutorConfigs(ctx, configs)
		if len(identities) > 0 {
			return identities, groupIDs, nil
		}
	}
	// 优先级 1：assignee 单人（${initiator} 解析为发起人，对位 Java VARIABLES_KEY_INITIATOR 分支）
	if assignee := strings.TrimSpace(meta.Assignee); assignee != "" {
		if id := resolveExecutorExpression(assignee, variables); id != "" {
			return []string{id}, nil, nil
		}
	}
	// 优先级 2：candidateUsers 多人（逗号拆分由模型解析器完成，此处解析表达式后逐人收集）
	if len(meta.CandidateUsers) > 0 {
		identities := make([]string, 0, len(meta.CandidateUsers))
		for _, raw := range meta.CandidateUsers {
			// 表达式逐项解析：${变量} 取变量值，字面量原样作为身份 Id
			if id := resolveExecutorExpression(raw, variables); id != "" {
				identities = append(identities, id)
			}
		}
		if len(identities) > 0 {
			return identities, nil, nil
		}
	}
	// 优先级 3：candidateGroups 组 → 引擎身份库展开组内用户（对位 Java getIdentityIdsByGroupIds）
	if len(meta.CandidateGroups) > 0 {
		identities, groupIDs := s.expandGroups(ctx, meta.CandidateGroups)
		// 与 Java 一致：组内无有效成员时报"执行人身份不存在"
		if len(identities) == 0 {
			return nil, nil, apiError(errCodeStartNoIdentity, "保存第一个用户任务下一步骤信息异常：执行人身份不存在")
		}
		return identities, groupIDs, nil
	}
	// 全空：按 Java 语义报错阻断（【taskKey】步骤未设置执行者）
	return nil, nil, apiError(errCodeStartNoExecutor, "【"+activityID+"】步骤未设置执行者")
}

// expandExecutorConfigs 展开 executorList 配置为执行人（person 取身份；org/role 按组展开成员）。
// 展开为空的组不阻断其余配置（整体空判由调用方兜底），故无错误返回。
// @param ctx 操作上下文
// @param configs 执行者配置列表
// @return 执行人 Id 列表
// @return 候选组 Id 列表（与执行人等长配对）
func (s *StartService) expandExecutorConfigs(ctx context.Context, configs []ExecutorConfig) ([]string, []string) {
	identities := make([]string, 0, len(configs))
	groupIDs := make([]string, 0, len(configs))
	for _, config := range configs {
		switch config.ExecutorType {
		// org/role 视为组维度：经引擎身份库展开组内用户（角色在引擎身份库即组语义）
		case "org", "role":
			groupID := firstNonEmpty(config.OrgID, config.RoleID)
			// 组 Id 缺失的配置项跳过
			if groupID == "" {
				continue
			}
			members, err := s.engine.IdentityService().ListMembers(ctx, groupID)
			// 组查不到时不阻断其余配置（展开为空的组由整体空判兜底）
			if err != nil {
				continue
			}
			for _, member := range members {
				identities = append(identities, member.ID)
				groupIDs = append(groupIDs, groupID)
			}
		default:
			// person（含缺省）：直接取身份 Id
			if config.IdentityID != "" {
				identities = append(identities, config.IdentityID)
				groupIDs = append(groupIDs, "")
			}
		}
	}
	return identities, groupIDs
}

// expandGroups 展开候选组为组内用户列表（组员与所属组等长配对）。
// @param ctx 操作上下文
// @param groupIDs 候选组 Id 列表
// @return 执行人 Id 列表
// @return 与执行人等长的候选组 Id 列表
func (s *StartService) expandGroups(ctx context.Context, groupIDs []string) ([]string, []string) {
	identities := make([]string, 0, len(groupIDs))
	paired := make([]string, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		// 引擎身份库按组查成员（api.IdentityService.ListMembers 存在，无需组 ID 原样记录的降级路径）
		members, err := s.engine.IdentityService().ListMembers(ctx, groupID)
		if err != nil {
			continue
		}
		for _, member := range members {
			identities = append(identities, member.ID)
			paired = append(paired, groupID)
		}
	}
	return identities, paired
}

// resolveExecutorExpression 解析执行者表达式：${initiator}/initiator 解析为变量值，其余原样返回。
// @param raw 原始表达式
// @param variables 变量映射
// @return 身份 Id（无法解析返回空串）
func resolveExecutorExpression(raw string, variables map[string]any) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	// ${xxx} 形态剥壳后查变量
	if strings.HasPrefix(trimmed, "${") && strings.HasSuffix(trimmed, "}") {
		name := strings.TrimSuffix(strings.TrimPrefix(trimmed, "${"), "}")
		if value, ok := variables[name]; ok {
			return strings.TrimSpace(valueString(value))
		}
		return ""
	}
	// 裸变量名（Java VARIABLES_KEY_INITIATOR 分支等价）命中变量时取变量值
	if value, ok := variables[trimmed]; ok {
		return strings.TrimSpace(valueString(value))
	}
	return trimmed
}

// executorConfigsFromVariable 将 vo.Variables 中的 executorList 变量归一为执行者配置。
// 支持 []string、[]any（字符串元素）、逗号分隔字符串三种形态。
// @param raw 变量原始值
// @return 执行者配置列表
func executorConfigsFromVariable(raw any) []ExecutorConfig {
	switch value := raw.(type) {
	case []string:
		configs := make([]ExecutorConfig, 0, len(value))
		for _, id := range value {
			// 字符串切片按 person 逐项收集
			if id = strings.TrimSpace(id); id != "" {
				configs = append(configs, ExecutorConfig{IdentityID: id, ExecutorType: "person"})
			}
		}
		return configs
	case []any:
		configs := make([]ExecutorConfig, 0, len(value))
		for _, item := range value {
			// 混合切片：字符串元素按 person；映射元素按 identityId 字段提取
			switch typed := item.(type) {
			case string:
				if typed = strings.TrimSpace(typed); typed != "" {
					configs = append(configs, ExecutorConfig{IdentityID: typed, ExecutorType: "person"})
				}
			case map[string]any:
				if id, _ := typed["identityId"].(string); strings.TrimSpace(id) != "" {
					configs = append(configs, ExecutorConfig{IdentityID: strings.TrimSpace(id), ExecutorType: "person"})
				}
			}
		}
		return configs
	case string:
		// 逗号分隔串复用统一解析（JSON 形态亦兼容）
		return ParseExecutorConfigs(value)
	default:
		return nil
	}
}

// userDisplayName 查询用户展示名（查不到给空，不阻断主流程）。
// @param ctx 操作上下文
// @param userID 用户 Id
// @return 展示名
func (s *StartService) userDisplayName(ctx context.Context, userID string) string {
	user, err := s.engine.IdentityService().GetUser(ctx, userID)
	if err != nil {
		return ""
	}
	return userDisplayName(user)
}

// valueString 将变量值转为字符串（字符串直接返回，其余经通用格式化）。
// @param value 变量值
// @return 字符串值
func valueString(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

// createFirstTodo 创建首任务待办扩展（待办中心数据源）。
// owner=首任务实际执行人：办理人优先，缺省回退首步解析执行人首位。
// @param ctx 操作上下文
// @param vo 发起请求
// @param instance 流程实例
// @param task 首个任务
// @param definition 流程定义
// @param identities 首步解析执行人（owner 回退用）
// @return 落库后的待办扩展
// @return 错误
func (s *StartService) createFirstTodo(ctx context.Context, vo *StartProcessInstanceVo, instance api.ProcessInstance, task api.Task, definition api.ProcessDefinition, identities []string) (*MessageExtend, error) {
	// owner 解析：办理人优先，无办理人（纯候选任务）回退解析执行人首位
	ownerID := firstNonEmpty(task.Assignee, firstOf(identities))
	return s.messages.CreateForTask(ctx, &MessageExtend{
		BaseColumns:       BaseColumns{TenantID: vo.TenantID, Creator: vo.Operator},
		TaskID:            task.ID,
		ProcessInstanceID: instance.ID,
		Title:             vo.Title,
		Message:           vo.Remark,
		BusinessID:        vo.BusinessKey,
		TaskStepName:      task.Name,
		TaskStepCode:      task.TaskDefinitionKey,
		FlowCategoryCode:  definition.Key,
		FlowCategoryName:  definition.Name,
		OwnerID:           ownerID,
		OwnerName:         s.userDisplayName(ctx, ownerID),
		DocumentOwnerID:   vo.Operator,
		DocumentOwnerName: s.userDisplayName(ctx, vo.Operator),
		SenderID:          vo.Operator,
		Sender:            s.userDisplayName(ctx, vo.Operator),
	})
}

// firstOf 返回列表首个元素（空列表返回空串）。
// @param values 值列表
// @return 首个元素
func firstOf(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
