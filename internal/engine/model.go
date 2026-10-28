package engine

import (
	"context"
	"sort"
	"strings"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/tenant"
)

// ListModels 返回全部流程模型。
// @param ctx 上下文
// @return 模型列表
// @return 查询错误
func (e *Engine) ListModels(ctx context.Context) ([]api.Model, error) {
	result, err := e.read(func() (any, error) {
		requested := e.tenantOf(tenant.From(ctx))
		out := make([]api.Model, 0, len(e.state.Models))
		for _, model := range e.state.Models {
			if requested != model.Info.TenantID {
				continue
			}
			out = append(out, model.Info)
		}
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]api.Model), nil
}

// GetModel 查询流程模型。
// @param ctx 上下文
// @param modelID 模型 ID
// @return 模型
// @return 查询错误
func (e *Engine) GetModel(ctx context.Context, modelID string) (api.Model, error) {
	result, err := e.read(func() (any, error) {
		model, ok := e.state.Models[modelID]
		if !ok {
			return nil, api.NewError("ENGINE-1001", "流程模型不存在", api.ErrNotFound)
		}
		if err := e.allowTenant(ctx, model.Info.TenantID); err != nil {
			return nil, err
		}
		return model.Info, nil
	})
	if err != nil {
		return api.Model{}, err
	}
	return result.(api.Model), nil
}

// CreateModel 创建流程模型。
// @param ctx 上下文
// @param req 创建请求
// @return 模型
// @return 创建错误
func (e *Engine) CreateModel(ctx context.Context, req api.CreateModelRequest) (api.Model, error) {
	result, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		name := strings.TrimSpace(req.Name)
		key := strings.TrimSpace(req.Key)
		if name == "" || key == "" {
			return nil, api.NewError("ENGINE-1002", "模型名称与 Key 不能为空", api.ErrBadRequest)
		}
		tenantID, err := e.requestTenantID(cc.ctx, req.TenantID)
		if err != nil {
			return nil, err
		}
		version := req.Version
		if version <= 0 {
			version = 1
		}
		model := &modelState{Info: api.Model{
			ID:             e.newID(),
			Name:           name,
			Key:            key,
			Category:       req.Category,
			Version:        version,
			MetaInfo:       req.MetaInfo,
			TenantID:       tenantID,
			CreateTime:     cc.now,
			LastUpdateTime: cc.now,
		}}
		e.state.Models[model.Info.ID] = model
		return model.Info, nil
	})
	if err != nil {
		return api.Model{}, err
	}
	return result.(api.Model), nil
}

// UpdateModel 更新流程模型。
// @param ctx 上下文
// @param modelID 模型 ID
// @param update 更新内容
// @return 更新后的模型
// @return 更新错误
func (e *Engine) UpdateModel(ctx context.Context, modelID string, update api.ModelUpdate) (api.Model, error) {
	result, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		model, ok := e.state.Models[modelID]
		if !ok {
			return nil, api.NewError("ENGINE-1001", "流程模型不存在", api.ErrNotFound)
		}
		if err := e.allowTenant(cc.ctx, model.Info.TenantID); err != nil {
			return nil, err
		}
		if update.Name != nil {
			model.Info.Name = *update.Name
		}
		if update.Key != nil {
			model.Info.Key = *update.Key
		}
		if update.Category != nil {
			model.Info.Category = *update.Category
		}
		if update.MetaInfo != nil {
			model.Info.MetaInfo = *update.MetaInfo
		}
		if update.DeploymentID != nil {
			model.Info.DeploymentID = *update.DeploymentID
		}
		if update.TenantID != nil {
			if err := e.allowTenant(cc.ctx, *update.TenantID); err != nil {
				return nil, err
			}
			model.Info.TenantID = *update.TenantID
		}
		if update.Version != nil {
			model.Info.Version = *update.Version
		}
		model.Info.LastUpdateTime = cc.now
		return model.Info, nil
	})
	if err != nil {
		return api.Model{}, err
	}
	return result.(api.Model), nil
}

// DeleteModel 删除流程模型。
// @param ctx 上下文
// @param modelID 模型 ID
// @return 删除错误
func (e *Engine) DeleteModel(ctx context.Context, modelID string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		model, ok := e.state.Models[modelID]
		if !ok {
			return nil, api.NewError("ENGINE-1001", "流程模型不存在", api.ErrNotFound)
		}
		if err := e.allowTenant(cc.ctx, model.Info.TenantID); err != nil {
			return nil, err
		}
		delete(e.state.Models, modelID)
		return nil, nil
	})
	return err
}

// GetModelSource 返回模型编辑源。
// @param ctx 上下文
// @param modelID 模型 ID
// @return 源内容
// @return 查询错误
func (e *Engine) GetModelSource(ctx context.Context, modelID string) ([]byte, error) {
	return e.readModelSource(ctx, modelID, false)
}

// SetModelSource 设置模型编辑源。
// @param ctx 上下文
// @param modelID 模型 ID
// @param source 源内容
// @return 设置错误
func (e *Engine) SetModelSource(ctx context.Context, modelID string, source []byte) error {
	return e.writeModelSource(ctx, modelID, source, false)
}

// GetModelSourceExtra 返回模型扩展源码。
// @param ctx 上下文
// @param modelID 模型 ID
// @return 扩展源
// @return 查询错误
func (e *Engine) GetModelSourceExtra(ctx context.Context, modelID string) ([]byte, error) {
	return e.readModelSource(ctx, modelID, true)
}

// SetModelSourceExtra 设置模型扩展源码。
// @param ctx 上下文
// @param modelID 模型 ID
// @param source 扩展源
// @return 设置错误
func (e *Engine) SetModelSourceExtra(ctx context.Context, modelID string, source []byte) error {
	return e.writeModelSource(ctx, modelID, source, true)
}

// readModelSource 读取模型源。
// @param ctx 上下文
// @param modelID 模型 ID
// @param extra 是否读取扩展源
// @return 源内容
// @return 查询错误
func (e *Engine) readModelSource(ctx context.Context, modelID string, extra bool) ([]byte, error) {
	result, err := e.read(func() (any, error) {
		model, ok := e.state.Models[modelID]
		if !ok {
			return nil, api.NewError("ENGINE-1001", "流程模型不存在", api.ErrNotFound)
		}
		if err := e.allowTenant(ctx, model.Info.TenantID); err != nil {
			return nil, err
		}
		content := model.Source
		if extra {
			content = model.SourceExtra
		}
		if content == nil {
			return nil, api.NewError("ENGINE-1003", "模型源不存在", api.ErrNotFound)
		}
		return append([]byte(nil), content...), nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]byte), nil
}

// writeModelSource 写入模型源。
// @param ctx 上下文
// @param modelID 模型 ID
// @param source 源内容
// @param extra 是否写入扩展源
// @return 设置错误
func (e *Engine) writeModelSource(ctx context.Context, modelID string, source []byte, extra bool) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		model, ok := e.state.Models[modelID]
		if !ok {
			return nil, api.NewError("ENGINE-1001", "流程模型不存在", api.ErrNotFound)
		}
		if err := e.allowTenant(cc.ctx, model.Info.TenantID); err != nil {
			return nil, err
		}
		copied := append([]byte(nil), source...)
		if extra {
			model.SourceExtra = copied
		} else {
			model.Source = copied
		}
		model.Info.LastUpdateTime = cc.now
		return nil, nil
	})
	return err
}
