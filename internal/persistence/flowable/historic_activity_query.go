package flowable

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/flowable-go/flowable/api"
)

// HistoricActivityInstanceQuery 是历史活动查询条件。
// 过滤字段只覆盖 ACT_HI_ACTINST 已持久化的列；模糊匹配过滤尚未下推。
type HistoricActivityInstanceQuery struct {
	ID                   string     // 历史活动 ID
	ProcessInstanceID    string     // 流程实例 ID
	ProcessDefinitionID  string     // 流程定义 ID
	ProcessDefinitionKey string     // 流程定义 Key
	ActivityID           string     // 活动 ID
	ActivityName         string     // 活动名称
	ActivityType         string     // 活动类型
	Assignee             string     // 处理人
	TenantID             string     // 租户，空表示不过滤
	StartedAfter         *time.Time // 开始时间下界，含边界
	StartedBefore        *time.Time // 开始时间上界，含边界
	FinishedAfter        *time.Time // 结束时间下界，含边界
	FinishedBefore       *time.Time // 结束时间上界，含边界
	Finished             *bool      // 是否已结束，空表示不过滤
	Start                int        // 偏移量
	Size                 int        // 每页数量
}

// QueryHistoricActivityInstanceIDs 按条件统计历史活动总数并分页返回活动 ID。
// 查询在 ACT_HI_ACTINST 上执行，返回顺序固定为开始时间升序、活动 ID 升序。
// @param ctx 上下文
// @param query 查询条件
// @return 活动 ID 列表
// @return 活动总数
// @return 查询错误
func (r *Repository) QueryHistoricActivityInstanceIDs(ctx context.Context, query HistoricActivityInstanceQuery) ([]string, int64, error) {
	queryDB, ok := r.db.(QueryDB)
	if !ok {
		return nil, 0, api.NewError("ROW-0055", "数据库不支持查询", api.ErrBadRequest)
	}
	start, size := normalizeTaskQueryPage(query.Start, query.Size)
	from, where, args := buildHistoricActivityQuerySQL(r.dialect, query)
	total, err := queryHistoricActivityCount(ctx, queryDB, "SELECT COUNT(*) FROM "+from+where, args)
	if err != nil {
		return nil, 0, err
	}
	selectQuery := "SELECT A.ID_ FROM " + from + where +
		" ORDER BY A.START_TIME_ ASC, A.ID_ ASC LIMIT " + placeholder(r.dialect, len(args)+1) +
		" OFFSET " + placeholder(r.dialect, len(args)+2)
	selectArgs := append(append([]any(nil), args...), size, start)
	ids, err := queryHistoricActivityIDList(ctx, queryDB, selectQuery, selectArgs)
	if err != nil {
		return nil, 0, err
	}
	return ids, total, nil
}

