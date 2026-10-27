package engine

import (
	"context"
	"sort"

	"github.com/flowable-go/flowable/api"
)

// ListDeployments 返回全部部署。
// @param ctx 上下文
// @return 部署列表
// @return 查询错误
func (e *Engine) ListDeployments(ctx context.Context) ([]api.Deployment, error) {
	result, err := e.read(func() (any, error) {
		out := make([]api.Deployment, 0, len(e.state.Deployments))
		for _, deployment := range e.state.Deployments {
			out = append(out, deployment.Info)
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].DeployTime.Equal(out[j].DeployTime) {
				return out[i].ID < out[j].ID
			}
			return out[i].DeployTime.Before(out[j].DeployTime)
		})
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]api.Deployment), nil
}

// GetDeployment 查询部署。
// @param ctx 上下文
// @param deploymentID 部署 ID
// @return 部署信息
// @return 查询错误
func (e *Engine) GetDeployment(ctx context.Context, deploymentID string) (api.Deployment, error) {
	result, err := e.read(func() (any, error) {
		deployment, ok := e.state.Deployments[deploymentID]
		if !ok {
			return nil, api.NewError("ENGINE-0404", "部署不存在", api.ErrNotFound)
		}
		if err := e.allowTenant(ctx, deployment.Info.TenantID); err != nil {
			return nil, err
		}
		return deployment.Info, nil
	})
	if err != nil {
		return api.Deployment{}, err
	}
	return result.(api.Deployment), nil
}

// ListDeploymentResources 返回部署资源名列表。
// @param ctx 上下文
// @param deploymentID 部署 ID
// @return 资源名列表
// @return 查询错误
func (e *Engine) ListDeploymentResources(ctx context.Context, deploymentID string) ([]string, error) {
	result, err := e.read(func() (any, error) {
		deployment, ok := e.state.Deployments[deploymentID]
		if !ok {
			return nil, api.NewError("ENGINE-0404", "部署不存在", api.ErrNotFound)
		}
		if err := e.allowTenant(ctx, deployment.Info.TenantID); err != nil {
			return nil, err
		}
		names := make([]string, 0, len(deployment.Resources))
		for name := range deployment.Resources {
			names = append(names, name)
		}
		sort.Strings(names)
		return names, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]string), nil
}

// GetDeploymentResource 返回部署资源内容。
// @param ctx 上下文
// @param deploymentID 部署 ID
// @param resourceName 资源名
// @return 资源内容
// @return 查询错误
func (e *Engine) GetDeploymentResource(ctx context.Context, deploymentID, resourceName string) ([]byte, error) {
	result, err := e.read(func() (any, error) {
		deployment, ok := e.state.Deployments[deploymentID]
		if !ok {
			return nil, api.NewError("ENGINE-0404", "部署不存在", api.ErrNotFound)
		}
		if err := e.allowTenant(ctx, deployment.Info.TenantID); err != nil {
			return nil, err
		}
		data, exists := deployment.Resources[resourceName]
		if !exists {
			return nil, api.NewError("ENGINE-0404", "部署资源不存在", api.ErrNotFound)
		}
		return append([]byte(nil), data...), nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]byte), nil
}

// DeleteDeployment 删除部署。
// @param ctx 上下文
// @param deploymentID 部署 ID
// @param cascade 是否级联删除流程定义
// @return 删除错误
func (e *Engine) DeleteDeployment(ctx context.Context, deploymentID string, cascade bool) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		deployment, ok := e.state.Deployments[deploymentID]
		if !ok {
			return nil, api.NewError("ENGINE-0404", "部署不存在", api.ErrNotFound)
		}
		if err := e.allowTenant(ctx, deployment.Info.TenantID); err != nil {
			return nil, err
		}
		definitions := make([]*definitionState, 0)
		for _, definition := range e.state.Definitions {
			if definition.Info.DeploymentID == deploymentID {
				definitions = append(definitions, definition)
			}
		}
		if len(definitions) > 0 && !cascade {
			return nil, api.NewError("ENGINE-0660", "部署仍包含流程定义，需要级联删除", api.ErrConflict)
		}
		for _, definition := range definitions {
			for _, instance := range e.state.Instances {
				if instance.ProcessDefinitionID == definition.Info.ID && !instance.Ended {
					return nil, api.NewError("ENGINE-0661", "部署仍存在运行中实例，不能删除", api.ErrConflict)
				}
			}
			delete(e.state.Definitions, definition.Info.ID)
			for jobID, job := range e.state.Jobs {
				if job.ProcessDefinitionID == definition.Info.ID {
					delete(e.state.Jobs, jobID)
				}
			}
			for subscriptionID, subscription := range e.state.Subscriptions {
				if subscription.ProcessDefinitionID == definition.Info.ID {
					delete(e.state.Subscriptions, subscriptionID)
				}
			}
			delete(e.state.Versions, definition.Info.Key)
		}
		for key := range e.state.Versions {
			maxVersion := 0
			for _, definition := range e.state.Definitions {
				if definition.Info.Key == key && definition.Info.Version > maxVersion {
					maxVersion = definition.Info.Version
				}
			}
			if maxVersion == 0 {
				delete(e.state.Versions, key)
			} else {
				e.state.Versions[key] = maxVersion
			}
		}
		delete(e.state.Deployments, deploymentID)
		_ = deployment
		return nil, nil
	})
	return err
}
