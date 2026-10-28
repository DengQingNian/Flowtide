package rest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

const startableGenericXML = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="startableGeneric" isExecutable="true"><startEvent id="s"/><sequenceFlow id="f" sourceRef="s" targetRef="e"/><endEvent id="e"/></process></definitions>`

const startableUserXML = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn"><process id="startableUser" isExecutable="true" flowable:candidateStarterUsers="alice"><startEvent id="s"/><sequenceFlow id="f" sourceRef="s" targetRef="e"/><endEvent id="e"/></process></definitions>`

const startableGroupXML = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn"><process id="startableGroup" isExecutable="true" flowable:candidateStarterGroups="g1"><startEvent id="s"/><sequenceFlow id="f" sourceRef="s" targetRef="e"/><endEvent id="e"/></process></definitions>`

// TestREST_StartableByUser 验证按候选人过滤流程定义。
// @param t 测试上下文
func TestREST_StartableByUser(t *testing.T) {
	server, _ := newTestServer(t)
	for _, xml := range []string{startableGenericXML, startableUserXML, startableGroupXML} {
		response, _ := http.Post(server.URL+"/service/repository/deployments", "application/xml", bytes.NewBufferString(xml))
		_ = response.Body.Close()
	}
	response := postJSON(t, server.URL+"/service/identity/users", map[string]any{"id": "bob"})
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/identity/groups", map[string]any{"id": "g1", "name": "G1"})
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/identity/groups/g1/members", map[string]any{"userId": "bob"})
	_ = response.Body.Close()

	cases := map[string]map[string]bool{
		"alice": {"startableGeneric": true, "startableUser": true},
		"bob":   {"startableGeneric": true, "startableGroup": true},
		"carol": {"startableGeneric": true},
	}
	for user, expected := range cases {
		response, _ := http.Get(server.URL + "/service/repository/process-definitions?size=100&startableByUser=" + user)
		var page struct {
			Data []struct {
				Key string `json:"key"`
			} `json:"data"`
		}
		_ = json.NewDecoder(response.Body).Decode(&page)
		_ = response.Body.Close()
		keys := map[string]bool{}
		for _, definition := range page.Data {
			if _, isStartable := expected[definition.Key]; isStartable {
				keys[definition.Key] = true
			}
		}
		if len(keys) != len(expected) {
			t.Fatalf("用户 %s 可发起定义错误: got %+v want %+v", user, keys, expected)
		}
	}
}
