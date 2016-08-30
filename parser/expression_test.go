package parser

import (
	"reflect"
	"testing"
)

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
	`"food"`: Literal{
		Type:  String,
		Value: "food",
	},
	"a = bar": ExprNode{
		Op:    Assign,
		Left:  IdName("a"),
		Right: IdName("bar"),
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
	"foo()": ProcCall{
		Callee: IdName("foo"),
		Args:   []interface{}{},
	},
	"foo(jo * ni, hog + rust)": ProcCall{
		Callee: IdName("foo"),
		Args: []interface{}{
			ExprNode{
				Op:    Star,
				Left:  IdName("jo"),
				Right: IdName("ni"),
			},
			ExprNode{
				Op:    Plus,
				Left:  IdName("hog"),
				Right: IdName("rust"),
			},
		},
	},
	"fluke() + belief(jo, rust)": ExprNode{
		Op: Plus,
		Left: ProcCall{
			Callee: IdName("fluke"),
			Args:   []interface{}{},
		},
		Right: ProcCall{
			Callee: IdName("belief"),
			Args:   []interface{}{IdName("jo"), IdName("rust")},
		},
	},
	"f(g(gp(cat, dog)), foo)": ProcCall{
		Callee: IdName("f"),
		Args: []interface{}{
			ProcCall{
				Callee: IdName("g"),
				Args: []interface{}{
					ProcCall{
						Callee: IdName("gp"),
						Args: []interface{}{
							IdName("cat"),
							IdName("dog"),
						},
					},
				},
			},
			IdName("foo"),
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
	// a := parser.ProcCall{Callee: "f", Args: []interface{}{
	// 	parser.ProcCall{Callee: "g", Args: []interface{}{
	// 		parser.ProcCall{
	// 			Callee: "gp",
	// 			Args:   []interface{}{"cat"}}}}}}
}

func tryParseExpr(t *testing.T, toParse string, correctResult interface{}) {
	node, err := ParseExpr(toParse)
	if err != nil {
		t.Errorf(`Not able to successfully parse "%s". %#v`, toParse, err)
		return
	}
	if !reflect.DeepEqual(node, correctResult) {
		t.Errorf(`Incorrectly parsed "%s". Got %#v`, toParse, node)
		return
	}
}
