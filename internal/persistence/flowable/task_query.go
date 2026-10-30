package flowable

import (
	"context"
	"strconv"
	"strings"

	"github.com/DengQingNian/Flowtide/api"
)

// TaskQuery 是运行时任务查询条件。
type TaskQuery struct {
	Assignee             string   // 处理人
	Owner                string   // 所有者
	DelegationState      string   // 委派状态
	ProcessInstanceID    string   // 流程实例 ID
	ProcessDefinitionKey string   // 流程定义 Key
	ParentTaskID         string   // 父任务 ID
	TenantID             string   // 租户，空表示不过滤
	CandidateUser        string   // 候选人
	CandidateGroups      []string // 候选组
	Start                int      // 偏移量
	Size                 int      // 每页数量
}

// QueryTaskIDs 按条件统计任务总数并分页返回任务 ID。
// 查询在 ACT_RU_TASK 上执行，返回顺序固定为创建时间升序、ID 升序。
// @param ctx 上下文
// @param query 查询条件
// @return 任务 ID 列表
// @return 任务总数
// @return 查询错误
func (r *Repository) QueryTaskIDs(ctx context.Context, query TaskQuery) ([]string, int64, error) {
	queryDB, ok := r.db.(QueryDB)
	if !ok {
		return nil, 0, api.NewError("ROW-0017", "数据库不支持查询", api.ErrBadRequest)
	}
	start, size := normalizeTaskQueryPage(query.Start, query.Size)
	from, where, args := buildTaskQuerySQL(r.dialect, query)
	total, err := queryTaskCount(ctx, queryDB, "SELECT COUNT(*) FROM "+from+where, args)
	if err != nil {
		return nil, 0, err
	}
	selectQuery := "SELECT T.ID_ FROM " + from + where +
		" ORDER BY T.CREATE_TIME_ ASC, T.ID_ ASC LIMIT " + placeholder(r.dialect, len(args)+1) +
		" OFFSET " + placeholder(r.dialect, len(args)+2)
	selectArgs := append(append([]any(nil), args...), size, start)
	ids, err := queryTaskIDList(ctx, queryDB, selectQuery, selectArgs)
	if err != nil {
		return nil, 0, err
	}
	return ids, total, nil
}

// buildTaskQuerySQL 按白名单拼接任务查询的 FROM、WHERE 与参数。
// @param dialect 数据库方言
// @param query 查询条件
// @return FROM 片段
// @return WHERE 片段，无条件时为空
// @return 参数列表
func buildTaskQuerySQL(dialect string, query TaskQuery) (string, string, []any) {
	from := "ACT_RU_TASK T"
	args := make([]any, 0, 8)
	// JOIN 谓词位于 WHERE 之前，先占位以保证 PostgreSQL 参数编号与 SQL 文本顺序一致
	if query.ProcessDefinitionKey != "" {
		args = append(args, query.ProcessDefinitionKey)
		from += " INNER JOIN ACT_RE_PROCDEF P ON P.ID_ = T.PROC_DEF_ID_ AND P.KEY_ = " + placeholder(dialect, len(args))
	}
	conditions := make([]string, 0, 8)
	appendEqual := func(column, value string) {
		// 空条件不参与拼接，避免生成无意义的等值判断
		if value == "" {
			return
		}
		args = append(args, value)
		conditions = append(conditions, column+" = "+placeholder(dialect, len(args)))
	}
	appendEqual("T.ASSIGNEE_", query.Assignee)
	appendEqual("T.OWNER_", query.Owner)
	appendEqual("T.DELEGATION_", query.DelegationState)
	appendEqual("T.PROC_INST_ID_", query.ProcessInstanceID)
	appendEqual("T.PARENT_TASK_ID_", query.ParentTaskID)
	// 租户为空表示不过滤；非空租户以标准行 TENANT_ID_ 等值下推
	appendEqual("T.TENANT_ID_", query.TenantID)
	// 候选人条件同时约束任务未被签收，并匹配用户候选或用户所在组的候选
	if query.CandidateUser != "" {
		args = append(args, query.CandidateUser)
		identity := "L.USER_ID_ = " + placeholder(dialect, len(args))
		groupPlaceholders := make([]string, 0, len(query.CandidateGroups))
		for _, group := range query.CandidateGroups {
			// 空组名没有对应身份链接，跳过以避免无效参数
			if group == "" {
				continue
			}
			args = append(args, group)
			groupPlaceholders = append(groupPlaceholders, placeholder(dialect, len(args)))
		}
		// 候选组为空时只匹配用户候选，绝不生成非法 IN ()
		if len(groupPlaceholders) > 0 {
			identity += " OR L.GROUP_ID_ IN (" + strings.Join(groupPlaceholders, ", ") + ")"
		}
		conditions = append(conditions,
			"(T.ASSIGNEE_ IS NULL OR T.ASSIGNEE_ = '')",
			"EXISTS (SELECT 1 FROM ACT_RU_IDENTITYLINK L WHERE L.TASK_ID_ = T.ID_ AND L.TYPE_ = 'candidate' AND ("+identity+"))")
	}
	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}
	return from, where, args
}

