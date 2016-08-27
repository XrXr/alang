package parser

import (
	"fmt"
)

type TypeName string
type IdName string

type Declearation struct {
	Type TypeName
	Name IdName
}

type ProcNode struct {
	Name IdName
	Args []Declearation
	Ret  TypeName
	Body []interface{}
}

const Invalid = 0

type Operator int

const (
	Star  = 1
	Minus = iota
	Plus
	Divide
	Call
	Assign
	Declare
	Dereference
	Dot
)

type LiteralType int

const (
	Number = 1
	String = iota
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
