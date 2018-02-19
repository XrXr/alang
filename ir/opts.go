package ir

type Inst struct {
	Type    InstType
	Oprand1 int
	Oprand2 int
	Extra   interface{}
}

//go:generate $GOPATH/bin/stringer -type=InstType
type InstType int

// !!! the order here is important !!!
// make sure you are putting things under the correct section.
// arity is how many vars are used in the main struct
const (
	ZeroVarInstructions InstType = iota

	Transclude
	Jump
	StartProc
	EndProc
	Label

	UnaryInstructions

	AssignImm
	Increment
	Decrement
	JumpIfFalse
	Call // only one variable (out) is in the body

	BinaryInstructions

	Sub
	Assign
	Add
	Mult
	Div
	TakeAddress
	ArrayToPointer
	IndirectWrite
	IndirectLoad
	StructMemberPtr
	LoadStructMember
	Compare
)

func (i *Inst) Left() int {
	return i.Oprand1
}

func (i *Inst) Right() int {
	return i.Oprand2
}

func (i *Inst) In() int {
	return i.Oprand1
}

func (i *Inst) Out() int {
	return i.Oprand2
}

func (i *Inst) Swap(original int, newVar int) {
	if i.Oprand1 == original {
		i.Oprand1 = newVar
	}
	if i.Oprand2 == original {
		i.Oprand2 = newVar
	}
}

func MakeUnaryInstWithAux(instType InstType, one int, extra interface{}) Inst {
	var newInst Inst
	newInst.Type = instType
	newInst.Extra = extra
	newInst.Oprand1 = one
	return newInst
}

func MakeBinaryInstWithAux(instType InstType, one int, two int, extra interface{}) Inst {
	var newInst Inst
	newInst.Type = instType
	newInst.Extra = extra
	newInst.Oprand1 = one
	newInst.Oprand2 = two
	return newInst
}

// type AssignImm struct {
// 	Var int
// 	Val interface{}
// }

// type JumpIfFalse struct {
// 	VarToCheck int
// 	Label      string
// }

// type Jump struct {
// 	Label string
// }

// type Label struct {
// 	Name string
// }

// type StartProc struct {
// 	Name string
// }

// type EndProc struct {
// }

// type Increment struct {
// 	Var int
// }

// type Decrement struct {
// 	Var int
// }

// type TakeAddress struct {
// 	Var int
// 	Out int
// }

// type ArrayToPointer struct {
// 	Array int
// 	Out   int
// }

// type IndirectLoad struct {
// 	Pointer int
// 	Out     int
// }

// type IndirectWrite struct {
// 	Pointer int
// 	Data    int
// }

type CallExtra struct {
	Name    string
	ArgVars []int
}

// type Call struct {
// 	Name    string
// 	ArgVars []int
// 	Out     int
// }

// type Compare struct {
// 	How   Comparative
// 	Left  int
// 	Right int
// 	Out   int
// }

const (
	Lesser int = iota
	Greater
	GreaterOrEqual
	LesserOrEqual
	AreEqual
	NotEqual
)

type CompareExtra struct {
	How int
	Out int
}

// type Comparative int

// type Transclude struct {
// 	Node *interface{}
// }

// type StructMemberPtr struct {
// 	Base   int
// 	Member string
// 	Out    int
// }

// type LoadStructMember struct {
// 	Base   int
// 	Member string
// 	Out    int
// }
