package api

import (
	"encoding/json"
	"testing"
)

// TestCompleteTaskVoUnmarshalJSON 验证会签结论显式标记的解析语义：
// 缺省与显式 null 视为未设置，显式 0（否决）与 1（通过）均视为已设置。
// @param t 测试上下文
func TestCompleteTaskVoUnmarshalJSON(t *testing.T) {
	cases := []struct {
		name      string // 用例名
		data      string // 原始 JSON
		wantSet   bool   // 期望 CountersignResultSet
		wantValue int    // 期望 CountersignResult
		wantTask  string // 期望解析出的任务 Id
	}{
		{name: "缺省", data: `{"taskId":"t1","userCode":"u1","message":"同意","type":"SP"}`, wantSet: false, wantValue: 0, wantTask: "t1"},
		{name: "显式否决零", data: `{"taskId":"t2","countersignResult":0}`, wantSet: true, wantValue: 0, wantTask: "t2"},
		{name: "显式通过", data: `{"taskId":"t3","countersignResult":1}`, wantSet: true, wantValue: 1, wantTask: "t3"},
		{name: "显式 null", data: `{"taskId":"t4","countersignResult":null}`, wantSet: false, wantValue: 0, wantTask: "t4"},
	}
	for _, tc := range cases {
		var vo CompleteTaskVo
		if err := json.Unmarshal([]byte(tc.data), &vo); err != nil {
			t.Fatalf("用例 %s 解析失败: %v", tc.name, err)
		}
		if vo.CountersignResultSet != tc.wantSet || vo.CountersignResult != tc.wantValue {
			t.Fatalf("用例 %s 会签结论错误: set=%v value=%d", tc.name, vo.CountersignResultSet, vo.CountersignResult)
		}
		if vo.TaskID != tc.wantTask {
			t.Fatalf("用例 %s 基础字段错误: %s", tc.name, vo.TaskID)
		}
	}
}

// TestCompleteTaskVoUnmarshalJSONInvalid 验证非法 JSON 返回解析错误。
// @param t 测试上下文
func TestCompleteTaskVoUnmarshalJSONInvalid(t *testing.T) {
	var vo CompleteTaskVo
	if err := json.Unmarshal([]byte(`{"taskId":`), &vo); err == nil {
		t.Fatalf("非法 JSON 应报错")
	}
}

// TestCompleteTaskVoUnmarshalJSONTypeMismatch 验证字段类型不匹配时透传解析错误
// （JSON 语法本身合法，错误由别名结构体解析产生）。
// @param t 测试上下文
func TestCompleteTaskVoUnmarshalJSONTypeMismatch(t *testing.T) {
	var vo CompleteTaskVo
	if err := json.Unmarshal([]byte(`{"taskId":123}`), &vo); err == nil {
		t.Fatalf("字段类型不匹配应报错")
	}
}

// TestROkMsgAndRErr 验证成功与失败返回结构的字段契约。
// @param t 测试上下文
func TestROkMsgAndRErr(t *testing.T) {
	plain := ROk("done")
	if plain.Code != "200" || !plain.Success || plain.Msg != "" || plain.Data != "done" {
		t.Fatalf("无消息成功返回字段错误: %+v", plain)
	}
	ok := ROkMsg("已保存", map[string]string{"id": "m1"})
	if ok.Code != "200" || !ok.Success || ok.Msg != "已保存" || ok.ErrorMessage != "" {
		t.Fatalf("成功返回字段错误: %+v", ok)
	}
	failed := RErr("500", "模型不存在")
	if failed.Code != "500" || failed.Success || failed.ErrorMessage != "模型不存在" || failed.Msg != "" {
		t.Fatalf("失败返回字段错误: %+v", failed)
	}
}
