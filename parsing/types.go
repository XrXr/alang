package parsing

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

type ProcDecl struct {
	Args      []Declaration
	Return    TypeDecl
	IsForeign bool
}

type Block []interface{}

type ProcNode struct {
	ProcDecl
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
	Range
	Divide
	Call
	Assign
	Declare
	PlusEqual
	MinusEqual
	Lesser
	LesserEqual
	Greater
	GreaterEqual
	DoubleEqual
	BangEqual
	LogicalAnd
	LogicalOr
	LogicalNot
	ConstDeclare
	Dereference
	AddressOf
	ArrayAccess
)

//go:generate $GOPATH/bin/stringer -type=LiteralType
type LiteralType int

const (
	Number LiteralType = iota + 1
	String
	Array
	Boolean
	NilPtr
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

type IfNode struct {
	Condition interface{}
}

type Loop struct {
	Expression interface{}
}

type ReturnNode struct {
	Values []interface{}
}

type ElseNode struct{}

type ContinueNode struct{}

type BreakNode struct{}

type BlockEnd int
