package flowable

import (
	"context"
	"strconv"
	"strings"

	"github.com/flowable-go/flowable/api"
)

// HistoricVariableInstanceQuery 是历史变量查询条件。
// ACT_HI_VARINST 未持久化租户与流程定义列，租户与流程归属过滤由调用方在内存完成。
type HistoricVariableInstanceQuery struct {
	ID                string // 历史/变量 ID
	Name              string // 变量名
	ProcessInstanceID string // 流程实例 ID
	ExecutionID       string // 执行 ID
	TaskID            string // 任务 ID
	VariableType      string // 变量类型
	Start             int    // 偏移量
	Size              int    // 每页数量
}

// QueryHistoricVariableInstanceIDs 按条件统计历史变量总数并分页返回变量 ID。
// 查询在 ACT_HI_VARINST 上执行，返回顺序固定为变量名升序、变量 ID 升序。
// @param ctx 上下文
// @param query 查询条件
// @return 变量 ID 列表
// @return 变量总数
// @return 查询错误
func (r *Repository) QueryHistoricVariableInstanceIDs(ctx context.Context, query HistoricVariableInstanceQuery) ([]string, int64, error) {
	queryDB, ok := r.db.(QueryDB)
	if !ok {
		return nil, 0, api.NewError("ROW-0060", "数据库不支持查询", api.ErrBadRequest)
	}
	start, size := normalizeTaskQueryPage(query.Start, query.Size)
	from, where, args := buildHistoricVariableQuerySQL(r.dialect, query)
	total, err := queryHistoricVariableCount(ctx, queryDB, "SELECT COUNT(*) FROM "+from+where, args)
	if err != nil {
		return nil, 0, err
	}
	selectQuery := "SELECT V.ID_ FROM " + from + where +
		" ORDER BY V.NAME_ ASC, V.ID_ ASC LIMIT " + placeholder(r.dialect, len(args)+1) +
		" OFFSET " + placeholder(r.dialect, len(args)+2)
	selectArgs := append(append([]any(nil), args...), size, start)
	ids, err := queryHistoricVariableIDList(ctx, queryDB, selectQuery, selectArgs)
	if err != nil {
		return nil, 0, err
	}
	return ids, total, nil
}

// buildHistoricVariableQuerySQL 按白名单拼接历史变量查询的 FROM、WHERE 与参数。
// @param dialect 数据库方言
// @param query 查询条件
// @return FROM 片段
// @return WHERE 片段，无条件时为空
// @return 参数列表
func buildHistoricVariableQuerySQL(dialect string, query HistoricVariableInstanceQuery) (string, string, []any) {
	from := "ACT_HI_VARINST V"
	args := make([]any, 0, 8)
	conditions := make([]string, 0, 8)
	appendEqual := func(column, value string) {
		// 空条件不参与拼接，避免生成无意义的等值判断
		if value == "" {
			return
		}
		args = append(args, value)
		conditions = append(conditions, column+" = "+placeholder(dialect, len(args)))
	}
	appendEqual("V.ID_", query.ID)
	appendEqual("V.NAME_", query.Name)
	appendEqual("V.PROC_INST_ID_", query.ProcessInstanceID)
	appendEqual("V.EXECUTION_ID_", query.ExecutionID)
	appendEqual("V.TASK_ID_", query.TaskID)
	appendEqual("V.VAR_TYPE_", query.VariableType)
	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}
	return from, where, args
}

// queryHistoricVariableCount 执行历史变量计数查询。
// @param ctx 上下文
// @param db 查询数据库
// @param query SQL
// @param args 参数
// @return 历史变量总数
// @return 统计错误
func queryHistoricVariableCount(ctx context.Context, db QueryDB, query string, args []any) (int64, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, api.NewError("ROW-0061", "统计历史变量数量失败", err)
	}
	defer func() { _ = rows.Close() }()
	// 计数查询必须返回一行，否则按数据库异常处理而不是当作空结果
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, api.NewError("ROW-0061", "统计历史变量数量失败", err)
		}
		return 0, api.NewError("ROW-0061", "统计历史变量数量失败", nil)
	}
	var raw any
	if err := rows.Scan(&raw); err != nil {
		return 0, api.NewError("ROW-0061", "统计历史变量数量失败", err)
	}
	total, err := historicVariableCountValue(raw)
	if err != nil {
		return 0, api.NewError("ROW-0061", "统计历史变量数量失败", err)
	}
	return total, nil
}

// queryHistoricVariableIDList 执行历史变量 ID 列表查询。
// @param ctx 上下文
// @param db 查询数据库
// @param query SQL
// @param args 参数
// @return 变量 ID 列表
// @return 查询错误
func queryHistoricVariableIDList(ctx context.Context, db QueryDB, query string, args []any) ([]string, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, api.NewError("ROW-0062", "查询历史变量 ID 失败", err)
	}
	defer func() { _ = rows.Close() }()
	ids := make([]string, 0)
	for rows.Next() {
		var raw any
		if err := rows.Scan(&raw); err != nil {
			return nil, api.NewError("ROW-0063", "读取历史变量 ID 失败", err)
		}
		id, err := historicVariableIDValue(raw)
		if err != nil {
			return nil, api.NewError("ROW-0063", "读取历史变量 ID 失败", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, api.NewError("ROW-0064", "遍历历史变量 ID 失败", err)
	}
	return ids, nil
}

// historicVariableCountValue 将驱动返回的计数转换为整数。
// @param value 驱动值
// @return 计数
// @return 转换错误
func historicVariableCountValue(value any) (int64, error) {
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
		return 0, api.NewError("ROW-0061", "统计历史变量数量失败", api.ErrBadRequest)
	}
}

// historicVariableIDValue 将驱动返回的历史变量 ID 转换为字符串。
// @param value 驱动值
// @return 变量 ID
// @return 转换错误
func historicVariableIDValue(value any) (string, error) {
	switch typed := value.(type) {
	case string:
		return typed, nil
	case []byte:
		return string(typed), nil
	default:
		return "", api.NewError("ROW-0063", "读取历史变量 ID 失败", api.ErrBadRequest)
	}
}
