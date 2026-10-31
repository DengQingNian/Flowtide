# 006 系列文档之十五：REST 路由切换与管理台九页实现记录

> 承接：014（P5 门面统一接线与集成）、013（P5.B 审阅/高亮）。
> 范围：P6 前半——/workflow/** REST 由 501 桩切换到 Service 真实现；管理台"工作流"九大页面。

## 1. REST 路由切换（internal/rest）

### 1.1 结构

- `server.go`：新增 `workflow *workflow.Service` 字段、`WithWorkflowService(svc)` 选项与 `WorkflowService()` 访问器。
- `workflow_routes.go`：`workflowHandlerFor(path)` 按路由绑定独立 handler；`serveWorkflow` 统一守卫（未注入服务 → 501）与参数解析（query 优先、JSON body 兜底）。
- `workflow_handlers.go`：/process、/task、/dict 共 22 个 handler。
- `workflow_handlers_mgmt.go`：/model、/category、/step、/hisstep、/template、/delegate、/remind、/exception 共 34 个 handler。
- 协议：R 信封 HTTP 200 承载成功（Success=true）与业务失败（Success=false + ErrorMessage）；参数/系统错误 500 + RErr。路径参数走 `r.PathValue`。

### 1.2 端点面（路由表共 95 条）

- **已接线 56 条**：发起/办结/退回/软终止（end、stop/{pid}）/删除（按实例、按业务键）/挂起/激活/待办/已办/任务详情/历史（按任务、按实例、按业务键分派）/处理人调整/会签结论/加签/减签/撤回/直送/部署（deplay+deploy→DeployModel 部署+快照链路）/模型与分类增删改查树/步骤增删改查/hisstep 快照查询/模板增删改查/委托 CRUD/催办五端点/异常两端点/高亮（highlight+normal）/字典。
- **保留 501 共 39 条**（逐条注释待接线原因）：
  | 类别 | 数量 | 说明 |
  | --- | --- | --- |
  | 设计器/导入导出 | 19 | add/model、getModelJsonByType、save/xml、export/import、exportProcess/importProcess、copyProcess、index、create、inject/node、exmple/wait、getUserTaskJsonInfo 等（P6 设计器阶段） |
  | 批量与发布族 | 5 | suspend|activate processes（无批量方法）、public 三端点 |
  | 实例分页 | 1 | instance/all/list（Service 无实例分页能力） |
  | 引擎语义差异 | 3 | delegate/task（与委托规则 CRUD 语义不同）、resolve、claim |
  | 域服务内部端口 | 5 | extend 两端点 + weight 三端点（审阅绑定/会签权重未走门面） |
  | 其他 | 6 | operationButtonList、instance/list、hisstep 增删改（快照只读）、identity/tree 等 |

### 1.3 测试

新增 8 个测试函数：核心生命周期全链路、管理链路 + 部署快照、加签减签会签族、委托/催办/异常/字典、
保留 501 抽查 ×20、未注入服务回 501、参数读取器单测、防御分支直调 ×14。新文件行覆盖 90.2%。

## 2. 管理台九大页面（admin/）

- `api/workflow.ts`：类型化客户端，R 信封三态解包（成功/业务失败/501 → notImplemented 标记不抛错）；`client.ts` 兼容 R 大写 `ErrorMessage`。
- 九页（路由组"工作流"）：流程分类（树表/添加子级）、流程模型（部署按钮 → /process/deploy）、流程配置（三 Tab：步骤管理/流程模板+XML 预览/流程规则占位）、待办/已办（身份选择 + 办理弹窗：意见 + 变量 JSON）、流程实例（业务键状态查询 + 终止/挂起/激活/删除 + 高亮抽屉嵌 DiagramPanel）、委托配置（启停切换）、催办配置、流程异常（标记已处理）。
- 公共件：FormModal/ApprovalDialog、`useWorkflowListSource` 三态列表状态机、`pickField` 驼峰/下划线字段兼容。
- 501 端点一律空态降级（"接口接线中"），不阻塞页面。
- `npm run build`（vue-tsc + vite）通过；`npm test` 632 用例全过（lines 95.67%，达 90% 门禁）。

## 3. 集成发现与修复

1. **撤回不重建待办镜像**（REST 接线验收时发现）：撤回恢复的重做任务其待办已在办结时消耗，
   待办中心不可见。已在 rollback.go 收尾链补 `generateTodosAfterRollback`（与退回链同构），门面回归用例断言撤回后待办可见。
2. `/process/complete/sign` 在未启用会签退回规则的实例上返回成功且 data=false（规则未启用与未达比例暂不可区分，P3-A 接口位约定）。
3. 委托规则 `VaildFrom/VaildTo` REST JSON 需 RFC3339 格式或省略（Java 侧传时间戳，无碍）。

## 4. 遗留与后续

1. 设计器/导入导出 19 条端点（P6 后半：BPMN 设计器属性面板 + 模型 XML 存取链路）。
2. 实例分页查询能力（Service 层补 QueryService 实例分页）。
3. operationButtonList 按钮聚合、identity/tree 组树。
4. 审阅绑定与权重保存走门面（当前为域服务内部端口）。
