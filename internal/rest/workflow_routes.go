package rest

import (
	"net/http"

	"github.com/DengQingNian/Flowtide/api"
)

// workflowRoute 是 /workflow/** 路由定义。
type workflowRoute struct {
	method string // HTTP 方法，空串表示任意方法（对应 Java @RequestMapping ANY）
	path   string // 完整路径（D8：Java 兼容形态 /workflow/{模块}/{方法}）
}

// workflowRouteTable 是 workflow 门面 REST 路由清单。
// 来源：005 接口清单 §1 中"非桩"端点 + D8 保留拼写决策 + D9/D10/D11 内置能力路径。
// 已排除：Java 桩端点（/task/getNextUserTaskAndUserList、/task/complete/task、
// /task/getBackNodesByProcessInstanceId、/task/reject|delegate|end|reset 等）、
// 明确裁剪模块（/rule、/model/discard、遗留四表 task/history/record/example、旧版 /consigner）。
// 每条路由由 workflowHandlerFor 绑定业务 handler（接 workflow.Service 真实现或 501 桩）。
var workflowRouteTable = []workflowRoute{
	// ── /process/**：FlowableController（核心门面，005 §1.1）──
	{method: http.MethodPost, path: "/workflow/process/add/model"},
	{method: http.MethodGet, path: "/workflow/process/getModelJsonByType"},
	{method: http.MethodGet, path: "/workflow/process/getHisModelJson"},
	{method: http.MethodPost, path: "/workflow/process/save/xml"},
	{method: http.MethodPost, path: "/workflow/process/updateHisModelXml"},
	{method: http.MethodGet, path: "/workflow/process/export/xml"},
	{method: http.MethodPost, path: "/workflow/process/import/xml"},
	{method: http.MethodGet, path: "/workflow/process/exportProcess"},
	{method: http.MethodPost, path: "/workflow/process/importProcess"},
	{method: http.MethodPost, path: "/workflow/process/copyProcess"},
	{method: http.MethodGet, path: "/workflow/process/model/list"},
	{method: "", path: "/workflow/process/index"},
	{method: "", path: "/workflow/process/create"},
	// D8：保留 Java 兼容拼写 deplay，同时提供 deploy 规范别名
	{method: "", path: "/workflow/process/deplay"},
	{method: "", path: "/workflow/process/deploy"},
	{method: "", path: "/workflow/process/model/delete/{id}"},
	{method: http.MethodPost, path: "/workflow/process/public/model/id"},
	{method: http.MethodPost, path: "/workflow/process/public/deploymentId"},
	{method: http.MethodPost, path: "/workflow/process/public/processes/id"},
	{method: http.MethodPost, path: "/workflow/process/suspend/processes"},
	{method: http.MethodPost, path: "/workflow/process/activate/processes"},
	{method: http.MethodPost, path: "/workflow/process/suspend/processInstance/{pid}"},
	{method: http.MethodPost, path: "/workflow/process/activate/processInstance/{pid}"},
	{method: http.MethodPost, path: "/workflow/process/stop/processInstance/{pid}"},
	{method: "", path: "/workflow/process/instance/all/delete/{id}"},
	{method: http.MethodDelete, path: "/workflow/process/delete/process"},
	{method: http.MethodDelete, path: "/workflow/process/back/process"},
	{method: http.MethodDelete, path: "/workflow/process/end/process"},
	{method: http.MethodDelete, path: "/workflow/process/highlight/img"},
	{method: http.MethodDelete, path: "/workflow/process/normal/img"},
	{method: http.MethodPost, path: "/workflow/process/apply/task"},
	{method: http.MethodPost, path: "/workflow/process/complete/task"},
	{method: http.MethodGet, path: "/workflow/process/instance/list"},
	{method: http.MethodGet, path: "/workflow/process/instance/all/list"},
	{method: http.MethodGet, path: "/workflow/process/task/list"},
	{method: http.MethodGet, path: "/workflow/process/task/all/todoList"},
	{method: http.MethodGet, path: "/workflow/process/task/all/doneList"},
	{method: http.MethodGet, path: "/workflow/process/history/step/list/{taskId}"},
	{method: http.MethodGet, path: "/workflow/process/history/task/list"},
	{method: http.MethodPost, path: "/workflow/process/delegate/task"},
	{method: http.MethodPost, path: "/workflow/process/transfer/task"},
	{method: http.MethodPost, path: "/workflow/process/resolve/task"},
	{method: http.MethodPost, path: "/workflow/process/inject/node"},
	{method: http.MethodPost, path: "/workflow/process/complete/sign"},
	{method: http.MethodPost, path: "/workflow/process/claim/task"},
	{method: http.MethodPost, path: "/workflow/process/add/execution"},
	{method: http.MethodDelete, path: "/workflow/process/del/execution"},
	// Java 原拼写 exmple（D8 兼容保留）
	{method: http.MethodPost, path: "/workflow/process/exmple/node/list"},
	{method: http.MethodPost, path: "/workflow/process/wait/node/list"},

	// ── /model/**：WorkflowModelController（D11 模型页）──
	{method: http.MethodPost, path: "/workflow/model/add"},
	{method: http.MethodPost, path: "/workflow/model/edit"},
	{method: "", path: "/workflow/model/delete/{ids}"},
	{method: http.MethodGet, path: "/workflow/model/list"},
	{method: http.MethodGet, path: "/workflow/model/getUserTaskJsonInfo"},
	{method: http.MethodGet, path: "/workflow/model/getProcessDefinitionJsonInfo"},
	{method: http.MethodGet, path: "/workflow/model/findModelAndStepPageList"},
	{method: http.MethodGet, path: "/workflow/model/tree/list"},

	// ── /category/**：流程分类页（D11：workflow_model 一表两页）──
	{method: http.MethodPost, path: "/workflow/category/add"},
	{method: http.MethodPost, path: "/workflow/category/edit"},
	{method: "", path: "/workflow/category/delete/{ids}"},
	{method: http.MethodGet, path: "/workflow/category/list"},

	// ── /step/**：WorkflowStepController ──
	{method: http.MethodPost, path: "/workflow/step/add"},
	{method: http.MethodPost, path: "/workflow/step/edit"},
	{method: "", path: "/workflow/step/delete/{ids}"},
	{method: http.MethodGet, path: "/workflow/step/list"},
	{method: http.MethodGet, path: "/workflow/step/findById"},

	// ── /hisstep/**：WorkflowHisStepController（list 空时创建衍生流程定义）──
	{method: http.MethodPost, path: "/workflow/hisstep/add"},
	{method: http.MethodPost, path: "/workflow/hisstep/edit"},
	{method: "", path: "/workflow/hisstep/delete/{ids}"},
	{method: http.MethodGet, path: "/workflow/hisstep/list"},

	// ── /template/**：WorkflowTemplateController ──
	{method: http.MethodPost, path: "/workflow/template/add"},
	{method: http.MethodPost, path: "/workflow/template/edit"},
	{method: "", path: "/workflow/template/delete/{ids}"},
	{method: http.MethodGet, path: "/workflow/template/list"},

	// ── /extend/**：WorkflowActExtendController（活动扩展属性）──
	{method: http.MethodPost, path: "/workflow/extend/save"},
	{method: http.MethodGet, path: "/workflow/extend/getActByProcessDefKeyAndActId"},

	// ── /weight/**：WorkflowCountersignWeightController（会签权重）──
	{method: http.MethodPost, path: "/workflow/weight/save"},
	{method: http.MethodPost, path: "/workflow/weight/delete"},
	{method: http.MethodGet, path: "/workflow/weight/getWeightByProcessDefKeyAndActId"},

	// ── /delegate/**：WorkflowTaskDelegateController（新版委托）──
	{method: http.MethodPost, path: "/workflow/delegate/add"},
	{method: http.MethodPost, path: "/workflow/delegate/edit"},
	{method: "", path: "/workflow/delegate/delete/{ids}"},
	{method: http.MethodGet, path: "/workflow/delegate/list"},

	// ── /task/**：WorkflowTaskController 中非桩且非遗留表端点 ──
	{method: http.MethodPost, path: "/workflow/task/rollback"},
	{method: http.MethodPost, path: "/workflow/task/completeToTargetActivity"},
	{method: http.MethodGet, path: "/workflow/task/operationButtonList/{processInstanceId}"},

	// ── /remind/**：WorkflowRemindController（统一返回 enm.R）──
	{method: http.MethodGet, path: "/workflow/remind/getPageList"},
	{method: http.MethodGet, path: "/workflow/remind/getWorkflowRemind/{id}"},
	{method: http.MethodPost, path: "/workflow/remind/add"},
	{method: http.MethodDelete, path: "/workflow/remind/delete/{id}"},
	{method: http.MethodPost, path: "/workflow/remind/edit"},

	// ── /exception/**：WorkflowTaskExceptionController ──
	{method: http.MethodGet, path: "/workflow/exception/list"},
	{method: "", path: "/workflow/exception/delete/{ids}"},

	// ── /dict/**：内置静态字典（D9）──
	{method: http.MethodGet, path: "/workflow/dict/{code}"},

	// ── /identity/**：内置身份库树接口（D10）──
	{method: http.MethodGet, path: "/workflow/identity/tree"},
}

