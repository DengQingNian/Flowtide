package engine

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/flowable-go/flowable/api"
	flowablerow "github.com/flowable-go/flowable/internal/persistence/flowable"
)

// runtimeControl 保存 ACT 标准列无法表达的 Go 续跑控制信息；不宣称 Java 可读。
type runtimeControl struct {
	Version                   int                  `json:"version"`                             // 扩展版本
	Instance                  *instanceState       `json:"instance"`                            // 实例控制
	Executions                []*executionState    `json:"executions"`                          // 不含变量的执行控制
	Jobs                      []*jobState          `json:"jobs"`                                // 作业控制
	Subscriptions             []*subscriptionState `json:"subscriptions"`                       // 订阅控制
	Compensations             []*compensationState `json:"compensations"`                       // 补偿登记
	RecordSequenceFlowHistory bool                 `json:"recordSequenceFlowHistory,omitempty"` // Java 迁移顺序流历史模式
}

// runtimeControlRows 在每个实例的独立字节行中保存复杂执行控制。
// @return 扩展字节行
// @return 编码错误
func (e *Engine) runtimeControlRows() ([]flowablerow.Row, error) {
	rows := make([]flowablerow.Row, 0)
	// 只保存活跃实例的内部控制，变量值仍由标准变量行单独存储。
	for _, instance := range e.state.Instances {
		if instance.Ended {
			continue
		}
		control := runtimeControl{Version: 1, Instance: copyValue(instance).(*instanceState), Compensations: e.state.Compensations[instance.ID], RecordSequenceFlowHistory: e.state.RecordSequenceFlowHistory}
		control.Instance.RootExecutionID = instance.ID
		for _, execution := range e.state.Executions {
			if execution.ProcessInstanceID != instance.ID || execution.Ended {
				continue
			}
			copied := *execution
			copied.Variables = nil
			copied.Transient = nil
			if copied.ID == instance.RootExecutionID {
				copied.ID = instance.ID
			}
			if copied.ParentID == instance.RootExecutionID {
				copied.ParentID = instance.ID
			}
			control.Executions = append(control.Executions, &copied)
		}
		for _, job := range e.state.Jobs {
			if job.ProcessInstanceID == instance.ID {
				control.Jobs = append(control.Jobs, job)
			}
		}
		for _, subscription := range e.state.Subscriptions {
			if subscription.ProcessInstanceID == instance.ID {
				control.Subscriptions = append(control.Subscriptions, subscription)
			}
		}
		sort.Slice(control.Executions, func(i, j int) bool { return control.Executions[i].ID < control.Executions[j].ID })
		sort.Slice(control.Jobs, func(i, j int) bool { return control.Jobs[i].ID < control.Jobs[j].ID })
		sort.Slice(control.Subscriptions, func(i, j int) bool { return control.Subscriptions[i].ID < control.Subscriptions[j].ID })
		data, err := json.Marshal(control)
		if err != nil {
			return nil, err
		}
		row, _ := runtimeRow("ACT_GE_BYTEARRAY", []string{"ID_", "REV_", "NAME_", "DEPLOYMENT_ID_", "BYTES_"})
		name := "flowable-go-control/" + instance.ID
		_ = row.Set("ID_", byteArrayID("", name))
		_ = row.Set("REV_", 1)
		_ = row.Set("NAME_", name)
		_ = row.Set("DEPLOYMENT_ID_", nil)
		_ = row.Set("BYTES_", data)
		rows = append(rows, *row)
	}
	return rows, nil
}

// decodeRuntimeControls 读取并校验本实现写入的续跑扩展。
// @param rows 待恢复行
// @return 扩展控制集合
// @return 解析错误
func decodeRuntimeControls(rows []flowablerow.Row) ([]runtimeControl, error) {
	// 只解析本实现的保留命名空间，未知版本不能作为正常状态续跑。
	controls := make([]runtimeControl, 0)
	for _, row := range rows {
		if row.Table != "ACT_GE_BYTEARRAY" {
			continue
		}
		name, _ := row.Values["NAME_"].(string)
		if !strings.HasPrefix(name, "flowable-go-control/") {
			continue
		}
		data, ok := row.Values["BYTES_"].([]byte)
		var control runtimeControl
		if !ok || json.Unmarshal(data, &control) != nil || control.Version != 1 || control.Instance == nil || name != "flowable-go-control/"+control.Instance.ID {
			return nil, api.NewError("ENGINE-0920", "续跑扩展无效或版本不支持", api.ErrBadRequest)
		}
		controls = append(controls, control)
	}
	return controls, nil
}

// restoreRuntimeControls 恢复多实例、网关、历史关联及其他续跑控制。
// @param controls 已解析控制信息
// @return 恢复错误
func (e *Engine) restoreRuntimeControls(controls []runtimeControl) error {
	restored := make(map[string]bool)
	for _, control := range controls {
		if _, exists := e.state.Instances[control.Instance.ID]; !exists {
			return api.NewError("ENGINE-0920", "续跑扩展缺少实例", api.ErrBadRequest)
		}
		e.state.Instances[control.Instance.ID] = control.Instance
		// 行仓储恢复也需沿用迁移历史模式，否则重启续跑会遗漏顺序流历史。
		e.state.RecordSequenceFlowHistory = e.state.RecordSequenceFlowHistory || control.RecordSequenceFlowHistory
		for _, execution := range control.Executions {
			if execution == nil {
				return api.NewError("ENGINE-0920", "执行扩展不能为空", api.ErrBadRequest)
			}
			current, exists := e.state.Executions[execution.ID]
			// 扩展只补充内部控制，不能替换标准行中的身份和当前活动。
			if !exists || current.ProcessInstanceID != control.Instance.ID || current.DefinitionID != execution.DefinitionID || current.ActivityID != execution.ActivityID || current.ParentID != execution.ParentID {
				return api.NewError("ENGINE-0920", "执行扩展与标准行不一致", api.ErrBadRequest)
			}
			execution.Variables = current.Variables
			execution.Transient = nil
			e.state.Executions[execution.ID] = execution
			restored[execution.ID] = true
		}
		for _, job := range control.Jobs {
			if job == nil {
				return api.NewError("ENGINE-0920", "作业扩展不能为空", api.ErrBadRequest)
			}
			if _, exists := e.state.Jobs[job.ID]; exists {
				e.state.Jobs[job.ID] = job
			}
		}
		for _, subscription := range control.Subscriptions {
			if subscription == nil {
				return api.NewError("ENGINE-0920", "订阅扩展不能为空", api.ErrBadRequest)
			}
			if _, exists := e.state.Subscriptions[subscription.ID]; exists {
				e.state.Subscriptions[subscription.ID] = subscription
			}
		}
		e.state.Compensations[control.Instance.ID] = control.Compensations
	}
	for _, execution := range e.state.Executions {
		// 原始 Java 多实例控制没有映射时阻断，防止成功导入却无法继续执行。
		if execution.MultiInstanceRoot && !restored[execution.ID] {
			return api.NewError("ENGINE-0921", "多实例缺少可识别的续跑控制，需要迁移转换", api.ErrBadRequest)
		}
	}
	return nil
}
