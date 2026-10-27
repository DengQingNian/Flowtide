package xml

import (
	"testing"

	"github.com/flowable-go/flowable/bpmn/model"
)

// TestParse_Transaction 验证事务子流程与取消事件解析。
// @param t 测试上下文
func TestParse_Transaction(t *testing.T) {
	data := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p"><startEvent id="s"/><sequenceFlow id="f1" sourceRef="s" targetRef="tx"/><transaction id="tx"><startEvent id="ts"/><sequenceFlow id="tf" sourceRef="ts" targetRef="cancelEnd"/><endEvent id="cancelEnd"><cancelEventDefinition/></endEvent></transaction><boundaryEvent id="cb" attachedToRef="tx"><cancelEventDefinition/></boundaryEvent><sequenceFlow id="f2" sourceRef="tx" targetRef="e"/><sequenceFlow id="f3" sourceRef="cb" targetRef="e"/><endEvent id="e"/></process></definitions>`
	parsed, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	tx := parsed.Processes[0].Elements["tx"].(*model.SubProcess)
	if !tx.Transaction || tx.ElementType() != "transaction" {
		t.Fatalf("事务子流程解析错误: %+v", tx)
	}
	end := tx.Elements["cancelEnd"].(*model.EndEvent)
	if !end.Cancel {
		t.Fatalf("取消结束事件解析错误")
	}
	boundary := parsed.Processes[0].Elements["cb"].(*model.BoundaryEvent)
	if !boundary.Cancel {
		t.Fatalf("取消边界解析错误")
	}
}
