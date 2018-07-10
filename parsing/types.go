package parsing

type ASTNode interface {
	GetLineNumber() int
	GetStartColumn() int
	GetEndColumn() int
}

type sourceLocation struct {
	line        int
	startColumn int
	endColumn   int
}

func (s sourceLocation) GetLineNumber() int {
	return s.line
}

func (s sourceLocation) GetStartColumn() int {
	return s.startColumn
}

func (s sourceLocation) GetEndColumn() int {
	return s.endColumn
}

type IdName struct {
	sourceLocation
	Name string
}

// Either Base is set or ArraySizes and ArrayBase is set.
// Indirection always happens before the base/array
type TypeDecl struct {
	sourceLocation
	Base               IdName
	ArraySizes         []int
	ArrayBase          *TypeDecl
	LevelOfIndirection int
}

type Declaration struct {
	sourceLocation
	Type TypeDecl
	Name IdName
}

type ProcDecl struct {
	sourceLocation
	Args      []Declaration
	Return    TypeDecl
	IsForeign bool
}

type ProcCall struct {
	sourceLocation
	Callee IdName
	Args   []ASTNode
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
	sourceLocation
	Type  LiteralType
	Value string
}

type ExprNode struct {
	sourceLocation
	Op    Operator
	Left  ASTNode
	Right ASTNode
}

type StructDeclare struct {
	sourceLocation
	Name IdName
}

type IfNode struct {
	sourceLocation
	Condition ASTNode
}

type Loop struct {
	sourceLocation
	Expression ASTNode
}

type ReturnNode struct {
	sourceLocation
	Values []ASTNode
}

type ElseNode struct {
	sourceLocation
}

type ContinueNode struct {
	sourceLocation
}

type BreakNode struct {
	sourceLocation
}

type BlockEnd struct {
	sourceLocation
}
