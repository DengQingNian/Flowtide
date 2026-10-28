package engine

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/flowable-go/flowable/api"
)

// 批处理类型与状态常量。
const (
	// BatchTypeProcessInstanceMigration 表示流程实例迁移批次。
	BatchTypeProcessInstanceMigration = "process-instance-migration"
	// BatchStatusInProgress 表示批次执行中。
	BatchStatusInProgress = "in-progress"
	// BatchStatusCompleted 表示批次全部成功。
	BatchStatusCompleted = "completed"
	// BatchStatusFailed 表示批次存在失败分片。
	BatchStatusFailed = "failed"
	// BatchPartStatusQueued 表示分片等待执行。
	BatchPartStatusQueued = "queued"
	// BatchPartStatusCompleted 表示分片执行成功。
	BatchPartStatusCompleted = "completed"
	// BatchPartStatusFailed 表示分片执行失败。
	BatchPartStatusFailed = "failed"
)

// ListBatches 返回全部批处理作业。
// 按开始时间倒序、ID 升序稳定排序，保证相同开始时间下结果确定。
// @param ctx 上下文
// @return 批次列表
// @return 查询错误
func (e *Engine) ListBatches(ctx context.Context) ([]api.Batch, error) {
	result, err := e.read(func() (any, error) {
		out := make([]api.Batch, 0, len(e.state.Batches))
		for _, batch := range e.state.Batches {
			if e.allowTenant(ctx, batch.Info.TenantID) != nil {
				continue
			}
			out = append(out, cloneBatchInfo(batch.Info))
		}
		sort.SliceStable(out, func(i, j int) bool {
			if !out[i].StartTime.Equal(out[j].StartTime) {
				return out[i].StartTime.After(out[j].StartTime)
			}
			return out[i].ID < out[j].ID
		})
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]api.Batch), nil
}

// GetBatch 查询批处理作业。
// @param ctx 上下文
// @param batchID 批次 ID
// @return 批次
// @return 查询错误
func (e *Engine) GetBatch(ctx context.Context, batchID string) (api.Batch, error) {
	result, err := e.read(func() (any, error) {
		batch, ok := e.state.Batches[batchID]
		if !ok {
			return nil, api.NewError("ENGINE-0700", "批处理作业不存在", api.ErrNotFound)
		}
		if err := e.allowTenant(ctx, batch.Info.TenantID); err != nil {
			return nil, err
		}
		return cloneBatchInfo(batch.Info), nil
	})
	if err != nil {
		return api.Batch{}, err
	}
	return result.(api.Batch), nil
}

// ListBatchParts 返回批次的分片列表。
// 按开始时间升序、ID 升序稳定排序。
// @param ctx 上下文
// @param batchID 批次 ID
// @return 分片列表
// @return 查询错误
func (e *Engine) ListBatchParts(ctx context.Context, batchID string) ([]api.BatchPart, error) {
	result, err := e.read(func() (any, error) {
		batch, ok := e.state.Batches[batchID]
		if !ok {
			return nil, api.NewError("ENGINE-0700", "批处理作业不存在", api.ErrNotFound)
		}
		if err := e.allowTenant(ctx, batch.Info.TenantID); err != nil {
			return nil, err
		}
		out := make([]api.BatchPart, 0, len(batch.Parts))
		for _, part := range batch.Parts {
			out = append(out, cloneBatchPart(part))
		}
		sort.SliceStable(out, func(i, j int) bool {
			if !out[i].StartTime.Equal(out[j].StartTime) {
				return out[i].StartTime.Before(out[j].StartTime)
			}
			return out[i].ID < out[j].ID
		})
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]api.BatchPart), nil
}

// GetBatchPart 查询批处理分片。
// @param ctx 上下文
// @param partID 分片 ID
// @return 分片
// @return 查询错误
func (e *Engine) GetBatchPart(ctx context.Context, partID string) (api.BatchPart, error) {
	result, err := e.read(func() (any, error) {
		for _, batch := range e.state.Batches {
			if e.allowTenant(ctx, batch.Info.TenantID) != nil {
				continue
			}
			if part, ok := batch.Parts[partID]; ok {
				return cloneBatchPart(part), nil
			}
		}
		return nil, api.NewError("ENGINE-0701", "批处理分片不存在", api.ErrNotFound)
	})
	if err != nil {
		return api.BatchPart{}, err
	}
	return result.(api.BatchPart), nil
}