// normalizeTaskQueryPage 规范化任务查询分页参数，与引擎内存路径保持一致。
// @param start 偏移量
// @param size 每页数量
// @return 规范化偏移量
// @return 规范化每页数量
func normalizeTaskQueryPage(start, size int) (int, int) {
	if start < 0 {
		start = 0
	}
	if size <= 0 {
		size = 10
	}
	if size > 1000 {
		size = 1000
	}
	return start, size
}

// queryTaskCount 执行任务计数查询。
// @param ctx 上下文
// @param db 查询数据库
// @param query SQL
// @param args 参数
// @return 任务总数
// @return 统计错误
func queryTaskCount(ctx context.Context, db QueryDB, query string, args []any) (int64, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, api.NewError("ROW-0036", "统计任务数量失败", err)
	}
	defer func() { _ = rows.Close() }()
	// 计数查询必须返回一行，否则按数据库异常处理而不是当作空结果
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, api.NewError("ROW-0036", "统计任务数量失败", err)
		}
		return 0, api.NewError("ROW-0036", "统计任务数量失败", nil)
	}
	var raw any
	if err := rows.Scan(&raw); err != nil {
		return 0, api.NewError("ROW-0036", "统计任务数量失败", err)
	}
	total, err := taskCountValue(raw)
	if err != nil {
		return 0, api.NewError("ROW-0036", "统计任务数量失败", err)
	}
	return total, nil
}

// queryTaskIDList 执行任务 ID 列表查询。
// @param ctx 上下文
// @param db 查询数据库
// @param query SQL
// @param args 参数
// @return 任务 ID 列表
// @return 查询错误
func queryTaskIDList(ctx context.Context, db QueryDB, query string, args []any) ([]string, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, api.NewError("ROW-0037", "查询任务 ID 失败", err)
	}
	defer func() { _ = rows.Close() }()
	ids := make([]string, 0)
	for rows.Next() {
		var raw any
		if err := rows.Scan(&raw); err != nil {
			return nil, api.NewError("ROW-0038", "读取任务 ID 失败", err)
		}
		id, err := taskIDValue(raw)
		if err != nil {
			return nil, api.NewError("ROW-0038", "读取任务 ID 失败", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, api.NewError("ROW-0039", "遍历任务 ID 失败", err)
	}
	return ids, nil
}

// taskCountValue 将驱动返回的计数转换为整数。
// @param value 驱动值
// @return 计数
// @return 转换错误
func taskCountValue(value any) (int64, error) {
	switch typed := value.(type) {
	case int64:
		return typed, nil
	case int:
		return int64(typed), nil
	case string:
		return strconv.ParseInt(typed, 10, 64)
	case []byte:
		return strconv.ParseInt(string(typed), 10, 64)
	default:
		return 0, api.NewError("ROW-0036", "统计任务数量失败", api.ErrBadRequest)
	}
}

// taskIDValue 将驱动返回的任务 ID 转换为字符串。
// @param value 驱动值
// @return 任务 ID
// @return 转换错误
func taskIDValue(value any) (string, error) {
	switch typed := value.(type) {
	case string:
		return typed, nil
	case []byte:
		return string(typed), nil
	default:
		return "", api.NewError("ROW-0038", "读取任务 ID 失败", api.ErrBadRequest)
	}
}
