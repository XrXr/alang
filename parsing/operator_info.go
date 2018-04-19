package parsing

var tokToOp = map[string]Operator{
	"::": ConstDeclare,
	"..": Range,
	"[":  ArrayAccess,
	":=": Declare,
	"<":  Lesser,
	"<=": LesserEqual,
	">":  Greater,
	">=": GreaterEqual,
	"==": DoubleEqual,
	"!=": BangEqual,
	"=":  Assign,
	"+":  Plus,
	"/":  Divide,
	"@":  Dereference,
	"*":  Star,
	"-":  Minus,
	".":  Dot,
	"&":  AddressOf,
}

var precedence = map[Operator]int{
	Dot:          0,
	ArrayAccess:  0,
	Dereference:  5,
	AddressOf:    5,
	Star:         10,
	Divide:       10,
	Plus:         20,
	Minus:        20,
	Lesser:       30,
	LesserEqual:  30,
	Greater:      30,
	GreaterEqual: 30,
	DoubleEqual:  30,
	BangEqual:    30,
	Range:        90,
	Assign:       100,
	Declare:      100,
	ConstDeclare: 100,
}

var isUnary = map[Operator]bool{
	Dereference: true,
}
