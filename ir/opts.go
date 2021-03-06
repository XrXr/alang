package ir

import "fmt"
import "github.com/XrXr/alang/parsing"

//go:generate $GOPATH/bin/stringer -type=InstType
type InstType int

// !!! the order here is important !!!
// make sure you are putting things under the correct section.
// these are only about main struct
const (
	ZeroVarInstructions InstType = iota

	Return
	Transclude
	Jump
	StartProc
	EndProc
	Label
	OutsideLoopMutations
	OutOfScopeMutations
	OptionSelectStart
	OptionEnd
	OptionSelectEnd
	LoopEnd

	MutateOnlyInstructions

	Call
	AssignImm
	Increment
	Decrement

	ReadOnlyInstructions

	JumpIfTrue
	JumpIfFalse
	ShortJumpIfTrue
	ShortJumpIfFalse
	Compare

	ReadAndMutateInstructions

	Assign
	TakeAddress
	ArrayToPointer
	IndirectWrite
	IndirectLoad
	StructMemberPtr
	PeelStruct
	Not

	TwoOperandUpdateInstructions

	Add
	Sub
	Mult
	Div
	And
	Or
)

func (i InstType) ZeroVar() bool {
	return i > ZeroVarInstructions && i < MutateOnlyInstructions
}

func (i InstType) MutateOnly() bool {
	return i > MutateOnlyInstructions && i < ReadOnlyInstructions
}

func (i InstType) ReadOnly() bool {
	return i > ReadOnlyInstructions && i < ReadAndMutateInstructions
}

func (i InstType) ReadAndMutate() bool {
	return i > ReadAndMutateInstructions
}

func (i InstType) InputOutput() bool {
	return i > ReadAndMutateInstructions && i < TwoOperandUpdateInstructions
}

func (i InstType) TwoOperandUpdate() bool {
	return i > TwoOperandUpdateInstructions
}

type Inst struct {
	Type          InstType
	MutateOperand int
	ReadOperand   int
	Extra         interface{}
	GeneratedFrom parsing.ASTNode
}

func (i *Inst) Left() int {
	return i.MutateOperand
}

func (i *Inst) Right() int {
	return i.ReadOperand
}

func (i *Inst) In() int {
	return i.ReadOperand
}

func (i *Inst) Out() int {
	return i.MutateOperand
}

func (i *Inst) Swap(original int, newVar int) {
	if i.MutateOperand == original {
		i.MutateOperand = newVar
	}
	if i.ReadOperand == original {
		i.ReadOperand = newVar
	}
}

type CallExtra struct {
	Name     string
	ArgVars  []int
	ReturnTo []int
}

type ReturnExtra struct {
	Values []int
}

//go:generate $GOPATH/bin/stringer -type=ComparisonMethod
type ComparisonMethod int

const (
	Lesser ComparisonMethod = iota
	Greater
	GreaterOrEqual
	LesserOrEqual
	AreEqual
	NotEqual
)

type CompareExtra struct {
	How   ComparisonMethod
	Right int
	Out   int
}

func MakePlainInst(instType InstType, extra interface{}) Inst {
	var newInst Inst
	newInst.Type = instType
	newInst.Extra = extra
	return newInst

}

func MakeReadOnlyInst(instType InstType, readVn int, extra interface{}) Inst {
	var newInst Inst
	newInst.Type = instType
	newInst.Extra = extra
	newInst.ReadOperand = readVn
	return newInst
}

func MakeMutateOnlyInst(instType InstType, mutateVn int, extra interface{}) Inst {
	var newInst Inst
	newInst.Type = instType
	newInst.Extra = extra
	newInst.MutateOperand = mutateVn
	return newInst
}

func MakeBinaryInst(instType InstType, mutateVn int, readVn int, extra interface{}) Inst {
	var newInst Inst
	newInst.Type = instType
	newInst.Extra = extra
	newInst.MutateOperand = mutateVn
	newInst.ReadOperand = readVn
	return newInst
}

