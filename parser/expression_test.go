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
	"-10.20": Literal{
		Type:  Number,
		Value: "-10.20",
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
	"(food + good * (cat - dog))": ExprNode{
		Op:   Plus,
		Left: IdName("food"),
		Right: ExprNode{
			Op:   Star,
			Left: IdName("good"),
			Right: ExprNode{
				Op:    Minus,
				Left:  IdName("cat"),
				Right: IdName("dog"),
			},
		},
	},
	"(((food)))": IdName("food"),
	"(good * (cat - dog) + puf - (joker + (flow - flock)))": ExprNode{
		Op: Plus,
		Left: ExprNode{
			Op:   Star,
			Left: IdName("good"),
			Right: ExprNode{
				Op:    Minus,
				Left:  IdName("cat"),
				Right: IdName("dog"),
			},
		},
		Right: ExprNode{
			Op:   Minus,
			Left: IdName("puf"),
			Right: ExprNode{
				Op:   Plus,
				Left: IdName("joker"),
				Right: ExprNode{
					Op:    Minus,
					Left:  IdName("flow"),
					Right: IdName("flock"),
				},
			},
		},
	},
	"flower.grace + foo * 1000": ExprNode{
		Op: Plus,
		Left: ExprNode{
			Op:    Dot,
			Left:  IdName("flower"),
			Right: IdName("grace"),
		},
		Right: ExprNode{
			Op:   Star,
			Left: IdName("foo"),
			Right: Literal{
				Type:  Number,
				Value: "1000",
			},
		},
	},
}

func TestParseExpr(t *testing.T) {
	for expr, expected := range parseExprCases {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Panicked while trying to parse %s. %v", expr, r)
			}
		}()
		tryParseExpr(t, expr, expected)
	}
}

func tryParseExpr(t *testing.T, toParse string, correctResult interface{}) {
	node, err := parseExpr(toParse)
	if err != nil {
		t.Errorf(`Not able to successfully parse "%s"`, toParse)
		return
	}
	if !reflect.DeepEqual(node, correctResult) {
		t.Errorf(`Incorrectly parsed "%s". Got %#v`, toParse, node)
		return
	}
}
