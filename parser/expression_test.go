package parser

import (
	"reflect"
	"testing"
)

// func TestEmptyFunc(t *testing.T) {
//     node, err := Parse("proc flyers(flow: int) int {}")
//     if err != nil {
//         t.Error()
//         return
//     }
//     pNode, parsed := node.(ProcNode)
//     if !parsed {
//         t.Error()
//     }
//     argList := make([]Declearation, 1)
//     argList[0] = Declearation{Type: "int", Name: "flow"}
//     reflect.DeepEqual(pNode, ProcNode{
//         Name: "flyers",
//         Args: argList,
//         Ret: "int",
//         Body: make([]interface{}, 0),
//     })
// }

var parseExprCases = map[string]interface{}{
	"+ br1dg3": ExprNode{
		Op:    Plus,
		Left:  nil,
		Right: IdName("br1dg3"),
	},
	"-10.20": ExprNode{
		Op:   Minus,
		Left: nil,
		Right: Literal{
			Type:  Number,
			Value: "10.20",
		},
	},
	"12.82 + foo * bar - 1000": ExprNode{
		Op: Plus,
		Left: Literal{
			Type:  Number,
			Value: "12.82",
		},
		Right: ExprNode{
			Op: Minus,
			Left: ExprNode{
				Op:    Star,
				Left:  IdName("foo"),
				Right: IdName("bar"),
			},
			Right: Literal{
				Type:  Number,
				Value: "1000",
			},
		},
	},
	"12.82 + foo * (bar - 1000)": ExprNode{
		Op: Plus,
		Left: Literal{
			Type:  Number,
			Value: "12.82",
		},
		Right: ExprNode{
			Op:   Star,
			Left: IdName("foo"),
			Right: ExprNode{
				Op:   Minus,
				Left: IdName("bar"),
				Right: Literal{
					Type:  Number,
					Value: "1000",
				},
			},
		},
	},
	"flower.grace + foo * -1000": ExprNode{
		Op: Plus,
		Left: ExprNode{
			Op:    Dot,
			Left:  IdName("flower"),
			Right: IdName("grace"),
		},
		Right: ExprNode{
			Op:   Star,
			Left: IdName("foo"),
			Right: ExprNode{
				Op:   Minus,
				Left: nil,
				Right: Literal{
					Type:  Number,
					Value: "1000",
				},
			},
		},
	},
}

func TestParseExpr(t *testing.T) {
	for expr, expected := range parseExprCases {
		tryParseExpr(t, expr, expected)
	}
}

func tryParseExpr(t *testing.T, toParse string, correctResult interface{}) {
	node, err := parseExpr([]byte(toParse))
	if err != nil {
		t.Errorf(`Not able to successfully parse "%s"`, toParse)
		return
	}
	if !reflect.DeepEqual(node, correctResult) {
		t.Errorf(`Incorrectly parsed "%s"`, toParse)
		return
	}
}
