package parser

import (
    "testing"
    "reflect"
)

func TestEmptyFunc(t *testing.T) {
    node, err := Parse("proc flyers(flow: int) int {}")
    if err != nil {
        t.Error()
        return
    }
    pNode, parsed := node.(ProcNode)
    if !parsed {
        t.Error()
    }
    argList := make([]Declearation, 1)
    argList[0] = Declearation{Type: "int", Name: "flow"}
    reflect.DeepEqual(pNode, ProcNode{
        Name: "flyers",
        Args: argList,
        Ret: "int",
        Body: make([]interface{}, 0),
    })
}

// func tryParseExpr(s string, pred func (interface{}) bool) {

// }

func TestUnaryExpression(t *testing.T) {
    node, err := parseExpr([]byte("+ br1dg3"))
    if err != nil {
        t.Error()
        return
    }

    expr := node.(ExprNode)
    if (expr.Right).(IdName) != "br1dg3" {
        t.Error()
        return
    }

    node, err = parseExpr([]byte("-10.20"))
    expr = node.(ExprNode)
    if l := (expr.Right).(Literal); l.Type != NUMBER || l.Value != "10.20" {
        t.Error()
        return
    }
}