// BatchMigrateProcessInstances 批量迁移流程实例并记录批次与分片。
// 与 Flowable 一致，批次与分片会持久化到批次表；本实现仅写入内存快照。
// @param ctx 上下文
// @param targetDefinitionID 目标流程定义 ID
// @param instanceIDs 流程实例 ID 列表
// @param activityMappings 源活动 ID 到目标活动 ID
// @return 最终批次
// @return 迁移错误
func (e *Engine) BatchMigrateProcessInstances(ctx context.Context, targetDefinitionID string, instanceIDs []string, activityMappings map[string]string) (api.Batch, error) {
	if len(instanceIDs) == 0 {
		return api.Batch{}, api.NewError("ENGINE-0702", "流程实例列表不能为空", api.ErrBadRequest)
	}
	var tenantID string
	// 先只读校验目标定义，避免创建批次后才发现目标不存在
	if _, err := e.read(func() (any, error) {
		definition, ok := e.findDefinition(targetDefinitionID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "目标流程定义不存在", api.ErrNotFound)
		}
		if err := e.allowTenant(ctx, definition.Info.TenantID); err != nil {
			return nil, err
		}
		tenantID = definition.Info.TenantID
		return nil, nil
	}); err != nil {
		return api.Batch{}, err
	}

	partIDs := make([]string, 0, len(instanceIDs))
	// 第一步：在单个事务内创建批次和全部分片，保证批次与分片同时可见
	result, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		batch := &api.Batch{
			ID:        e.newID(),
			Type:      BatchTypeProcessInstanceMigration,
			Total:     len(instanceIDs),
			Status:    BatchStatusInProgress,
			StartTime: cc.now,
			TenantID:  tenantID,
		}
		created := &batchState{Info: *batch, Parts: make(map[string]*api.BatchPart, len(instanceIDs))}
		document, marshalErr := json.Marshal(batchDocument{
			ID:                        batch.ID,
			Type:                      batch.Type,
			Total:                     batch.Total,
			Status:                    batch.Status,
			TargetProcessDefinitionID: targetDefinitionID,
			ProcessInstanceIDs:        instanceIDs,
			StartTime:                 batch.StartTime,
			TenantID:                  tenantID,
		})
		// 文档序列化失败说明内部数据结构异常，直接中止批次创建
		if marshalErr != nil {
			return nil, api.NewError("ENGINE-0703", "批次文档序列化失败", marshalErr)
		}
		batch.Document = document
		created.Info.Document = document
		for _, instanceID := range instanceIDs {
			part := &api.BatchPart{
				ID:                e.newID(),
				BatchID:           batch.ID,
				Type:              BatchTypeProcessInstanceMigration,
				ProcessInstanceID: instanceID,
				Status:            BatchPartStatusQueued,
				StartTime:         cc.now,
			}
			partDocument, partErr := json.Marshal(batchPartDocument{
				ID:                part.ID,
				BatchID:           part.BatchID,
				Type:              part.Type,
				ProcessInstanceID: part.ProcessInstanceID,
				Status:            part.Status,
				StartTime:         part.StartTime,
			})
			// 分片文档与批次文档使用同一序列化路径，失败视为内部错误
			if partErr != nil {
				return nil, api.NewError("ENGINE-0704", "分片文档序列化失败", partErr)
			}
			part.Document = partDocument
			created.Parts[part.ID] = part
			partIDs = append(partIDs, part.ID)
		}
		e.state.Batches[batch.ID] = created
		return *batch, nil
	})
	if err != nil {
		return api.Batch{}, err
	}
	batch := result.(api.Batch)

	// 第二步：逐实例同步迁移，失败只标记当前分片，不影响其余实例
	completed := 0
	for index, instanceID := range instanceIDs {
		status := BatchPartStatusCompleted
		message := ""
		if migrateErr := e.MigrateProcessInstance(ctx, api.MigrationRequest{
			ProcessInstanceID:         instanceID,
			TargetProcessDefinitionID: targetDefinitionID,
			ActivityMappings:          activityMappings,
		}); migrateErr != nil {
			status = BatchPartStatusFailed
			message = migrateErr.Error()
		} else {
			completed++
		}
		if updateErr := e.updateBatchPart(ctx, batch.ID, partIDs[index], status, message); updateErr != nil {
			return api.Batch{}, updateErr
		}
	}

	// 第三步：写入完成数、批次状态与结束时间
	final, err := e.finishBatch(ctx, batch.ID, completed)
	if err != nil {
		return api.Batch{}, err
	}
	return final, nil
}

// updateBatchPart 更新单个分片状态与结束时间。
// @param ctx 上下文
// @param batchID 批次 ID
// @param partID 分片 ID
// @param status 新状态
// @param message 失败原因
// @return 更新错误
func (e *Engine) updateBatchPart(ctx context.Context, batchID, partID, status, message string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		batch, ok := e.state.Batches[batchID]
		if !ok {
			return nil, api.NewError("ENGINE-0700", "批处理作业不存在", api.ErrNotFound)
		}
		part, ok := batch.Parts[partID]
		if !ok {
			return nil, api.NewError("ENGINE-0701", "批处理分片不存在", api.ErrNotFound)
		}
		part.Status = status
		part.EndTime = cloneTime(&cc.now)
		part.ErrorMessage = message
		return nil, nil
	})
	return err
}

