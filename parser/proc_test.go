package parser

import (
    "testing"
    "reflect"
)

func TestEmptyFunc(t *testing.T) {
    node, success := Parse("proc flyers(int flow) int {}")
    if !success {
        t.Error()
        return
    }
    pNode, parsed := node.(ProcNode)
    if !parsed {
        t.Error()
    }
    argList := make([]Identifier, 1)
    argList[0] = Identifier{Type: "int", Name: "flow"}
    reflect.DeepEqual(pNode, ProcNode{
        Name: "flyers",
        Args: argList,
        Ret: "int",
        Body: make([]interface{}, 0),
    })
}
