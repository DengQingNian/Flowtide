package rest

import (
	"net/http"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/bpmn/model"
)

// definitionModelDTO 是流程定义模型的 REST 表示。
type definitionModelDTO struct {
	ID        string                 `json:"id"`        // 流程定义 ID
	Name      string                 `json:"name"`      // 流程定义名称
	Processes []definitionProcessDTO `json:"processes"` // 流程列表
	Messages  []definitionMessageDTO `json:"messages"`  // 全局消息
	Signals   []definitionSignalDTO  `json:"signals"`   // 全局信号
	Errors    []definitionErrorDTO   `json:"errors"`    // 全局错误
}

// definitionProcessDTO 是 BPMN 流程的 REST 表示。
type definitionProcessDTO struct {
	ID            string                     `json:"id"`            // 流程 ID
	Name          string                     `json:"name"`          // 流程名称
	Documentation string                     `json:"documentation"` // 文档说明
	Executable    bool                       `json:"executable"`    // 是否可执行
	FlowElements  []definitionFlowElementDTO `json:"flowElements"`  // 流程元素，按文档顺序
}

// definitionFlowElementDTO 是 BPMN 流程元素的 REST 表示。
type definitionFlowElementDTO struct {
	ID              string   `json:"id"`                        // 元素 ID
	Name            string   `json:"name"`                      // 元素名称
	Type            string   `json:"type"`                      // 元素类型
	SourceRef       string   `json:"sourceRef,omitempty"`       // 顺序流源节点
	TargetRef       string   `json:"targetRef,omitempty"`       // 顺序流目标节点
	Assignee        string   `json:"assignee,omitempty"`        // 用户任务处理人
	FormKey         string   `json:"formKey,omitempty"`         // 用户任务表单 Key
	CandidateUsers  []string `json:"candidateUsers,omitempty"`  // 用户任务候选用户
	CandidateGroups []string `json:"candidateGroups,omitempty"` // 用户任务候选组
}

// definitionMessageDTO 是全局消息的 REST 表示。
type definitionMessageDTO struct {
	ID   string `json:"id"`   // 消息 ID
	Name string `json:"name"` // 消息名称
}

// definitionSignalDTO 是全局信号的 REST 表示。
type definitionSignalDTO struct {
	ID   string `json:"id"`   // 信号 ID
	Name string `json:"name"` // 信号名称
}

// definitionErrorDTO 是全局错误的 REST 表示。
type definitionErrorDTO struct {
	ID        string `json:"id"`        // 错误 ID
	ErrorCode string `json:"errorCode"` // 错误码
}

// handleDefinitionModel 返回流程定义模型。
// @param w 响应
// @param r 请求
func (s *Server) handleDefinitionModel(w http.ResponseWriter, r *http.Request) {
	definition, err := s.engine.RepositoryService().GetProcessDefinition(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	bpmnModel, err := s.engine.RepositoryService().GetBpmnModel(r.Context(), definition.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, buildDefinitionModelDTO(definition, bpmnModel))
}

// handleDefinitionDecisions 返回流程定义关联的决策。
// @param w 响应
// @param r 请求
// 说明：当前引擎尚未集成 DMN 决策引擎，校验流程定义后返回空数组。
func (s *Server) handleDefinitionDecisions(w http.ResponseWriter, r *http.Request) {
	if _, err := s.engine.RepositoryService().GetProcessDefinition(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, []any{})
}

// handleDefinitionDecisionTables 返回流程定义关联的决策表。
// @param w 响应
// @param r 请求
// 说明：当前引擎尚未集成 DMN 决策引擎，校验流程定义后返回空数组。
func (s *Server) handleDefinitionDecisionTables(w http.ResponseWriter, r *http.Request) {
	if _, err := s.engine.RepositoryService().GetProcessDefinition(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, []any{})
}

// handleDefinitionFormDefinitions 返回流程定义关联的表单定义。
// @param w 响应
// @param r 请求
// 说明：当前引擎尚未集成表单引擎，校验流程定义后返回空数组。
func (s *Server) handleDefinitionFormDefinitions(w http.ResponseWriter, r *http.Request) {
	if _, err := s.engine.RepositoryService().GetProcessDefinition(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, []any{})
}

// buildDefinitionModelDTO 将 BPMN 模型转换为 REST DTO。
// @param definition 流程定义
// @param bpmnModel BPMN 模型
// @return 流程定义模型 DTO
func buildDefinitionModelDTO(definition api.ProcessDefinition, bpmnModel *model.BpmnModel) definitionModelDTO {
	dto := definitionModelDTO{
		ID:        definition.ID,
		Name:      definition.Name,
		Processes: make([]definitionProcessDTO, 0),
		Messages:  make([]definitionMessageDTO, 0),
		Signals:   make([]definitionSignalDTO, 0),
		Errors:    make([]definitionErrorDTO, 0),
	}
	if bpmnModel == nil {
		return dto
	}
	for _, process := range bpmnModel.Processes {
		// 跳过空流程，避免异常模型导致 panic
		if process == nil {
			continue
		}
		dto.Processes = append(dto.Processes, buildDefinitionProcessDTO(process))
	}
	for _, message := range bpmnModel.Messages {
		// 跳过空消息定义
		if message == nil {
			continue
		}
		dto.Messages = append(dto.Messages, definitionMessageDTO{ID: message.ID, Name: message.Name})
	}
	for _, signal := range bpmnModel.Signals {
		// 跳过空信号定义
		if signal == nil {
			continue
		}
		dto.Signals = append(dto.Signals, definitionSignalDTO{ID: signal.ID, Name: signal.Name})
	}
	for _, bpmnError := range bpmnModel.Errors {
		// 跳过空错误定义
		if bpmnError == nil {
			continue
		}
		dto.Errors = append(dto.Errors, definitionErrorDTO{ID: bpmnError.ID, ErrorCode: bpmnError.ErrorCode})
	}
	return dto
}

// buildDefinitionProcessDTO 将 BPMN 流程转换为 REST DTO。
// @param process BPMN 流程
// @return 流程 DTO
func buildDefinitionProcessDTO(process *model.Process) definitionProcessDTO {
	dto := definitionProcessDTO{
		ID:            process.ID,
		Name:          process.Name,
		Documentation: process.Documentation,
		Executable:    process.Executable,
		FlowElements:  make([]definitionFlowElementDTO, 0, len(process.Order)),
	}
	// 按模型记录的文档顺序输出元素，缺失的元素直接跳过
	for _, id := range process.Order {
		element, ok := process.Elements[id]
		if !ok || element == nil {
			continue
		}
		dto.FlowElements = append(dto.FlowElements, buildDefinitionFlowElementDTO(element))
	}
	return dto
}

// buildDefinitionFlowElementDTO 将 BPMN 元素转换为 REST DTO。
// @param element BPMN 元素
// @return 流程元素 DTO
func buildDefinitionFlowElementDTO(element model.FlowElement) definitionFlowElementDTO {
	dto := definitionFlowElementDTO{
		ID:   element.GetID(),
		Name: element.GetName(),
		Type: element.ElementType(),
	}
	switch typed := element.(type) {
	case *model.SequenceFlow:
		dto.SourceRef = typed.SourceRef
		dto.TargetRef = typed.TargetRef
	case *model.UserTask:
		dto.Assignee = typed.Assignee
		dto.FormKey = typed.FormKey
		dto.CandidateUsers = typed.CandidateUsers
		dto.CandidateGroups = typed.CandidateGroups
	}
	return dto
}
