// dict_test.go 验证静态字典服务（D9）：码表全集、条目内容、副本隔离与错误路径。
package workflow

import (
	"errors"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
)

// TestDictServiceListCodes 验证字典码全集与排序。
// @param t 测试上下文
func TestDictServiceListCodes(t *testing.T) {
	svc := NewDictService()
	codes := svc.ListDictCodes()
	expect := []string{
		DictCodeConsignType, DictCodeEnable, DictCodeEntrustCondition, DictCodeExceptionType,
		DictCodeFlowStatus, DictCodeHandleStatus, DictCodeMessageType, DictCodeRemindType,
		DictCodeTargetType, DictCodeTaskType, DictCodeVisible,
	}
	if len(codes) != len(expect) {
		t.Fatalf("字典码应 %d 个，实际 %d 个: %v", len(expect), len(codes), codes)
	}
	for i, code := range expect {
		if codes[i] != code {
			t.Errorf("第 %d 个字典码应为 %s，实际 %s（应升序）", i, code, codes[i])
		}
	}
}

// TestDictServiceGetDict 验证各码表条目内容与协议对齐点。
// @param t 测试上下文
func TestDictServiceGetDict(t *testing.T) {
	svc := NewDictService()
	// messageType 消息渠道
	if dict, err := svc.GetDict(DictCodeMessageType); err != nil || len(dict.Items) != 5 || dict.Code != DictCodeMessageType {
		t.Errorf("messageType 应 5 条目: %v %v", dict, err)
	}
	// consignType 值与 Java WorkflowTaskDelegateEntity.CONSIGNTYPE_* 逐字一致
	consign, err := svc.GetDict(DictCodeConsignType)
	if err != nil {
		t.Fatalf("consignType 获取失败: %v", err)
	}
	for _, want := range []string{"All", "SingleProject", "MultiProject", "AllProject", "SingleFlowCategory", "MultiFlowCategory", "AllFlowCategory"} {
		if !dictHasValue(consign.Items, want) {
			t.Errorf("consignType 缺少 Java 协议值 %s: %v", want, consign.Items)
		}
	}
	// exceptionType 值与 Java WorkflowTaskExceptionEntity.EXCEPTION_TYPE_* 四类逐字一致
	exception, err := svc.GetDict(DictCodeExceptionType)
	if err != nil || len(exception.Items) != 4 {
		t.Fatalf("exceptionType 应 4 条目: %v %v", exception, err)
	}
	for _, want := range []string{"FlowableException", "BusinessException", "RuntimeException", "NotTaskMessageException"} {
		if !dictHasValue(exception.Items, want) {
			t.Errorf("exceptionType 缺少 Java 协议值 %s", want)
		}
	}
	// entrustCondition 值即条件串前缀（006 §5）
	entrust, err := svc.GetDict(DictCodeEntrustCondition)
	if err != nil || len(entrust.Items) != 2 {
		t.Fatalf("entrustCondition 应 2 条目: %v %v", entrust, err)
	}
	if !dictHasValue(entrust.Items, "@ProjId") || !dictHasValue(entrust.Items, "@FlowCategory") {
		t.Errorf("entrustCondition 缺少条件前缀值: %v", entrust.Items)
	}
	// enable/visible 0/1
	for _, code := range []string{DictCodeEnable, DictCodeVisible} {
		dict, err := svc.GetDict(code)
		if err != nil || len(dict.Items) != 2 || dict.Items[0].Value != "1" || dict.Items[1].Value != "0" {
			t.Errorf("%s 应为 1/0 两档: %v %v", code, dict, err)
		}
	}
	// handleStatus 0/1 两档（异常处理状态流转）
	handle, err := svc.GetDict(DictCodeHandleStatus)
	if err != nil || len(handle.Items) != 2 || !dictHasValue(handle.Items, "0") || !dictHasValue(handle.Items, "1") {
		t.Errorf("handleStatus 应为 0/1 两档: %v %v", handle, err)
	}
	// taskType/flowStatus/targetType/remindType 条目存在
	for _, code := range []string{DictCodeTaskType, DictCodeFlowStatus, DictCodeTargetType, DictCodeRemindType} {
		if dict, err := svc.GetDict(code); err != nil || len(dict.Items) == 0 {
			t.Errorf("%s 条目不应为空: %v %v", code, dict, err)
		}
	}
}

// TestDictServiceUnknownCode 验证未名字典码报错。
// @param t 测试上下文
func TestDictServiceUnknownCode(t *testing.T) {
	svc := NewDictService()
	_, err := svc.GetDict("noSuchDict")
	if err == nil {
		t.Fatal("未名字典应报错")
	}
	var apiErr *api.Error
	if !errors.As(err, &apiErr) || apiErr.Code != errCodeDictUnknown {
		t.Errorf("应返回 %s 错误，实际 %v", errCodeDictUnknown, err)
	}
	// GetDictItems 便捷方法的未知码路径
	if _, err := GetDictItems("noSuchDict"); err == nil {
		t.Error("GetDictItems 未名码应报错")
	}
}

// TestDictServiceReturnsCopy 验证返回条目为副本（调用方修改不污染静态数据）。
// @param t 测试上下文
func TestDictServiceReturnsCopy(t *testing.T) {
	svc := NewDictService()
	first, err := svc.GetDict(DictCodeEnable)
	if err != nil {
		t.Fatalf("获取失败: %v", err)
	}
	first.Items[0].Value = "tampered"
	first.Items = append(first.Items, DictItem{Value: "x", Label: "污染"})
	second, err := svc.GetDict(DictCodeEnable)
	if err != nil {
		t.Fatalf("二次获取失败: %v", err)
	}
	if second.Items[0].Value != "1" || len(second.Items) != 2 {
		t.Errorf("静态数据被污染: %v", second.Items)
	}
}

// dictHasValue 判断条目列表是否含指定值。
// @param items 条目列表
// @param value 目标值
// @return 是否包含
func dictHasValue(items []DictItem, value string) bool {
	for _, item := range items {
		if item.Value == value {
			return true
		}
	}
	return false
}
