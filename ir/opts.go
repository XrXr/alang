package ir

type Declare struct {
	Var int
	Val interface{}
}

type Assign struct {
	Left  int
	Right int
}

type AssignImm struct {
	Var int
	Val interface{}
}

type Add struct {
	Left  int
	Right int
}

type Sub struct {
	Left  int
	Right int
}

type Mult struct {
	Left  int
	Right int
}

type Div struct {
	Left  int
	Right int
}
