package workflow

import (
	"context"
	"regexp"
	"strings"

	"github.com/DengQingNian/Flowtide/api"
)

// commentMaxRunes 是意见内容截断阈值（对位 Java AddHisCommentCmd 的 3900；
// Java substring 按 UTF-16 代码单元计数，Go 以 rune 计数等价实现）。
const commentMaxRunes = 3900

// whitespacePattern 匹配连续空白（Java message.replaceAll("\\s+", " ")）。
var whitespacePattern = regexp.MustCompile(`\s+`)

// CommentService 是审批意见域服务。
// 行为规格对位 Java WorkflowCommentServiceImpl + AddHisCommentCmd：
// 任务级意见经引擎 TaskService.AddComment、流程级意见经 AddProcessComment 落
// ACT_HI_COMMENT 等价存储；message 折叠空白并按 3900 截断。
// 说明：引擎 AddComment 不透出自定义 type（引擎侧固定 comment），
// 意见类型的权威台账为 workflow_operation_record.type（P1.2 办结流程同时写两者）。
type CommentService struct {
	tasks   TaskCommentEngine // 任务评论能力
	history TaskHistoryQuery  // 历史任务查询（补任务名）
	users   UserQuery         // 身份查询（补操作人姓名）
}

// NewCommentService 创建审批意见域服务。
// @param tasks 任务评论能力（engine.TaskService()）
// @param history 历史任务查询（engine.HistoryService()）
// @param users 身份查询（engine.IdentityService()）
// @return 域服务
func NewCommentService(tasks TaskCommentEngine, history TaskHistoryQuery, users UserQuery) *CommentService {
	return &CommentService{tasks: tasks, history: history, users: users}
}

// normalizeCommentMessage 归一化意见内容：折叠连续空白为单空格；超过 3900 字符截断并追加省略号
// （对位 Java AddHisCommentCmd eventMessage 语义）。
// @param message 原始内容
// @return 归一化内容
func normalizeCommentMessage(message string) string {
	// 空白折叠在截断之前（Java 先 replaceAll 再截断）
	collapsed := whitespacePattern.ReplaceAllString(message, " ")
	runes := []rune(collapsed)
	// 超长截断：保留前 3900 字符并追加 "..."（Java 同款处理）
	if len(runes) > commentMaxRunes {
		return string(runes[:commentMaxRunes]) + "..."
	}
	return collapsed
}

// Add 添加一条审批意见（任务级或流程级）。
// @param ctx 操作上下文
// @param req 意见请求（TaskID 为空表示流程级意见；Type 为空默认 SP）
// @return 引擎评论
// @return 错误
func (s *CommentService) Add(ctx context.Context, req CommentRequest) (api.Comment, error) {
	// 操作人与实例是意见留痕的最小定位信息
	if req.UserID == "" || req.ProcessInstanceID == "" {
		return api.Comment{}, apiError("WORKFLOW-1101", "添加意见缺少操作人或流程实例")
	}
	// 引擎侧拒绝空白评论（Java 允许空意见落库，Go 引擎统一拒绝，见 task_extra.go）
	if strings.TrimSpace(req.Message) == "" {
		return api.Comment{}, apiError("WORKFLOW-1102", "意见内容不能为空")
	}
	// Type 空时按 Java AddHisCommentCmd 默认 SP；引擎 API 不透出自定义 type，
	// 类型权威台账在 workflow_operation_record.type（由 OperationRecordService 落库）
	_ = firstNonEmpty(req.Type, CommentTypeSP)
	message := normalizeCommentMessage(req.Message)
	// 任务级意见挂任务，流程级意见挂实例（Java 直插 ACT_HI_COMMENT 的两条路径）
	if req.TaskID != "" {
		return s.tasks.AddComment(ctx, req.TaskID, req.UserID, message)
	}
	return s.tasks.AddProcessComment(ctx, req.ProcessInstanceID, req.UserID, message)
}

// ListByProcessInstance 查询实例全部意见并补全展示字段：
// 任务名（联历史任务）、中文类型名（GetEnumMsgByType 映射）、操作人姓名（联身份库）——
// 查不到一律给空（与 Java getFlowCommentVosByProcessInstanceId 行为一致）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 意见视图列表
// @return 错误
func (s *CommentService) ListByProcessInstance(ctx context.Context, processInstanceID string) ([]CommentView, error) {
	comments, err := s.tasks.ListInstanceComments(ctx, processInstanceID)
	if err != nil {
		return nil, err
	}
	views := make([]CommentView, 0, len(comments))
	for _, comment := range comments {
		view := CommentView{Comment: comment, TypeName: GetEnumMsgByType(comment.Type)}
		// 任务级评论联历史任务补任务名，查不到给空
		if comment.TaskID != "" {
			if task, err := s.history.GetHistoricTaskInstance(ctx, comment.TaskID); err == nil {
				view.TaskName = task.Name
			}
		}
		// 操作人姓名联身份库，查不到给空
		if comment.UserID != "" {
			if user, err := s.users.GetUser(ctx, comment.UserID); err == nil {
				view.UserName = userDisplayName(user)
			}
		}
		views = append(views, view)
	}
	return views, nil
}

// userDisplayName 返回用户展示名：优先 DisplayName，回退姓+名。
// @param user 用户
// @return 展示名，全空返回空串
func userDisplayName(user api.User) string {
	if user.DisplayName != "" {
		return user.DisplayName
	}
	// 姓+名拼接回退（Java 用户姓名来源的等价形态）
	return user.LastName + user.FirstName
}
