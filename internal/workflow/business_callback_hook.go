package workflow

import (
	"context"
	"sync"

	"github.com/DengQingNian/Flowtide/api"
)

// 业务回调在 BPMN 节点上的扩展属性键（对位调研 006 §3 报文约定）。
const (
	// AttrBusinessCallback 节点级业务回调开关属性（值为 "true" 时触发回调）。
	AttrBusinessCallback = "businessCallback"
	// AttrBusinessCallbackParams 回调报文 params 字段的原始扩展属性串（"k:v;k:v"）。
	AttrBusinessCallbackParams = "businessCallbackParams"
)

// BusinessCallbackHook 是任务创建时的业务回调全局钩子（TASK_CREATED）。
// 016 号审计：CallbackClient 此前实现完整但生产代码零调用点，文档 007:53 承诺
// "P1.2 在 TASK_CREATED 等钩子内组包并调用"从未落地，D5 回调能力实际不生效。
//
// 实现约束：钩子在引擎命令内同步执行且持有引擎互斥写锁，HTTP 回调绝不允许在锁内发起，
// 因此组包在钩子内完成、实际发送经 ctl.Defer 延迟到命令提交并释放锁后执行。
// 回调失败只记录日志与异常表（经 CallbackClient 的 ExceptionRecorder），不影响流程推进，
// 与 Java isFailOnException 语义一致。
type BusinessCallbackHook struct {
	mu     sync.RWMutex                        // 保护元数据缓存
	cache  map[string]map[string]*UserTaskMeta // 定义 ID → 节点 ID → 元数据（命令内只读）
	client *CallbackClient                     // 业务回调 client，nil 时不发回调
	meta   *ModelMetaService                   // 模型元数据（回调属性读取）
}

// NewBusinessCallbackHook 创建业务回调钩子。
// @param meta 模型元数据服务
// @param client 业务回调 client（nil 时钩子为空操作）
// @return 回调钩子
func NewBusinessCallbackHook(meta *ModelMetaService, client *CallbackClient) *BusinessCallbackHook {
	return &BusinessCallbackHook{
		cache:  make(map[string]map[string]*UserTaskMeta),
		client: client,
		meta:   meta,
	}
}

// Register 将业务回调钩子注册到引擎任务创建事件（TASK_CREATED）。
// 与执行人重写钩子同为全局任务钩子：注册后所有用户任务创建命令都会同步执行 Hook。
// client 未装配（未接入业务系统注册存储）时为空操作。
// @param eng 引擎门面
func (h *BusinessCallbackHook) Register(eng api.Engine) {
	if h == nil || h.client == nil || eng == nil {
		return
	}
	eng.AddTaskCreatedHook(h.Hook)
}

// Warmup 预热指定定义的节点元数据缓存（部署后调用，锁外）。
// @param ctx 操作上下文
// @param definitionID 流程定义 ID
// @return 预热错误
func (h *BusinessCallbackHook) Warmup(ctx context.Context, definitionID string) error {
	if h.meta == nil {
		return nil
	}
	metas, err := h.meta.ListUserTaskMetas(ctx, definitionID)
	if err != nil {
		return err
	}
	index := make(map[string]*UserTaskMeta, len(metas))
	for _, meta := range metas {
		index[meta.ActivityID] = meta
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cache[definitionID] = index
	return nil
}

// WarmupAll 预热全部已部署定义的节点元数据缓存（启动期调用，锁外）。
// @param engine 引擎门面
// @return 预热错误
// @param ctx 参数 ctx
func (h *BusinessCallbackHook) WarmupAll(ctx context.Context, engine api.Engine) error {
	if engine == nil {
		return nil
	}
	definitions, err := engine.RepositoryService().ListProcessDefinitions(ctx)
	if err != nil {
		return err
	}
	for _, definition := range definitions {
		// 单定义预热失败不阻断其余定义（部署损坏等场景）
		if err := h.Warmup(ctx, definition.ID); err != nil {
			continue
		}
	}
	return nil
}

// Hook 业务回调钩子主体（api.TaskHook 签名）。
// 节点未声明回调属性时直接返回；声明时组包并经 ctl.Defer 延迟发送。
// @param ctx 命令上下文（与所在引擎命令共用，命令失败整体回滚且不执行延迟动作）
// @param task 任务快照
// @param ctl 钩子控制
// @return 错误（始终返回 nil：回调失败不影响流程推进）
func (h *BusinessCallbackHook) Hook(ctx context.Context, task *api.Task, ctl api.HookControl) error {
	// client 未装配（未注册业务系统回调）或快照缺失时不做任何事
	if h.client == nil || task == nil || ctl == nil {
		return nil
	}
	meta := h.cachedMeta(task.ProcessDefinitionID, task.TaskDefinitionKey)
	if meta == nil {
		// 未预热：登记延迟预热，本次不发送（与执行人重写钩子同一降级策略）
		ctl.Defer(func(deferCtx context.Context) error { return h.Warmup(deferCtx, task.ProcessDefinitionID) })
		return nil
	}
	if !meta.BusinessCallbackEnabled() {
		return nil
	}
	callback := &TaskCallback{
		ProcessDefinitionID: task.ProcessDefinitionID,
		ProcessInstanceID:   task.ProcessInstanceID,
		BusinessKey:         valueString(task.Variables[VarBusinessKey]),
		TenantID:            task.TenantID,
		TaskID:              task.ID,
		TaskName:            task.Name,
		FlowElementID:       task.TaskDefinitionKey,
		FlowElementName:     task.Name,
		Params:              meta.BusinessCallbackParams(),
	}
	// 锁内只组包：HTTP 发送经 Defer 延迟到命令提交并释放锁之后
	ctl.Defer(func(deferCtx context.Context) error {
		return h.client.SendTaskCallback(deferCtx, callback)
	})
	return nil
}

// cachedMeta 命令内只读查询预热的节点元数据。
// @param definitionID 流程定义 ID
// @param activityID 节点 ID
// @return 元数据，未预热返回 nil
func (h *BusinessCallbackHook) cachedMeta(definitionID, activityID string) *UserTaskMeta {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if index, ok := h.cache[definitionID]; ok {
		return index[activityID]
	}
	return nil
}
