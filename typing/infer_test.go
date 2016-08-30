package typing

import (
	"github.com/XrXr/alang/parser"
	"testing"
)

var InferFixture = map[string]TypeName{
	`"food"`: "string",
}

func TestInferExprType(t *testing.T) {
	for expr, expected := range InferFixture {
		node, err := parser.ParseExpr(expr)
		if err != nil {
			t.Errorf(`Failed to parse %s. Error in the fixture?`, expr)
			return
		}
		actual := InferExprType(node)
		if expected != actual {
			t.Errorf(`Type of "%s" should be %s. Got %s`, expr, expected, actual)
		}
	}
}
