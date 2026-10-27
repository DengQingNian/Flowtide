package engine

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/bpmn/model"
	bpmnxml "github.com/flowable-go/flowable/internal/bpmn/xml"
)

// Deploy 部署 BPMN 资源。
// @param ctx 上下文
// @param req 部署请求
// @return 部署信息
// @return 部署错误
func (e *Engine) Deploy(ctx context.Context, req api.DeploymentRequest) (api.Deployment, error) {
	result, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		return e.deploy(cc, req)
	})
	if err != nil {
		return api.Deployment{}, err
	}
	return result.(api.Deployment), nil
}

// deploy 在命令内执行部署。
// @param cc 命令上下文
// @param req 部署请求
// @return 部署信息
// @return 部署错误
func (e *Engine) deploy(cc *commandContext, req api.DeploymentRequest) (api.Deployment, error) {
	if len(req.Resources) == 0 {
		return api.Deployment{}, api.NewError("ENGINE-0201", "部署资源不能为空", api.ErrBadRequest)
	}
	deploymentID := e.newID()
	info := api.Deployment{
		ID:         deploymentID,
		Name:       req.Name,
		TenantID:   e.tenantOf(req.TenantID),
		DeployTime: cc.now,
	}
	resources := make(map[string][]byte, len(req.Resources))
	names := make([]string, 0, len(req.Resources))
	for name := range req.Resources {
		names = append(names, name)
	}
	sort.Strings(names)
	// 版本号在同一部署内按 Key 递增
	localVersions := make(map[string]int)
	for _, name := range names {
		data := req.Resources[name]
		resources[name] = append([]byte(nil), data...)
		if !isBPMNResource(name) {
			continue
		}
		bpmnModel, err := bpmnxml.Parse(data)
		if err != nil {
			return api.Deployment{}, err
		}
		for _, process := range bpmnModel.Processes {
			if process.ID == "" {
				return api.Deployment{}, api.NewError("ENGINE-0202", "流程缺少 ID", nil)
			}
			version := e.state.Versions[process.ID] + 1 + localVersions[process.ID]
			localVersions[process.ID]++
			definitionID := fmt.Sprintf("%s:%d:%s", process.ID, version, e.newID())
			definition := &definitionState{
				Info: api.ProcessDefinition{
					ID:           definitionID,
					Key:          process.ID,
					Name:         process.Name,
					Version:      version,
					DeploymentID: deploymentID,
					ResourceName: name,
					TenantID:     info.TenantID,
				},
				Model: bpmnModel,
				XML:   append([]byte(nil), data...),
			}
			e.state.Definitions[definitionID] = definition
			e.createStartSubscriptions(cc, definition, process)
		}
	}
	for key, count := range localVersions {
		e.state.Versions[key] += count
	}
	e.state.Deployments[deploymentID] = &deploymentState{Info: info, Resources: resources}
	return info, nil
}

// isBPMNResource 判断资源是否为 BPMN 文件。
// @param name 资源名
// @return 是否为 BPMN
func isBPMNResource(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".bpmn20.xml") || strings.HasSuffix(lower, ".bpmn")
}

// ListProcessDefinitions 返回流程定义列表。
// @param ctx 上下文
// @return 流程定义列表
// @return 查询错误
func (e *Engine) ListProcessDefinitions(ctx context.Context) ([]api.ProcessDefinition, error) {
	result, err := e.read(func() (any, error) {
		out := make([]api.ProcessDefinition, 0, len(e.state.Definitions))
		for _, definition := range e.state.Definitions {
			out = append(out, definition.Info)
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Key == out[j].Key {
				return out[i].Version < out[j].Version
			}
			return out[i].Key < out[j].Key
		})
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]api.ProcessDefinition), nil
}

// GetProcessDefinition 按 ID 查询流程定义。
// @param ctx 上下文
// @param id 流程定义 ID
// @return 流程定义
// @return 查询错误
func (e *Engine) GetProcessDefinition(ctx context.Context, id string) (api.ProcessDefinition, error) {
	result, err := e.read(func() (any, error) {
		definition, ok := e.findDefinition(id)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "流程定义不存在", api.ErrNotFound)
		}
		return definition.Info, nil
	})
	if err != nil {
		return api.ProcessDefinition{}, err
	}
	return result.(api.ProcessDefinition), nil
}

// GetProcessDefinitionByKey 按 Key 查询最新流程定义。
// @param ctx 上下文
// @param key 流程定义 Key
// @return 流程定义
// @return 查询错误
func (e *Engine) GetProcessDefinitionByKey(ctx context.Context, key string) (api.ProcessDefinition, error) {
	result, err := e.read(func() (any, error) {
		definition, ok := e.findLatestDefinition(key)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "流程定义不存在", api.ErrNotFound)
		}
		return definition.Info, nil
	})
	if err != nil {
		return api.ProcessDefinition{}, err
	}
	return result.(api.ProcessDefinition), nil
}

// GetBpmnModel 返回流程定义模型。
// @param ctx 上下文
// @param definitionID 流程定义 ID
// @return BPMN 模型
// @return 查询错误
func (e *Engine) GetBpmnModel(ctx context.Context, definitionID string) (*model.BpmnModel, error) {
	result, err := e.read(func() (any, error) {
		definition, ok := e.findDefinition(definitionID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "流程定义不存在", api.ErrNotFound)
		}
		return definition.Model, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(*model.BpmnModel), nil
}

// GetProcessDefinitionXML 返回流程定义原始 XML。
// @param ctx 上下文
// @param definitionID 流程定义 ID
// @return XML 内容
// @return 查询错误
func (e *Engine) GetProcessDefinitionXML(ctx context.Context, definitionID string) ([]byte, error) {
	result, err := e.read(func() (any, error) {
		definition, ok := e.findDefinition(definitionID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "流程定义不存在", api.ErrNotFound)
		}
		return append([]byte(nil), definition.XML...), nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]byte), nil
}
