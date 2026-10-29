// milestones 统一首页和进度页的里程碑编号；完成状态只代表表述范围内的实现及验收。
export const milestones = [
  { id: 'M0', title: '基线与契约', detail: 'OpenAPI、双库 DDL、41 张 ACT 表', state: 'done' },
  { id: 'M1', title: 'BPMN 模型与部署', detail: '安全解析、版本、资源、模型仓库', state: 'done' },
  { id: 'M2', title: '运行时与执行树', detail: 'Command、Agenda、作用域、回滚', state: 'done' },
  { id: 'M3', title: '任务、身份与表单', detail: '任务闭环、身份链接、表单与监听器', state: 'done' },
  { id: 'M4', title: '作业与定时器', detail: '定时、异步续跑、重试与死信', state: 'done' },
  { id: 'M5', title: '历史、审计与明细', detail: '六级历史、full 明细、租户隔离与恢复', state: 'done' },
  { id: 'M6', title: 'REST 与安全', detail: '184 方法基础验收、69 个方法原版 HTTP 场景；全量 REST 待验收', state: 'partial' },
  { id: 'M7', title: '双库与迁移工具', detail: '流程与身份标准行双库单执行者及迁移闭环通过', state: 'done' },
  { id: 'M8', title: '集群与容量', detail: '租约、抢占、故障转移、长稳与生产容量', state: 'todo' },
  { id: 'M9', title: '生产切换与回退', detail: '现场停写、物理备份、反向转换与 Java 增量接管', state: 'todo' },
]
