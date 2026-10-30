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

// workflowRouteTable 是 workflow 门面 REST 路由清单（阶段 0 全部返回 501）。
// 来源：005 接口清单 §1 中"非桩"端点 + D8 保留拼写决策 + D9/D10/D11 内置能力路径。
// 已排除：Java 桩端点（/task/getNextUserTaskAndUserList、/task/complete/task、
// /task/getBackNodesByProcessInstanceId、/task/reject|delegate|end|reset 等）、
// 明确裁剪模块（/rule、/model/discard、遗留四表 task/history/record/example、旧版 /consigner）。
// 端点行为由 api.WorkflowFacade 服务层承接，实现分阶段填充。
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

// workflowNotImplementedCode 是阶段 0 空实现的协议状态码与文案。
const (
	workflowNotImplementedCode = "500"
	workflowNotImplementedMsg  = "接口未实现"
)

// registerWorkflowRoutes 挂载 /workflow/** 全部路由，统一返回 501 + R 失败体。
// 复用 Server 中间件链：认证/租户/审计/限流对 /workflow/** 与 /service/** 同策略（006 §6）。
// 阶段 0 为 501 空实现，不依赖 workflow 服务实例；P1 起逐端点改接 Service。
// @param s REST 服务
func registerWorkflowRoutes(s *Server) {
	handler := s.handleWorkflowNotImplemented
	for _, route := range workflowRouteTable {
		if route.method == "" {
			// 无方法前缀的 pattern 匹配任意 HTTP 方法（对应 Java ANY）
			s.mux.HandleFunc(route.path, handler)
			continue
		}
		s.mux.HandleFunc(route.method+" "+route.path, handler)
	}
}

// handleWorkflowNotImplemented 返回阶段 0 空实现响应：
// HTTP 501 + R{code:"500", ErrorMessage:"接口未实现", success:false}。
// @param w 响应
// @param r 请求
func (s *Server) handleWorkflowNotImplemented(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotImplemented, api.RErr(workflowNotImplementedCode, workflowNotImplementedMsg))
}
