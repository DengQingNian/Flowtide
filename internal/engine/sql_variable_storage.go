package engine

import (
	"fmt"

	"github.com/flowable-go/flowable/api"
	flowablerow "github.com/flowable-go/flowable/internal/persistence/flowable"
)

// expandVariableRows 将二进制和超过文本列容量的值移至字节表。
// @param rows 变量及其他行
// @return 包含字节行的集合
// @return 类型或编码错误
func expandVariableRows(rows []flowablerow.Row) ([]flowablerow.Row, error) {
	out := make([]flowablerow.Row, 0, len(rows))
	for _, row := range rows {
		typeColumn := "TYPE_"
		if row.Table == "ACT_HI_VARINST" || row.Table == "ACT_HI_DETAIL" {
			typeColumn = "VAR_TYPE_"
		} else if row.Table != "ACT_RU_VARIABLE" {
			out = append(out, row)
			continue
		}
		kind, _ := row.Values[typeColumn].(string)
		if kind == "unsupported" {
			return nil, api.NewError("ENGINE-0919", "变量类型或值无法导出", api.ErrBadRequest)
		}
		data, binary := row.Values["TEXT_"].([]byte)
		text, _ := row.Values["TEXT_"].(string)
		// 字节值总是外置，长字符串和 JSON 使用独立类型标记，读取时恢复原类型。
		if binary || len(text) > 4000 {
			if !binary {
				data = []byte(text)
				if kind == "string" {
					kind = "longString"
				} else {
					kind = "longJson"
				}
			}
			id := byteArrayID(row.Table, fmt.Sprint(row.Values["ID_"]))
			bytesRow, _ := runtimeRow("ACT_GE_BYTEARRAY", []string{"ID_", "REV_", "NAME_", "DEPLOYMENT_ID_", "BYTES_"})
			_ = bytesRow.Set("ID_", id)
			_ = bytesRow.Set("REV_", 1)
			_ = bytesRow.Set("DEPLOYMENT_ID_", nil)
			_ = bytesRow.Set("NAME_", "flowable-go-variable")
			_ = bytesRow.Set("BYTES_", data)
			_ = row.Set(typeColumn, kind)
			_ = row.Set("TEXT_", nil)
			_ = row.Set("BYTEARRAY_ID_", id)
			row.InsertColumns = append(row.InsertColumns, "BYTEARRAY_ID_")
			out = append(out, *bytesRow)
		}
		out = append(out, row)
	}
	return out, nil
}

// normalizeExecutionReferences 将旧快照独立根执行 ID 映射为真实表要求的实例 ID。
// @param rows 待导出行
// @return 引用统一后的行
func (e *Engine) normalizeExecutionReferences(rows []flowablerow.Row) []flowablerow.Row {
	aliases := make(map[string]string)
	for _, instance := range e.state.Instances {
		aliases[instance.RootExecutionID] = instance.ID
	}
	for i := range rows {
		// 只转换执行标识，不改变业务实例和任务标识。
		for _, column := range []string{"EXECUTION_ID_", "PARENT_ID_", "SUPER_EXEC_"} {
			if id, ok := rows[i].Values[column].(string); ok {
				if actual, exists := aliases[id]; exists {
					rows[i].Values[column] = actual
				}
			}
		}
		if rows[i].Table == "ACT_RU_EXECUTION" {
			if id, ok := rows[i].Values["ID_"].(string); ok {
				if actual, exists := aliases[id]; exists {
					rows[i].Values["ID_"] = actual
				}
			}
		}
	}
	return rows
}
