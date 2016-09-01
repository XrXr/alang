package parser

import (
	"fmt"
)

type TypeName string
type IdName string

type Declaration struct {
	Type TypeName
	Name IdName
}

type Block []interface{}

type ProcNode struct {
	Args []Declaration
	Ret  TypeName
	Body Block
}

type ProcCall struct {
	Callee IdName
	Args   []interface{}
}

const Invalid = 0

//go:generate $GOPATH/bin/stringer -type=Operator
type Operator int

const (
	Dot Operator = iota + 1
	Star
	Minus
	Plus
	Divide
	Call
	Assign
	Declare
	ConstDeclare
	Dereference
)

//go:generate $GOPATH/bin/stringer -type=LiteralType
type LiteralType int

const (
	Number LiteralType = iota + 1
	String
	Array
)

type Literal struct {
	Type  LiteralType
	Value string
}

type ExprNode struct {
	Op    Operator
	Left  interface{} // either a ExprNode or a Literal or a IdName
	Right interface{}
}

type ParseError struct {
	Line    int
	Column  int
	Message string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%d:%d", e.Line, e.Column)
}