// buildHistoricActivityQuerySQL 按白名单拼接历史活动查询的 FROM、WHERE 与参数。
// @param dialect 数据库方言
// @param query 查询条件
// @return FROM 片段
// @return WHERE 片段，无条件时为空
// @return 参数列表
func buildHistoricActivityQuerySQL(dialect string, query HistoricActivityInstanceQuery) (string, string, []any) {
	from := "ACT_HI_ACTINST A"
	args := make([]any, 0, 8)
	// JOIN 谓词位于 WHERE 之前，先占位以保证 PostgreSQL 参数编号与 SQL 文本顺序一致
	if query.ProcessDefinitionKey != "" {
		args = append(args, query.ProcessDefinitionKey)
		from += " INNER JOIN ACT_RE_PROCDEF P ON P.ID_ = A.PROC_DEF_ID_ AND P.KEY_ = " + placeholder(dialect, len(args))
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
	appendEqual("A.ID_", query.ID)
	appendEqual("A.PROC_INST_ID_", query.ProcessInstanceID)
	appendEqual("A.PROC_DEF_ID_", query.ProcessDefinitionID)
	appendEqual("A.ACT_ID_", query.ActivityID)
	appendEqual("A.ACT_NAME_", query.ActivityName)
	appendEqual("A.ACT_TYPE_", query.ActivityType)
	appendEqual("A.ASSIGNEE_", query.Assignee)
	appendEqual("A.TENANT_ID_", query.TenantID)
	// 结束状态以 END_TIME_ 是否为空表达，与内存 EndTime 判空语义一致
	if query.Finished != nil {
		if *query.Finished {
			conditions = append(conditions, "A.END_TIME_ IS NOT NULL")
		} else {
			conditions = append(conditions, "A.END_TIME_ IS NULL")
		}
	}
	// 边界语义与内存匹配一致：下界含等于、上界含等于；空结束时间不满足任何结束区间
	if query.StartedAfter != nil {
		args = append(args, *query.StartedAfter)
		conditions = append(conditions, "A.START_TIME_ >= "+placeholder(dialect, len(args)))
	}
	if query.StartedBefore != nil {
		args = append(args, *query.StartedBefore)
		conditions = append(conditions, "A.START_TIME_ <= "+placeholder(dialect, len(args)))
	}
	if query.FinishedAfter != nil {
		args = append(args, *query.FinishedAfter)
		conditions = append(conditions, "A.END_TIME_ >= "+placeholder(dialect, len(args)))
	}
	if query.FinishedBefore != nil {
		args = append(args, *query.FinishedBefore)
		conditions = append(conditions, "A.END_TIME_ <= "+placeholder(dialect, len(args)))
	}
	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}
	return from, where, args
}

// queryHistoricActivityCount 执行历史活动计数查询。
// @param ctx 上下文
// @param db 查询数据库
// @param query SQL
// @param args 参数
// @return 历史活动总数
// @return 统计错误
func queryHistoricActivityCount(ctx context.Context, db QueryDB, query string, args []any) (int64, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, api.NewError("ROW-0056", "统计历史活动数量失败", err)
	}
	defer func() { _ = rows.Close() }()
	// 计数查询必须返回一行，否则按数据库异常处理而不是当作空结果
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, api.NewError("ROW-0056", "统计历史活动数量失败", err)
		}
		return 0, api.NewError("ROW-0056", "统计历史活动数量失败", nil)
	}
	var raw any
	if err := rows.Scan(&raw); err != nil {
		return 0, api.NewError("ROW-0056", "统计历史活动数量失败", err)
	}
	total, err := historicActivityCountValue(raw)
	if err != nil {
		return 0, api.NewError("ROW-0056", "统计历史活动数量失败", err)
	}
	return total, nil
}

// queryHistoricActivityIDList 执行历史活动 ID 列表查询。
// @param ctx 上下文
// @param db 查询数据库
// @param query SQL
// @param args 参数
// @return 活动 ID 列表
// @return 查询错误
func queryHistoricActivityIDList(ctx context.Context, db QueryDB, query string, args []any) ([]string, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, api.NewError("ROW-0057", "查询历史活动 ID 失败", err)
	}
	defer func() { _ = rows.Close() }()
	ids := make([]string, 0)
	for rows.Next() {
		var raw any
		if err := rows.Scan(&raw); err != nil {
			return nil, api.NewError("ROW-0058", "读取历史活动 ID 失败", err)
		}
		id, err := historicActivityIDValue(raw)
		if err != nil {
			return nil, api.NewError("ROW-0058", "读取历史活动 ID 失败", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, api.NewError("ROW-0059", "遍历历史活动 ID 失败", err)
	}
	return ids, nil
}

// historicActivityCountValue 将驱动返回的计数转换为整数。
// @param value 驱动值
// @return 计数
// @return 转换错误
func historicActivityCountValue(value any) (int64, error) {
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
		return 0, api.NewError("ROW-0056", "统计历史活动数量失败", api.ErrBadRequest)
	}
}

// historicActivityIDValue 将驱动返回的历史活动 ID 转换为字符串。
// @param value 驱动值
// @return 活动 ID
// @return 转换错误
func historicActivityIDValue(value any) (string, error) {
	switch typed := value.(type) {
	case string:
		return typed, nil
	case []byte:
		return string(typed), nil
	default:
		return "", api.NewError("ROW-0058", "读取历史活动 ID 失败", api.ErrBadRequest)
	}
}
