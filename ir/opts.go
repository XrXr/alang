package ir

type AssignImm struct {
	Var int
	Val interface{}
}

type BinaryVarOpt struct {
	Left  int
	Right int
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
}

type Transclude struct {
	Node *interface{}
}
