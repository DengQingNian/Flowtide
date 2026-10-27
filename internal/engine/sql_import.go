package engine

import (
	"context"

	"github.com/flowable-go/flowable/api"
	bpmnxml "github.com/flowable-go/flowable/internal/bpmn/xml"
	flowablerow "github.com/flowable-go/flowable/internal/persistence/flowable"
)

// ImportRows 从 ACT 行恢复部署与流程定义。
// @param ctx 上下文
// @param rows 行列表
// @return 导入错误
func (e *Engine) ImportRows(ctx context.Context, rows []flowablerow.Row) error {
	snapshot, err := flowablerow.DecodeRows(rows)
	if err != nil {
		return err
	}
	_, err = e.execute(ctx, func(cc *commandContext) (any, error) {
		for _, deployment := range snapshot.Deployments {
			e.state.Deployments[deployment.ID] = &deploymentState{Info: deployment, Resources: snapshot.Resources[deployment.ID]}
		}
		for _, definition := range snapshot.Definitions {
			data := snapshot.Resources[definition.DeploymentID][definition.ResourceName]
			parsed, parseErr := bpmnxml.Parse(data)
			if parseErr != nil {
				return nil, api.NewError("ENGINE-0912", "导入流程定义失败: "+definition.ID, parseErr)
			}
			deleteStartArtifacts(e.state, definition.ID)
			e.state.Definitions[definition.ID] = &definitionState{Info: definition, Model: parsed, XML: data}
			process := processOfDefinition(e.state.Definitions[definition.ID])
			if process == nil {
				return nil, api.NewError("ENGINE-0501", "流程定义不包含流程", nil)
			}
			e.createStartSubscriptions(cc, e.state.Definitions[definition.ID], process)
			if timerErr := e.scheduleStartTimers(cc, e.state.Definitions[definition.ID], process); timerErr != nil {
				return nil, timerErr
			}
			if current := e.state.Versions[definition.Key]; definition.Version > current {
				e.state.Versions[definition.Key] = definition.Version
			}
		}
		return nil, nil
	})
	return err
}

// deleteStartArtifacts 清理定义关联的启动订阅与定时作业。
// @param state 引擎状态
// @param definitionID 流程定义 ID
func deleteStartArtifacts(state *state, definitionID string) {
	for id, subscription := range state.Subscriptions {
		if subscription.ProcessDefinitionID == definitionID {
			delete(state.Subscriptions, id)
		}
	}
	for id, job := range state.Jobs {
		if job.ProcessDefinitionID == definitionID {
			delete(state.Jobs, id)
		}
	}
}
