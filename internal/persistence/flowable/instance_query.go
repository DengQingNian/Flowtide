package flowable

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/DengQingNian/Flowtide/api"
)

// ProcessInstanceQuery 是运行时流程实例查询条件。
// 过滤字段只覆盖 ACT_RU_EXECUTION 已持久化的列；变量过滤尚未下推。
type ProcessInstanceQuery struct {
	ID                   string     // 流程实例 ID
	ProcessDefinitionID  string     // 流程定义 ID
	ProcessDefinitionKey string     // 流程定义 Key
	BusinessKey          string     // 业务键
	TenantID             string     // 租户，空表示不过滤
	Suspended            *bool      // 是否挂起，空表示不过滤
	StartedAfter         *time.Time // 开始时间下界，含边界
	StartedBefore        *time.Time // 开始时间上界，含边界
	Start                int        // 偏移量
	Size                 int        // 每页数量
}

// QueryProcessInstanceIDs 按条件统计运行中实例总数并分页返回实例 ID。
// 查询限定 ACT_RU_EXECUTION 的实例根行（PARENT_ID_ 为空），返回顺序固定为开始时间升序、实例 ID 升序。
// @param ctx 上下文
// @param query 查询条件
// @return 实例 ID 列表
// @return 实例总数
// @return 查询错误
func (r *Repository) QueryProcessInstanceIDs(ctx context.Context, query ProcessInstanceQuery) ([]string, int64, error) {
	queryDB, ok := r.db.(QueryDB)
	if !ok {
		return nil, 0, api.NewError("ROW-0040", "数据库不支持查询", api.ErrBadRequest)
	}
	start, size := normalizeTaskQueryPage(query.Start, query.Size)
	from, where, args := buildProcessInstanceQuerySQL(r.dialect, query)
	total, err := queryInstanceCount(ctx, queryDB, "SELECT COUNT(*) FROM "+from+where, args)
	if err != nil {
		return nil, 0, err
	}
	selectQuery := "SELECT E.PROC_INST_ID_ FROM " + from + where +
		" ORDER BY E.START_TIME_ ASC, E.PROC_INST_ID_ ASC LIMIT " + placeholder(r.dialect, len(args)+1) +
		" OFFSET " + placeholder(r.dialect, len(args)+2)
	selectArgs := append(append([]any(nil), args...), size, start)
	ids, err := queryInstanceIDList(ctx, queryDB, selectQuery, selectArgs)
	if err != nil {
		return nil, 0, err
	}
	return ids, total, nil
}

// buildProcessInstanceQuerySQL 按白名单拼接实例查询的 FROM、WHERE 与参数。
// @param dialect 数据库方言
// @param query 查询条件
// @return FROM 片段
// @return WHERE 片段，无条件时仍包含实例根限定
// @return 参数列表
func buildProcessInstanceQuerySQL(dialect string, query ProcessInstanceQuery) (string, string, []any) {
	from := "ACT_RU_EXECUTION E"
	args := make([]any, 0, 8)
	// JOIN 谓词位于 WHERE 之前，先占位以保证 PostgreSQL 参数编号与 SQL 文本顺序一致
	if query.ProcessDefinitionKey != "" {
		args = append(args, query.ProcessDefinitionKey)
		from += " INNER JOIN ACT_RE_PROCDEF P ON P.ID_ = E.PROC_DEF_ID_ AND P.KEY_ = " + placeholder(dialect, len(args))
	}
	conditions := []string{"E.PARENT_ID_ IS NULL"}
	appendEqual := func(column, value string) {
		// 空条件不参与拼接，避免生成无意义的等值判断
		if value == "" {
			return
		}
		args = append(args, value)
		conditions = append(conditions, column+" = "+placeholder(dialect, len(args)))
	}
	appendEqual("E.ID_", query.ID)
	appendEqual("E.PROC_DEF_ID_", query.ProcessDefinitionID)
	appendEqual("E.BUSINESS_KEY_", query.BusinessKey)
	// 租户为空表示不过滤；非空租户以标准行 TENANT_ID_ 等值下推
	appendEqual("E.TENANT_ID_", query.TenantID)
	// 挂起状态以标准行导出的 SUSPENSION_STATE_ 编码为准：1 活动、2 挂起
	if query.Suspended != nil {
		state := 1
		if *query.Suspended {
			state = 2
		}
		args = append(args, state)
		conditions = append(conditions, "E.SUSPENSION_STATE_ = "+placeholder(dialect, len(args)))
	}
	// 边界语义与内存匹配一致：下界含等于、上界含等于
	if query.StartedAfter != nil {
		args = append(args, *query.StartedAfter)
		conditions = append(conditions, "E.START_TIME_ >= "+placeholder(dialect, len(args)))
	}
	if query.StartedBefore != nil {
		args = append(args, *query.StartedBefore)
		conditions = append(conditions, "E.START_TIME_ <= "+placeholder(dialect, len(args)))
	}
	return from, " WHERE " + strings.Join(conditions, " AND "), args
}

// queryInstanceCount 执行实例计数查询。
// @param ctx 上下文
// @param db 查询数据库
// @param query SQL
// @param args 参数
// @return 实例总数
// @return 统计错误
func queryInstanceCount(ctx context.Context, db QueryDB, query string, args []any) (int64, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, api.NewError("ROW-0041", "统计流程实例数量失败", err)
	}
	defer func() { _ = rows.Close() }()
	// 计数查询必须返回一行，否则按数据库异常处理而不是当作空结果
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, api.NewError("ROW-0041", "统计流程实例数量失败", err)
		}
		return 0, api.NewError("ROW-0041", "统计流程实例数量失败", nil)
	}
	var raw any
	if err := rows.Scan(&raw); err != nil {
		return 0, api.NewError("ROW-0041", "统计流程实例数量失败", err)
	}
	total, err := instanceCountValue(raw)
	if err != nil {
		return 0, api.NewError("ROW-0041", "统计流程实例数量失败", err)
	}
	return total, nil
}

// queryInstanceIDList 执行实例 ID 列表查询。
// @param ctx 上下文
// @param db 查询数据库
// @param query SQL
// @param args 参数
// @return 实例 ID 列表
// @return 查询错误
func queryInstanceIDList(ctx context.Context, db QueryDB, query string, args []any) ([]string, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, api.NewError("ROW-0042", "查询流程实例 ID 失败", err)
	}
	defer func() { _ = rows.Close() }()
	ids := make([]string, 0)
	for rows.Next() {
		var raw any
		if err := rows.Scan(&raw); err != nil {
			return nil, api.NewError("ROW-0043", "读取流程实例 ID 失败", err)
		}
		id, err := instanceIDValue(raw)
		if err != nil {
			return nil, api.NewError("ROW-0043", "读取流程实例 ID 失败", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, api.NewError("ROW-0044", "遍历流程实例 ID 失败", err)
	}
	return ids, nil
}

// instanceCountValue 将驱动返回的计数转换为整数。
// @param value 驱动值
// @return 计数
// @return 转换错误
func instanceCountValue(value any) (int64, error) {
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
		return 0, api.NewError("ROW-0041", "统计流程实例数量失败", api.ErrBadRequest)
	}
}

// instanceIDValue 将驱动返回的实例 ID 转换为字符串。
// @param value 驱动值
// @return 实例 ID
// @return 转换错误
func instanceIDValue(value any) (string, error) {
	switch typed := value.(type) {
	case string:
		return typed, nil
	case []byte:
		return string(typed), nil
	default:
		return "", api.NewError("ROW-0043", "读取流程实例 ID 失败", api.ErrBadRequest)
	}
}
