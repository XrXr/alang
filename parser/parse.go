package parser

import (
	"container/heap"
	"fmt"
	// "github.com/XrXr/alang/parser/fsm"
	"unicode"
)

var _ = fmt.Printf // for debugging. remove when done

func Parse(s string) (interface{}, error) {
	return nil, &ParseError{0, 0, "Not implemented"}
}

// /*
//    Walk over runes in a string, return index of first occurance where
//    `pred` returns false. If pred returns true for all runes, `len(s)` is
//    returned
// */
// func walkUntilFalse(s []byte, pred func(c rune) bool) int {
// 	var i int
// 	var c byte
// 	found := false
// 	for i, c = range s {
// 		if !pred(rune(c)) {
// 			found = true
// 			break
// 		}
// 	}

// 	if !found {
// 		return len(s)
// 	}
// 	return i
// }

// // TODO: do these two with a the list of operators
// func rightOperatorBoundary(c rune) bool {
// 	singleChar := (c == '+' || c == '-' || c == '*' || c == '/' || c == '=')
// 	return singleChar
// }

// func leftOperatorBoundary(c rune) bool {
// 	return rightOperatorBoundary(c)
// }

// // TODO: do this with a map and HasPrefix
// func consumeOperator(s []byte, i int) (Operator, int, bool) {
// 	switch s[i] {
// 	case '+':
// 		return PLUS, i + 1, true
// 	case '-':
// 		return MINUS, i + 1, true
// 	case '*':
// 		return MULTIPLY, i + 1, true
// 	case '=':
// 		return ASSIGN, i + 1, true
// 	case ':':
// 		if i+1 < len(s) && s[i+1] == '=' {
// 			return DECLEAR, i + 2, true
// 		}
// 		fallthrough
// 	case '#':
// 		return DEREFERENCE, i + 1, true
// 	}
// 	return 0, 0, false
// }

// func iAfterWhitespaceRight(s []byte, start int) int {
// 	for i := start; i < len(s); i++ {
// 		if s[i] != ' ' {
// 			return i
// 		}
// 	}
// 	return -1
// }

// func iAfterWhitespaceLeft(s []byte, start int) int {
// 	for i := start; i >= 0; i-- {
// 		if s[i] != ' ' {
// 			return i
// 		}
// 	}
// 	return -1
// }

// func scanLeftForToken(s []byte, start int) (int, int) {
// 	i := start
// 	idMachine := fsm.NewBackwardNFA(fsm.IdentifierName())
// 	machines := []fsm.StateMachine{fsm.StateMachine(&idMachine)}
// 	machines = append(machines, fsm.BackwardNumLiteralMachines()...)
// 	acceptedIdx := -1
// 	for ; i >= 0 && !rightOperatorBoundary(rune(s[i])); i-- {
// 		acceptedIdx = fsm.AdvanceAll(rune(s[i]), machines...)
// 	}
// 	return i + 1, acceptedIdx
// }

// func scanRightForToken(s []byte, start int) (int, int) {
// 	i := start
// 	idMachine := fsm.NewForwardDFA(fsm.IdentifierName())
// 	machines := []fsm.StateMachine{fsm.StateMachine(&idMachine)}
// 	machines = append(machines, fsm.ForwardNumLiteralMachines()...)
// 	acceptedIdx := -1
// 	for ; i < len(s) && !leftOperatorBoundary(rune(s[i])); i++ {
// 		acceptedIdx = fsm.AdvanceAll(rune(s[i]), machines...)
// 	}
// 	return i, acceptedIdx
// }

type parsedNode struct {
	node   interface{}
	jumpTo int
}

// index for characters in the token string fall in the interval

type opToken struct {
	index int
	op    Operator
}

type OpHeap []opToken

func (o OpHeap) Len() int           { return len(o) }
func (o OpHeap) Less(i, j int) bool { return int(o[i].op) < int(o[j].op) }
func (o OpHeap) Swap(i, j int)      { o[i], o[j] = o[j], o[i] }

func (o *OpHeap) Push(x interface{}) {
	*o = append(*o, x.(opToken))
}

func (o *OpHeap) Pop() interface{} {
	old := *o
	n := len(old)
	x := old[n-1]
	*o = old[0 : n-1]
	return x
}

var tokToOp = map[string]Operator{
	"+": Plus,
	"/": Divide,
	"*": Star,
	"-": Minus,
}

func parseExpr(s []byte) (interface{}, error) {
	var ops OpHeap
	heap.Init(&ops)
	var parsed map[int]parsedNode
	tokens := Tokenize(string(s))

	for i, tok := range tokens {
		op, good := tokToOp[tok]
		if good {
			heap.Push(&ops, opToken{i, op})
		}
	}

	var finalNode ExprNode
	for len(ops) > 0 {
		opTok := heap.Pop(&ops).(opToken)

		var newNode ExprNode
		newNode.Op = opTok.op
		if opTok.index > 0 {
			node, good := parsed[opTok.index-1]
			if good {
				newNode.Left = node
			} else {
				newNode.Left = tokenType(tokens[opTok.index-1])
			}
		}
		if opTok.index < len(tokens)-1 {
			node, good := parsed[opTok.index+1]
			if good {
				newNode.Right = node
			} else {
				newNode.Right = tokenType(tokens[opTok.index+1])
			}
		}
		finalNode = newNode
	}

	return finalNode, nil
}

func tokenType(s string) interface{} {
	// TODO: complete this
	if unicode.IsDigit(rune(s[0])) {
		return Literal{Number, s}
	} else {
		return IdName(s)
	}
}
