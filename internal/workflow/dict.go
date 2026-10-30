// dict.go 提供工作流静态字典服务（D9）：
// 码表内置 Go map（数据见 dict_data.go），对位 Java 前端 $store.getters.dictionary 后端字典，
// 管理台经 /workflow/dict/{code} 读取；未知名返回错误。
package workflow

import (
	"fmt"
	"sort"

	"github.com/DengQingNian/Flowtide/api"
)

// 字典码常量（管理台所需码表名，与 dict_data.go 的 staticDicts 键一一对应）。
const (
	// DictCodeMessageType 消息渠道（催办/提醒发送渠道）。
	DictCodeMessageType = "messageType"
	// DictCodeEnable 启用状态。
	DictCodeEnable = "enable"
	// DictCodeVisible 可见性。
	DictCodeVisible = "visible"
	// DictCodeConsignType 委托类型（值与 Java WorkflowTaskDelegateEntity.CONSIGNTYPE_* 一致）。
	DictCodeConsignType = "consignType"
	// DictCodeEntrustCondition 委托条件（值即条件串前缀，见 006 §5 硬约定）。
	DictCodeEntrustCondition = "entrustCondition"
	// DictCodeExceptionType 异常类型（值与 Java WorkflowTaskExceptionEntity.EXCEPTION_TYPE_* 一致）。
	DictCodeExceptionType = "exceptionType"
	// DictCodeTargetType 提醒目标类型。
	DictCodeTargetType = "targetType"
	// DictCodeRemindType 提醒类型。
	DictCodeRemindType = "remindType"
	// DictCodeTaskType 任务类型（BPMN 任务形态）。
	DictCodeTaskType = "taskType"
	// DictCodeFlowStatus 流程状态。
	DictCodeFlowStatus = "flowStatus"
	// DictCodeHandleStatus 异常处理状态（workflow_task_exception.handle_status）。
	DictCodeHandleStatus = "handleStatus"
)

// DictItem 是字典条目。
type DictItem struct {
	Value string `json:"value"` // 条目值（协议字面值）
	Label string `json:"label"` // 中文名称
}

// Dict 是一个字典码表。
type Dict struct {
	Code  string     `json:"code"`  // 字典码
	Items []DictItem `json:"items"` // 条目列表
}

// DictService 是静态字典服务（无状态，可安全并发使用）。
type DictService struct{}

// NewDictService 创建字典服务。
// @return 字典服务
func NewDictService() *DictService {
	return &DictService{}
}

// GetDict 按码名取字典。
// @param code 字典码（如 messageType）
// @return 字典（含条目列表）
// @return 未知名错误
func (s *DictService) GetDict(code string) (*Dict, error) {
	items, ok := staticDicts[code]
	if !ok {
		return nil, api.NewError(errCodeDictUnknown, fmt.Sprintf("字典不存在: %s", code), api.ErrNotFound)
	}
	// 返回条目副本，防止调用方污染静态数据
	copied := make([]DictItem, len(items))
	copy(copied, items)
	return &Dict{Code: code, Items: copied}, nil
}

// ListDictCodes 返回全部字典码（升序）。
// @return 字典码列表
func (s *DictService) ListDictCodes() []string {
	codes := make([]string, 0, len(staticDicts))
	for code := range staticDicts {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes
}

// GetDictItems 按码名直接取条目副本（内部与同包调用便捷方法）。
// @param code 字典码
// @return 条目列表
// @return 未知名错误
func GetDictItems(code string) ([]DictItem, error) {
	dict, err := NewDictService().GetDict(code)
	if err != nil {
		return nil, err
	}
	return dict.Items, nil
}
