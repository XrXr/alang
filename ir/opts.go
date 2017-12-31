package ir

type AssignImm struct {
	Var int
	Val interface{}
}

type BinaryVarOpt struct {
	Left  int
	Right int
}

func (s *Assign) Uses(out []int) int {
	out[0] = s.Left
	out[1] = s.Right
	return 2
}

func (s *Assign) Swap(original int, new int) {
	if s.Left == original {
		s.Left = new
	}
	if s.Right == original {
		s.Right = new
	}
}

type Assign BinaryVarOpt

type Add BinaryVarOpt

type Sub BinaryVarOpt

type Mult BinaryVarOpt

type Div BinaryVarOpt

type JumpIfFalse struct {
	VarToCheck int
	Label      string
}

type Jump struct {
	Label string
}

type Label struct {
	Name string
}

type StartProc struct {
	Name string
}

type EndProc struct {
}

type TakeAddress struct {
	Var int
	Out int
}

type IndirectLoad struct {
	Pointer int
	Out     int
}

type IndirectWrite struct {
	Pointer int
	Data    int
}

type Call struct {
	Label   string
	ArgVars []int
	Out     int
}

type Transclude struct {
	Node *interface{}
}

type StructMemberPtr struct {
	Base   int
	Member string
	Out    int
}

type LoadStructMember struct {
	Base   int
	Member string
	Out    int
}

func (s *LoadStructMember) Uses(out []int) int {
	out[0] = s.Base
	out[1] = s.Out
	return 2
}

func (s *LoadStructMember) Swap(original int, new int) {
	if s.Base == original {
		s.Base = new
	}
	if s.Out == original {
		s.Out = new
	}
}

type ReNumber interface {
	Uses([]int) int
	Swap(int, int)
}
