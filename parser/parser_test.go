package parser

import (
	"reflect"
	"testing"
)

func TestParseHelloWorld(t *testing.T) {
	var p Parser
	isComplete, firstNode, parent, _ := p.FeedLine("main :: proc () {")
	if isComplete {
		t.Errorf("First line should be incomplete")
		return
	}
	if parent != nil {
		t.Errorf("First line shouldn't have a parent")
		return
	}
	firstLineExpected := ExprNode{
		Op:   ConstDeclare,
		Left: IdName("main"),
		Right: ProcNode{
			Args: []Declaration{},
			Ret:  TypeName("void"),
			Body: Block{},
		},
	}
	if !reflect.DeepEqual(firstNode, firstLineExpected) {
		t.Errorf("Bad parse for first line. Got %#v", firstNode)
		return
	}
	isComplete, secondNode, parent, _ := p.FeedLine(`puts("Hello World")`)
	if !isComplete {
		t.Errorf("Second line should be complete")
		return
	}
	if !reflect.DeepEqual(parent, firstNode) {
		t.Errorf("Second line should point back to the first node. Got %#v", parent)
		return
	}
	secondLineExpected := ProcCall{
		Callee: IdName("puts"),
		Args: []interface{}{
			Literal{
				Type:  String,
				Value: "Hello World",
			},
		},
	}
	if !reflect.DeepEqual(secondNode, secondLineExpected) {
		t.Errorf("Bad parse for second line")
		return
	}
	isComplete, thirdNode, parent, _ := p.FeedLine(`}`)
	if !isComplete {
		t.Errorf("Third line should be complete")
		return
	}
	if parent != nil {
		t.Errorf("Third line should't have a parent")
		return
	}
	if !reflect.DeepEqual(thirdNode, firstNode) {
		t.Errorf("Third node should be first node. Got %#v", thirdNode)
		return
	}
}
