package typing

import (
	"github.com/XrXr/alang/parsing"
)

type TypeName string // should be good enough for our purposes

func InferExprType(expr interface{}) TypeName {
	switch node := expr.(type) {
	case parser.Literal:
		if node.Type == parser.Number {
			//TODO: obvious wrong. Placeholder
			return "s32"
		} else if node.Type == parser.String {
			return "string"
		}
	}
	return "(unknown)"
}