// finishBatch 汇总分片结果并结束批次。
// @param ctx 上下文
// @param batchID 批次 ID
// @param completed 成功分片数
// @return 最终批次
// @return 更新错误
func (e *Engine) finishBatch(ctx context.Context, batchID string, completed int) (api.Batch, error) {
	result, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		batch, ok := e.state.Batches[batchID]
		if !ok {
			return nil, api.NewError("ENGINE-0700", "批处理作业不存在", api.ErrNotFound)
		}
		batch.Info.Completed = completed
		// 存在失败分片时批次整体标记为失败，便于运维快速识别
		if completed == batch.Info.Total {
			batch.Info.Status = BatchStatusCompleted
		} else {
			batch.Info.Status = BatchStatusFailed
		}
		batch.Info.EndTime = cloneTime(&cc.now)
		if err := e.allowTenant(ctx, batch.Info.TenantID); err != nil {
			return nil, err
		}
		return cloneBatchInfo(batch.Info), nil
	})
	if err != nil {
		return api.Batch{}, err
	}
	return result.(api.Batch), nil
}

// cloneBatchPart 深拷贝批处理分片。
// @param source 源分片
// @return 拷贝后的分片
func cloneBatchPart(source *api.BatchPart) api.BatchPart {
	copied := *source
	copied.EndTime = cloneTime(source.EndTime)
	// 文档是可变字节切片，必须复制，避免读接口修改内部快照
	copied.Document = append([]byte(nil), source.Document...)
	return copied
}

// batchDocument 是批次文档的 JSON 结构。
type batchDocument struct {
	ID                        string    `json:"id"`                        // 批次 ID
	Type                      string    `json:"type"`                      // 批次类型
	Total                     int       `json:"total"`                     // 分片总数
	Status                    string    `json:"status"`                    // 创建时状态
	TargetProcessDefinitionID string    `json:"targetProcessDefinitionId"` // 目标流程定义
	ProcessInstanceIDs        []string  `json:"processInstanceIds"`        // 待迁移实例
	StartTime                 time.Time `json:"startTime"`                 // 开始时间
	TenantID                  string    `json:"tenantId,omitempty"`        // 租户
}

// batchPartDocument 是批次分片文档的 JSON 结构。
type batchPartDocument struct {
	ID                string    `json:"id"`                          // 分片 ID
	BatchID           string    `json:"batchId"`                     // 所属批次 ID
	Type              string    `json:"type"`                        // 分片类型
	ProcessInstanceID string    `json:"processInstanceId,omitempty"` // 流程实例 ID
	Status            string    `json:"status"`                      // 创建时状态
	StartTime         time.Time `json:"startTime"`                   // 开始时间
}

// DeleteBatch 删除批次及其全部分片。
// @param ctx 上下文
// @param batchID 批次 ID
// @return 删除错误
func (e *Engine) DeleteBatch(ctx context.Context, batchID string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		batch, ok := e.state.Batches[batchID]
		if !ok {
			return nil, api.NewError("ENGINE-0700", "批处理作业不存在", api.ErrNotFound)
		}
		if err := e.allowTenant(ctx, batch.Info.TenantID); err != nil {
			return nil, err
		}
		// 删除批次时连同分片一起移除，避免残留不可达的分片
		delete(e.state.Batches, batchID)
		return nil, nil
	})
	return err
}

// GetBatchDocument 返回批次文档。
// @param ctx 上下文
// @param batchID 批次 ID
// @return 文档字节
// @return 查询错误
func (e *Engine) GetBatchDocument(ctx context.Context, batchID string) ([]byte, error) {
	result, err := e.read(func() (any, error) {
		batch, ok := e.state.Batches[batchID]
		if !ok {
			return nil, api.NewError("ENGINE-0700", "批处理作业不存在", api.ErrNotFound)
		}
		if err := e.allowTenant(ctx, batch.Info.TenantID); err != nil {
			return nil, err
		}
		// 旧快照或未写入文档的批次按 Flowable 语义视为不存在
		if len(batch.Info.Document) == 0 {
			return nil, api.NewError("ENGINE-0705", "批次没有文档", api.ErrNotFound)
		}
		return append([]byte(nil), batch.Info.Document...), nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]byte), nil
}

// GetBatchPartDocument 返回批次分片文档。
// @param ctx 上下文
// @param partID 分片 ID
// @return 文档字节
// @return 查询错误
func (e *Engine) GetBatchPartDocument(ctx context.Context, partID string) ([]byte, error) {
	result, err := e.read(func() (any, error) {
		for _, batch := range e.state.Batches {
			if e.allowTenant(ctx, batch.Info.TenantID) != nil {
				continue
			}
			part, ok := batch.Parts[partID]
			if !ok {
				continue
			}
			if len(part.Document) == 0 {
				return nil, api.NewError("ENGINE-0706", "批次分片没有文档", api.ErrNotFound)
			}
			return append([]byte(nil), part.Document...), nil
		}
		return nil, api.NewError("ENGINE-0701", "批处理分片不存在", api.ErrNotFound)
	})
	if err != nil {
		return nil, err
	}
	return result.([]byte), nil
}