// workflowNotImplementedCode 是阶段 0 空实现/未接线端点的协议状态码与文案。
// 业务失败 R 复用同码（enm.R 协议仅 "200"/"500" 两态）。
const (
	workflowNotImplementedCode = "500"
	workflowNotImplementedMsg  = "接口未实现"
)

// registerWorkflowRoutes 挂载 /workflow/** 全部路由。
// 每条路由经 workflowHandlerFor 绑定独立 handler：已接线端点走 workflow.Service 真实现；
// 未注入 workflow 服务时统一回落 501 桩（guardWorkflow），保留端点恒为 501。
// 复用 Server 中间件链：认证/租户/审计/限流对 /workflow/** 与 /service/** 同策略（006 §6）。
// @param s REST 服务
func registerWorkflowRoutes(s *Server) {
	for _, route := range workflowRouteTable {
		handler := s.workflowHandlerFor(route.path)
		if route.method == "" {
			// 无方法前缀的 pattern 匹配任意 HTTP 方法（对应 Java ANY）
			s.mux.HandleFunc(route.path, handler)
			continue
		}
		s.mux.HandleFunc(route.method+" "+route.path, handler)
	}
}

// workflowHandlerFor 返回路由路径对应的业务 handler。
// 已接线端点经 serveWorkflow 包装（未注入 workflow 服务时回落 501）；
// 保留 501 的端点直接返回桩并注释"待接线原因"。
// @param path 路由模板路径
// @return HTTP 处理器
func (s *Server) workflowHandlerFor(path string) http.HandlerFunc {
	switch path {
	// ── /process/**：已接线端点 ──
	case "/workflow/process/apply/task": // 发起流程并预存首步 → StartProcessAndFirstStep
		return s.serveWorkflow(s.handleWorkflowApplyTask)
	case "/workflow/process/complete/task": // 办结任务 → Complete
		return s.serveWorkflow(s.handleWorkflowCompleteTask)
	case "/workflow/process/back/process": // 退回任务 → BackToStepTask
		return s.serveWorkflow(s.handleWorkflowBackProcess)
	case "/workflow/process/end/process", "/workflow/process/stop/processInstance/{pid}": // 软终止 → DoStopProcessInstance
		return s.serveWorkflow(s.handleWorkflowStopProcess)
	case "/workflow/process/delete/process": // 按业务单据删除流程 → DoDeleteProcessByBusinessID
		return s.serveWorkflow(s.handleWorkflowDeleteByBusiness)
	case "/workflow/process/instance/all/delete/{id}": // 按实例删除流程 → DoDeleteProcessInstance
		return s.serveWorkflow(s.handleWorkflowDeleteInstance)
	case "/workflow/process/suspend/processInstance/{pid}": // 挂起实例 → DoSuspendProcessInstance
		return s.serveWorkflow(s.handleWorkflowSuspendInstance)
	case "/workflow/process/activate/processInstance/{pid}": // 激活实例 → DoActivateProcessInstance
		return s.serveWorkflow(s.handleWorkflowActivateInstance)
	case "/workflow/process/suspend/processes": // 批量挂起实例 → SuspendProcessInstances
		return s.serveWorkflow(s.handleWorkflowBatchSuspend)
	case "/workflow/process/activate/processes": // 批量激活实例 → ActivateProcessInstances
		return s.serveWorkflow(s.handleWorkflowBatchActivate)
	case "/workflow/process/resolve/task": // 委派处理（处理人恢复为所有者）→ ResolveTask
		return s.serveWorkflow(s.handleWorkflowResolveTask)
	case "/workflow/process/claim/task": // 签收任务（组任务认领到个人）→ ClaimTask
		return s.serveWorkflow(s.handleWorkflowClaimTask)
	case "/workflow/process/instance/all/list", "/workflow/process/instance/list": // 运行实例分页 → GetProcessInstancePageList
		return s.serveWorkflow(s.handleWorkflowInstancePageList)
	case "/workflow/process/task/list": // 任务详情（taskId 必填）→ GetTaskInfoByTaskID
		return s.serveWorkflow(s.handleWorkflowTaskList)
	case "/workflow/process/task/all/todoList": // 待办列表 → GetTodoTaskListByIdentityID
		return s.serveWorkflow(s.handleWorkflowTodoList)
	case "/workflow/process/task/all/doneList": // 已办列表 → GetDoneTaskListByIdentityID
		return s.serveWorkflow(s.handleWorkflowDoneList)
	case "/workflow/process/history/step/list/{taskId}": // 按任务查历史 → GetTaskHistoryListByTaskID
		return s.serveWorkflow(s.handleWorkflowHistoryStepList)
	case "/workflow/process/history/task/list": // 按实例/业务键查历史 → 分派
		return s.serveWorkflow(s.handleWorkflowHistoryTaskList)
	case "/workflow/process/transfer/task": // 调整处理人 → DoTaskOwnerAdjustment
		return s.serveWorkflow(s.handleWorkflowTransferTask)
	case "/workflow/process/delegate/task": // 引擎委派（转他人办理，resolve 归还）→ DelegateTask
		return s.serveWorkflow(s.handleWorkflowDelegateTask)
	case "/workflow/process/complete/sign": // 会签比例结论 → CompleteSign
		return s.serveWorkflow(s.handleWorkflowCompleteSign)
	case "/workflow/process/add/execution": // 加签 → AddExecution
		return s.serveWorkflow(s.handleWorkflowAddExecution)
	case "/workflow/process/del/execution": // 减签 → DelExecution
		return s.serveWorkflow(s.handleWorkflowDelExecution)
	case "/workflow/process/deplay", "/workflow/process/deploy": // 部署模型 → DeployModel（D8 保留 deplay 拼写）
		return s.serveWorkflow(s.handleWorkflowDeploy)
	case "/workflow/process/public/model/id": // 按模型发布（部署）→ DeployModel
		return s.serveWorkflow(s.handleWorkflowDeploy)
	case "/workflow/process/model/list": // 模型列表 → ListModelCategory
		return s.serveWorkflow(s.handleWorkflowModelList)
	case "/workflow/process/index": // 流程设计首页数据（模型列表投影）→ ListModelCategory
		return s.serveWorkflow(s.handleWorkflowModelList)
	case "/workflow/process/add/model", "/workflow/process/create": // 设计器建模 → CreateProcessModel（create 为 Java ANY 方法）
		return s.serveWorkflow(s.handleWorkflowCreateModel)
	case "/workflow/process/getModelJsonByType", "/workflow/process/export/xml": // 读模型 XML（type=his 时读历史定义）→ GetModelXML/GetDefinitionXML
		return s.serveWorkflow(s.handleWorkflowGetModelJSON)
	case "/workflow/process/getHisModelJson": // 读历史定义 XML → GetDefinitionXML
		return s.serveWorkflow(s.handleWorkflowGetDefinitionJSON)
	case "/workflow/process/save/xml": // 保存模型 XML 编辑源 → SaveModelXML
		return s.serveWorkflow(s.handleWorkflowSaveModelXML)
	case "/workflow/process/updateHisModelXml": // 历史定义回填模型源 → UpdateModelFromDefinition
		return s.serveWorkflow(s.handleWorkflowUpdateModelFromDefinition)
	case "/workflow/process/copyProcess": // 复制源模型 XML 到目标模型 → CopyProcess
		return s.serveWorkflow(s.handleWorkflowCopyProcess)
	case "/workflow/process/model/delete/{id}": // 删除模型/分类 → DeleteModelCategory
		return s.serveWorkflow(s.handleWorkflowModelDelete)
	case "/workflow/process/highlight/img", "/workflow/process/normal/img": // 流程图高亮 → GetFlowHighlight（DELETE 保留 Java 兼容）
		return s.serveWorkflow(s.handleWorkflowHighlightImg)

	// ── /task/**：已接线端点 ──
	case "/workflow/task/rollback": // 撤回任务 → DoRollbackTask
		return s.serveWorkflow(s.handleWorkflowRollback)
	case "/workflow/task/completeToTargetActivity": // 直送 → CompleteToTargetActivity
		return s.serveWorkflow(s.handleWorkflowCompleteToTargetActivity)
	case "/workflow/task/operationButtonList/{processInstanceId}": // 操作按钮清单 → GetOperationButtons
		return s.serveWorkflow(s.handleWorkflowOperationButtons)

	// ── /extend/**、/weight/**：审阅人绑定与会签权重 ──
	case "/workflow/extend/save": // 保存审阅人绑定（活动扩展）→ SaveActExtend
		return s.serveWorkflow(s.handleWorkflowExtendSave)
	case "/workflow/extend/getActByProcessDefKeyAndActId": // 按定义/节点查审阅绑定 → ListActExtends
		return s.serveWorkflow(s.handleWorkflowExtendList)
	case "/workflow/weight/save": // 保存节点会签权重 → SaveCountersignWeights
		return s.serveWorkflow(s.handleWorkflowWeightSave)
	case "/workflow/weight/getWeightByProcessDefKeyAndActId": // 查询节点会签权重 → ListCountersignWeights
		return s.serveWorkflow(s.handleWorkflowWeightList)
	case "/workflow/weight/delete": // 清空节点会签权重（先删后插空集）→ DeleteCountersignWeights
		return s.serveWorkflow(s.handleWorkflowWeightDelete)

	// ── /identity/**：内置身份库树（D10）──
	case "/workflow/identity/tree": // 用户/组两级身份树 → GetIdentityTree
		return s.serveWorkflow(s.handleWorkflowIdentityTree)

	// ── /model/**、/category/**：模型/分类管理（一表两页，同语义投影）──
	case "/workflow/model/add", "/workflow/category/add": // 新增 → AddModelCategory
		return s.serveWorkflow(s.handleWorkflowModelAdd)
	case "/workflow/model/edit", "/workflow/category/edit": // 更新 → EditModelCategory
		return s.serveWorkflow(s.handleWorkflowModelEdit)
	case "/workflow/model/delete/{ids}", "/workflow/category/delete/{ids}": // 批量删除（逗号分隔）→ DeleteModelCategory
		return s.serveWorkflow(s.handleWorkflowModelDelete)
	case "/workflow/model/list", "/workflow/category/list": // 列表 → ListModelCategory
		return s.serveWorkflow(s.handleWorkflowModelList)
	case "/workflow/model/tree/list": // 分类树 → GetModelCategoryTree
		return s.serveWorkflow(s.handleWorkflowModelTree)
	case "/workflow/model/getUserTaskJsonInfo": // 定义用户任务节点元数据（设计器属性面板）→ GetUserTaskMetas
		return s.serveWorkflow(s.handleWorkflowUserTaskMetas)
	case "/workflow/model/getProcessDefinitionJsonInfo": // 定义 XML（设计器画布）→ GetDefinitionXML
		return s.serveWorkflow(s.handleWorkflowGetDefinitionJSON)
	case "/workflow/model/findModelAndStepPageList": // 模型+设计时步骤联查 → GetModelAndStepPage
		return s.serveWorkflow(s.handleWorkflowModelAndStepPage)

	// ── /step/**：设计时步骤 ──
	case "/workflow/step/add": // 新增 → AddStepDesign
		return s.serveWorkflow(s.handleWorkflowStepAdd)
	case "/workflow/step/edit": // 更新 → EditStepDesign
		return s.serveWorkflow(s.handleWorkflowStepEdit)
	case "/workflow/step/delete/{ids}": // 批量删除 → DeleteStepDesign
		return s.serveWorkflow(s.handleWorkflowStepDelete)
	case "/workflow/step/list": // 按模型查步骤列表 → ListStepDesignByModel
		return s.serveWorkflow(s.handleWorkflowStepList)
	case "/workflow/step/findById": // 步骤主键详情 → GetStepDesign
		return s.serveWorkflow(s.handleWorkflowStepGet)

	// ── /hisstep/**：部署步骤快照（只读）──
	case "/workflow/hisstep/list": // 按部署查快照 → ListHisSteps
		return s.serveWorkflow(s.handleWorkflowHisStepList)

	// ── /template/**：设计器模板 ──
	case "/workflow/template/add": // 新增 → AddTemplateDef
		return s.serveWorkflow(s.handleWorkflowTemplateAdd)
	case "/workflow/template/edit": // 更新 → EditTemplateDef
		return s.serveWorkflow(s.handleWorkflowTemplateEdit)
	case "/workflow/template/delete/{ids}": // 批量删除 → DeleteTemplateDef
		return s.serveWorkflow(s.handleWorkflowTemplateDelete)
	case "/workflow/template/list": // 模板列表 → ListTemplateDefs
		return s.serveWorkflow(s.handleWorkflowTemplateList)

	// ── /delegate/**：委托规则 CRUD ──
	case "/workflow/delegate/add": // 新增 → AddDelegateTask
		return s.serveWorkflow(s.handleWorkflowDelegateAdd)
	case "/workflow/delegate/edit": // 更新 → EditDelegateTask
		return s.serveWorkflow(s.handleWorkflowDelegateEdit)
	case "/workflow/delegate/delete/{ids}": // 删除（单个 Id）→ DeleteDelegateTask
		return s.serveWorkflow(s.handleWorkflowDelegateDelete)
	case "/workflow/delegate/list": // 规则列表（ownerId+tenantId）→ ListDelegateTask
		return s.serveWorkflow(s.handleWorkflowDelegateList)

	// ── /remind/**：催办配置 ──
	case "/workflow/remind/getPageList": // 分页列表 → GetRemindPageList
		return s.serveWorkflow(s.handleWorkflowRemindPageList)
	case "/workflow/remind/getWorkflowRemind/{id}": // 详情 → GetRemind
		return s.serveWorkflow(s.handleWorkflowRemindGet)
	case "/workflow/remind/add": // 新增 → AddRemind
		return s.serveWorkflow(s.handleWorkflowRemindAdd)
	case "/workflow/remind/edit": // 更新 → EditRemind
		return s.serveWorkflow(s.handleWorkflowRemindEdit)
	case "/workflow/remind/delete/{id}": // 删除 → DeleteRemind
		return s.serveWorkflow(s.handleWorkflowRemindDelete)

	// ── /exception/**：任务异常 ──
	case "/workflow/exception/list": // 按实例查异常 → ListTaskExceptions
		return s.serveWorkflow(s.handleWorkflowExceptionList)
	case "/workflow/exception/delete/{ids}": // 批量处理异常 → HandleTaskExceptions
		return s.serveWorkflow(s.handleWorkflowExceptionDelete)

	// ── /dict/**：内置静态字典（D9）──
	case "/workflow/dict/{code}": // 字典码表 → workflow.DictService（无状态可直接使用）
		return s.serveWorkflow(s.handleWorkflowDict)

	// ── 保留 501 的端点（服务已注入也维持桩，待后续阶段接线）──
	// 导入/导出/节点能力（P6 后续评估）：XML 与整流程导入、整流程导出、运行时节点注入、示例/等待节点清单
	case "/workflow/process/import/xml",
		"/workflow/process/importProcess",
		"/workflow/process/exportProcess",
		"/workflow/process/inject/node",      // 运行时节点注入（引擎直连能力，P6 评估）
		"/workflow/process/exmple/node/list", // 示例节点清单（P6）
		"/workflow/process/wait/node/list":   // 等待节点清单（P6）
		return s.handleWorkflowNotImplemented
	// public 发布族：仅按模型发布（public/model/id）已接 DeployModel，其余发布语义未提供（P6 评估）
	case "/workflow/process/public/deploymentId",
		"/workflow/process/public/processes/id":
		return s.handleWorkflowNotImplemented
	// 快照只读：hisstep 增删改破坏部署快照一致性，仅开放 list
	case "/workflow/hisstep/add", "/workflow/hisstep/edit", "/workflow/hisstep/delete/{ids}":
		return s.handleWorkflowNotImplemented
	default:
		return s.handleWorkflowNotImplemented
	}
}