func IterOverAllVars(opt Inst, cb func(vn int)) {
	if opt.Type.MutateOnly() {
		cb(opt.MutateOperand)
	} else if opt.Type.ReadOnly() {
		cb(opt.ReadOperand)
	} else if opt.Type.ReadAndMutate() {
		cb(opt.ReadOperand)
		cb(opt.MutateOperand)
	}

	switch opt.Type {
	case Call:
		for _, vn := range opt.Extra.(CallExtra).ArgVars {
			cb(vn)
		}
	case Return:
		for _, vn := range opt.Extra.(ReturnExtra).Values {
			cb(vn)
		}
	case Compare:
		cb(opt.Extra.(CompareExtra).Out)
		cb(opt.Extra.(CompareExtra).Right)
	}
}

func IterAndMutate(opt *Inst, cb func(vn *int)) {
	if opt.Type.MutateOnly() {
		cb(&opt.MutateOperand)
	} else if opt.Type.ReadOnly() {
		cb(&opt.ReadOperand)
	} else if opt.Type.ReadAndMutate() {
		cb(&opt.ReadOperand)
		cb(&opt.MutateOperand)
	}

	switch opt.Type {
	case Call:
		extra := opt.Extra.(CallExtra)
		for i := range extra.ArgVars {
			cb(&extra.ArgVars[i])
		}
		opt.Extra = extra
	case Return:
		extra := opt.Extra.(ReturnExtra)
		for i := range extra.Values {
			cb(&extra.Values[i])
		}
		opt.Extra = extra
	case Compare:
		extra := opt.Extra.(CompareExtra)
		cb(&extra.Out)
		cb(&extra.Right)
		opt.Extra = extra
	}
}

// There is at most one mutation per instruction
// not true if we do :multireturn
func FindMutationVar(opt *Inst) int {
	const noMutation = -1
	if opt.Type == IndirectWrite {
		return noMutation
	}

	if opt.Type.MutateOnly() {
		return opt.MutateOperand
	}
	if opt.Type.ReadAndMutate() {
		return opt.MutateOperand
	}
	switch opt.Type {
	case Compare:
		extra := opt.Extra.(CompareExtra)
		return extra.Out
	}
	return noMutation
}

func EnumerateAllReadOnlyVars(opt *Inst, cb func(vn int)) {
	if opt.Type.ReadOnly() || opt.Type.ReadAndMutate() {
		cb(opt.ReadOperand)
	}

	switch opt.Type {
	case Call:
		for _, vn := range opt.Extra.(CallExtra).ArgVars {
			cb(vn)
		}
	case Return:
		for _, vn := range opt.Extra.(ReturnExtra).Values {
			cb(vn)
		}
	case Compare:
		cb(opt.Extra.(CompareExtra).Right)
	}
}

func Dump(insts []Inst) {
	fmt.Println("IR Dump:")
	for i, opt := range insts {
		fmt.Printf("%d {%s", i, opt.Type.String())
		if opt.Type == Compare {
			extra := opt.Extra.(CompareExtra)
			fmt.Printf(" %d", extra.Out)
		}
		if opt.Type > MutateOnlyInstructions && opt.Type < ReadOnlyInstructions {
			fmt.Printf(" %d", opt.MutateOperand)
		} else if opt.Type > ReadOnlyInstructions && opt.Type < ReadAndMutateInstructions {
			fmt.Printf(" %d", opt.ReadOperand)
		} else if opt.Type > ReadAndMutateInstructions {
			fmt.Printf(" %d %d", opt.MutateOperand, opt.ReadOperand)
		}
		switch opt.Type {
		case Compare:
			extra := opt.Extra.(CompareExtra)
			fmt.Printf(" %d %s", extra.Right, extra.How.String())
		case Return:
			extra := opt.Extra.(ReturnExtra)
			fmt.Printf(" %v", extra.Values)
		case Call:
			extra := opt.Extra.(CallExtra)
			fmt.Printf(" %s %v", extra.Name, extra.ArgVars)
		case Label, Jump, JumpIfTrue, JumpIfFalse, StartProc, PeelStruct, StructMemberPtr:
			fmt.Printf(" %v", opt.Extra)
		case AssignImm, OptionSelectStart, OutsideLoopMutations, OptionEnd:
			fmt.Printf(" (%v)", opt.Extra)
		}
		fmt.Println("}")
	}
}
