package workflow

// Repositories 聚合全部 workflow 表仓储，便于调用方一次构建后按域访问。
type Repositories struct {
	BusinessSystems      BusinessSystemRepository     // 业务系统注册
	Models               ModelRepository              // 流程模型/分类
	Steps                StepRepository               // 设计时步骤配置
	HisSteps             HisStepRepository            // 步骤部署快照（规则字典）
	Templates            TemplateRepository           // 设计器模板
	ActExtends           ActExtendRepository          // 活动扩展（审阅人绑定）
	CountersignWeights   CountersignWeightRepository  // 会签权重
	NextActivities       NextActivityRepository       // 下一步预生成（运行态）
	HisNextActivities    HisNextActivityRepository    // 下一步历史（双写）
	MessageExtends       MessageExtendRepository      // 待办扩展
	HistoryExtends       HistoryExtendRepository      // 已办扩展
	DelegateTasks        DelegateTaskRepository       // 委托镜像待办
	Documents            DocumentRepository           // 业务单据流程状态
	OperationRecords     OperationRecordRepository    // 操作记录
	ReviewTasks          ReviewTaskRepository         // 审阅影子任务
	TaskExceptions       TaskExceptionRepository      // 任务异常
	CompleteToActivities CompleteToActivityRepository // 直送记录
	TaskDelegates        TaskDelegateRepository       // 委托规则主表
	TaskConsigners       TaskConsignersRepository     // 被委托人
	TaskConsignObjs      TaskConsignObjRepository     // 委托对象
	Reminds              RemindRepository             // 催办配置
	RemindTargets        RemindTargetRepository       // 催办对象
}

// NewRepositories 按仓储入口构建全部 workflow 表仓储。
// @param store 仓储入口
// @return 聚合仓储
func NewRepositories(store *Store) *Repositories {
	return &Repositories{
		BusinessSystems:      NewBusinessSystemRepository(store),
		Models:               NewModelRepository(store),
		Steps:                NewStepRepository(store),
		HisSteps:             NewHisStepRepository(store),
		Templates:            NewTemplateRepository(store),
		ActExtends:           NewActExtendRepository(store),
		CountersignWeights:   NewCountersignWeightRepository(store),
		NextActivities:       NewNextActivityRepository(store),
		HisNextActivities:    NewHisNextActivityRepository(store),
		MessageExtends:       NewMessageExtendRepository(store),
		HistoryExtends:       NewHistoryExtendRepository(store),
		DelegateTasks:        NewDelegateTaskRepository(store),
		Documents:            NewDocumentRepository(store),
		OperationRecords:     NewOperationRecordRepository(store),
		ReviewTasks:          NewReviewTaskRepository(store),
		TaskExceptions:       NewTaskExceptionRepository(store),
		CompleteToActivities: NewCompleteToActivityRepository(store),
		TaskDelegates:        NewTaskDelegateRepository(store),
		TaskConsigners:       NewTaskConsignersRepository(store),
		TaskConsignObjs:      NewTaskConsignObjRepository(store),
		Reminds:              NewRemindRepository(store),
		RemindTargets:        NewRemindTargetRepository(store),
	}
}