// workflowHandler 是 /workflow/** 已接线业务 handler 的统一签名。
// @param w 响应
// @param r 请求
// @param params 参数读取器（query 优先、JSON body 兜底）
type workflowHandler func(http.ResponseWriter, *http.Request, *workflowParams)

// serveWorkflow 包装业务 handler 为路由入口：
// 1. 未注入 workflow 服务时回落 501 桩（保留"未注入→501"协议行为）；
// 2. 统一解析参数（query 优先、JSON body 兜底），解析失败回参数错误。
// @param business 已接线业务 handler
// @return HTTP 处理器
func (s *Server) serveWorkflow(business workflowHandler) http.HandlerFunc {
	// 未注入门面服务：全部端点维持阶段 0 的 501 桩
	if s.workflow == nil {
		return s.handleWorkflowNotImplemented
	}
	return func(w http.ResponseWriter, r *http.Request) {
		params, err := newWorkflowParams(r)
		if err != nil {
			writeWorkflowParamError(w, err)
			return
		}
		business(w, r, params)
	}
}

// handleWorkflowNotImplemented 返回未接线端点响应：
// HTTP 501 + R{code:"500", ErrorMessage:"接口未实现", success:false}。
// @param w 响应
// @param r 请求
func (s *Server) handleWorkflowNotImplemented(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotImplemented, api.RErr(workflowNotImplementedCode, workflowNotImplementedMsg))
}
