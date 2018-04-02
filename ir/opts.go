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

type CallExtra struct {
	Name    string
	ArgVars []int
}

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
