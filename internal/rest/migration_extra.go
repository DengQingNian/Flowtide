package rest

import (
	"net/http"

	"github.com/flowable-go/flowable/api"
)

// definitionMigratePayload 是定义批量迁移请求。
type definitionMigratePayload struct {
	ProcessInstanceIDs []string          `json:"processInstanceIds"` // 实例 ID 列表
	ActivityMappings   map[string]string `json:"activityMappings"`   // 活动映射
}

// handleValidateMigration 校验流程实例迁移。
// @param w 响应
// @param r 请求
func (s *Server) handleValidateMigration(w http.ResponseWriter, r *http.Request) {
	var payload migratePayload
	if err := decodeTaskJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	if payload.TargetProcessDefinitionID == "" {
		writeError(w, api.NewError("REST-0133", "缺少 targetProcessDefinitionId", api.ErrBadRequest))
		return
	}
	messages, err := s.engine.RuntimeService().ValidateMigration(r.Context(), api.MigrationRequest{
		ProcessInstanceID:         r.PathValue("id"),
		TargetProcessDefinitionID: payload.TargetProcessDefinitionID,
		ActivityMappings:          payload.ActivityMappings,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"validationMessages": messages})
}

// handleDefinitionMigrate 将多个实例迁移到指定定义。
// @param w 响应
// @param r 请求
func (s *Server) handleDefinitionMigrate(w http.ResponseWriter, r *http.Request) {
	s.migrateInstances(w, r)
}

// handleDefinitionBatchMigrate 批量迁移实例并返回批处理作业。
// 与 Flowable 一致，成功时返回 201 与批次 JSON，失败时由批次分片记录具体原因。
// @param w 响应
// @param r 请求
func (s *Server) handleDefinitionBatchMigrate(w http.ResponseWriter, r *http.Request) {
	var payload definitionMigratePayload
	if err := decodeTaskJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	batch, err := s.engine.ManagementService().BatchMigrateProcessInstances(r.Context(), r.PathValue("id"),
		payload.ProcessInstanceIDs, payload.ActivityMappings)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, batch)
}

// migrateInstances 执行定义下的多实例迁移。
// @param w 响应
// @param r 请求
func (s *Server) migrateInstances(w http.ResponseWriter, r *http.Request) {
	var payload definitionMigratePayload
	if err := decodeTaskJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	if len(payload.ProcessInstanceIDs) == 0 {
		writeError(w, api.NewError("REST-0134", "processInstanceIds 不能为空", api.ErrBadRequest))
		return
	}
	targetID := r.PathValue("id")
	for _, instanceID := range payload.ProcessInstanceIDs {
		if err := s.engine.RuntimeService().MigrateProcessInstance(r.Context(), api.MigrationRequest{
			ProcessInstanceID:         instanceID,
			TargetProcessDefinitionID: targetID,
			ActivityMappings:          payload.ActivityMappings,
		}); err != nil {
			writeError(w, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
