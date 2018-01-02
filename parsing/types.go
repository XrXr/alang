package parsing

import (
	"fmt"
)

type IdName string

// Either Base is set or ArraySizes and ArrayBase is set.
// Indirection always happens before the base/array
type TypeDecl struct {
	Base               IdName
	ArraySizes         []int
	ArrayBase          *TypeDecl
	LevelOfIndirection int
}

type Declaration struct {
	Type TypeDecl
	Name IdName
}

type Block []interface{}

type ProcNode struct {
	Args []Declaration
	Ret  TypeDecl
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
	AddressOf
)

//go:generate $GOPATH/bin/stringer -type=LiteralType
type LiteralType int

const (
	Number LiteralType = iota + 1
	String
	Array
	Boolean
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

type StructDeclare struct {
	Name IdName
}

type ParseError struct {
	Line    int
	Column  int
	Message string
}

type IfNode struct {
	Condition interface{}
}

type ElseNode struct{}

type BlockEnd int

func (e *ParseError) Error() string {
	return fmt.Sprintf("%d:%d %s", e.Line, e.Column, e.Message)
}
