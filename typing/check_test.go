package typing

import (
	"github.com/XrXr/alang/parser"
	"testing"
)

// true means pass
var typeCheckFixture = map[string]bool{
	`print("food eater")`: true,
	`print(123)`:          false,
	`a = 100`:             false,
}

func TestTypeCheck(t *testing.T) {
	for expr, shouldPass := range typeCheckFixture {
		node, err := parser.ParseExpr(expr)
		if err != nil {
			t.Errorf(`Failed to parse %s. Error in the fixture?`, expr)
			return
		}
		env := BaseEnv()
		typeErr := TypeCheck(&env, node)
		passed := typeErr == nil
		if passed != shouldPass {
			verb := "pass"
			if !shouldPass {
				verb = "fail"
			}
			t.Errorf(`"%s" should %s type check`, expr, verb)
		}
	}
}
