package workflow

import (
	"context"
	"strings"

	"github.com/DengQingNian/Flowtide/bpmn/model"
)

// 会签监听器类名（对位 Java checkCountersignListenerConfig:316 校验的全限定类名后缀）：
// 线上语料以 Java 全限定类名声明监听器，Go 侧注册名以短名声明，两者均视为命中。
const (
	// javaCollectionConfigListenerSuffix Java 集合配置监听器全限定类名后缀。
	javaCollectionConfigListenerSuffix = "CountersignCollectionConfigExecutionListener"
	// javaWeightListenerSuffix Java 权重会签监听器全限定类名后缀。
	javaWeightListenerSuffix = "CountersignByWeightExecutionListener"
)

// ValidateCountersignConfig 部署强校验：会签节点必须配置集合变量设置监听器（P3.6，
// 对位 Java ProcessesUtil#checkCountersignListenerConfig:316）。
// 凡配置了多实例（MultiInstance!=nil）的用户任务节点：
//   - weightType=average 需含开始执行监听器 countersignCollectionConfig（或 Java 全限定类名）；
//   - weightType=custom 需含开始执行监听器 countersignByWeight（或 Java 全限定类名），
//     且 weightThreshold 非空（缺省报"未设置会签权重阈值"）；
//   - weightType 缺失按 Java 语义同样报未配置监听器（Java weightType=null 时命中标记不置位）。
//
// @param ctx 操作上下文
// @param definitionID 流程定义 ID
// @return 错误（首个不合规节点）
func (s *CountersignService) ValidateCountersignConfig(ctx context.Context, definitionID string) error {
	return ValidateCountersignConfigWith(ctx, s.meta, definitionID)
}

// ValidateCountersignConfigWith 部署强校验（显式注入模型元数据服务，供复用既有服务实例）。
// @param ctx 操作上下文
// @param meta 模型元数据服务
// @param definitionID 流程定义 ID
// @return 错误（首个不合规节点）
func ValidateCountersignConfigWith(ctx context.Context, meta *ModelMetaService, definitionID string) error {
	metas, err := meta.ListUserTaskMetas(ctx, definitionID)
	if err != nil {
		return err
	}
	for _, meta := range metas {
		// 非会签节点不校验（Java multiInstanceLoopCharacteristics 为空跳过）
		if meta.MultiInstance == nil {
			continue
		}
		switch meta.WeightType {
		case "average":
			// 普通会签：必须配置集合变量设置监听器
			if !hasExecutionListener(meta.ExecutionListenerClasses, ListenerCountersignCollectionConfig, javaCollectionConfigListenerSuffix) {
				return apiError("WORKFLOW-2301", "会签节点【"+meta.ActivityName+"】未配置集合变量设置监听器")
			}
		case "custom":
			// 权重会签：必须配置权重监听器
			if !hasExecutionListener(meta.ExecutionListenerClasses, ListenerCountersignByWeight, javaWeightListenerSuffix) {
				return apiError("WORKFLOW-2301", "会签节点【"+meta.ActivityName+"】未配置集合变量设置监听器")
			}
			// 权重阈值缺失（权重监听器运行时同样拒绝，部署期提前拦截）
			if strings.TrimSpace(meta.WeightThreshold) == "" {
				return apiError("WORKFLOW-2302", "会签节点【"+meta.ActivityName+"】未设置会签权重阈值")
			}
		default:
			// weightType 缺失/未知：与 Java 一致——命中标记不置位，一律报未配置监听器
			return apiError("WORKFLOW-2301", "会签节点【"+meta.ActivityName+"】未配置集合变量设置监听器")
		}
	}
	return nil
}

// hasExecutionListener 判断节点执行监听器类名列表是否命中目标监听器。
// 命中规则：短注册名精确匹配，或类名以 Java 全限定类名后缀结尾（兼容线上语料）。
// @param classes 监听器注册名列表（class/delegateExpression）
// @param shortName Go 注册短名
// @param javaSuffix Java 全限定类名后缀
// @return 是否命中
func hasExecutionListener(classes []string, shortName, javaSuffix string) bool {
	for _, class := range classes {
		trimmed := strings.TrimSpace(class)
		// 短名精确或 Java 全限定类名后缀命中
		if trimmed == shortName || strings.HasSuffix(trimmed, javaSuffix) {
			return true
		}
	}
	return false
}

// ValidateProcessIDMatch 校验流程 id 与预期流程 Key 一致（对位 Java checkCountersignListenerConfig
// 的导入校验段：id 为空报"流程定义key为空"，不一致报"导入流程与当前流程不匹配，请检查后重试。"）。
// @param process 流程模型
// @param expectedKey 预期流程定义 Key
// @return 错误
func ValidateProcessIDMatch(process *model.Process, expectedKey string) error {
	// 流程不存在（模型缺失）
	if process == nil {
		return apiError("WORKFLOW-2303", "流程模型不存在")
	}
	// Java：id 为空 → "流程定义key为空"
	if strings.TrimSpace(process.ID) == "" {
		return apiError("WORKFLOW-2304", "流程定义key为空")
	}
	// Java：id 与预期 Key 不一致 → 导入流程不匹配
	if process.ID != expectedKey {
		return apiError("WORKFLOW-2305", "导入流程与当前流程不匹配，请检查后重试。")
	}
	return nil
}
