package typing

import (
	"fmt"
	"github.com/XrXr/alang/parser"
)

type FuncType struct {
	Return TypeName
	Args   []TypeName
}

type VarType struct {
	Base       TypeName
	Parameters []TypeName
}

type FuncRecord map[parser.IdName]FuncType
type VarRecord map[parser.IdName]VarType

type EnvRecord struct {
	Funcs FuncRecord
	Vars  VarRecord
}

func (e *EnvRecord) contains(decl parser.IdName) bool {
	_, found := e.Vars[decl]
	if found {
		return true
	}
	_, found = e.Funcs[decl]
	return found
}

func TypeCheck(env *EnvRecord, expr interface{}) error {
	switch node := expr.(type) {
	case parser.ExprNode:
		if node.Op == parser.Assign {
			assignee := node.Left.(parser.IdName)
			if !env.contains(assignee) {
				return undefinedError(assignee)
			}
			return TypeCheck(env, expr)
		} else {
			left := TypeCheck(env, node.Left)
			right := TypeCheck(env, node.Right)
			if left != nil {
				return left
			}
			if right != nil {
				return right
			}
		}
	case parser.ProcCall:
		record, found := env.Funcs[node.Callee]
		if !found {
			return undefinedError(node.Callee)
		}
		if len(record.Args) != len(node.Args) {
			message := fmt.Sprintf(`%s() takes exactly %d arguments. %d were given`,
				node.Callee, len(record.Args), len(node.Args))
			return &parser.ParseError{0, 0, message}
		}
		for i, expectedType := range record.Args {
			ithArgType := InferExprType(node.Args[i])
			if expectedType != ithArgType {
				message := fmt.Sprintf(`got %s for argument number %d of %s instead of the expected %s`,
					ithArgType, i, node.Callee, expectedType)
				return &parser.ParseError{0, 0, message}
			}
		}
	}
	return nil
}

func undefinedError(varName parser.IdName) error {
	//TODO: change name of error
	return &parser.ParseError{0, 0, fmt.Sprintf(`"%s" undefined`, string(varName))}
}
