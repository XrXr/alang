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

var tokToOp = map[string]Operator{
	"+": Plus,
	"/": Divide,
	"*": Star,
	"-": Minus,
	".": Dot,
}

var precedence = map[Operator]int{
	Dot:    0,
	Star:   10,
	Divide: 10,
	Plus:   20,
	Minus:  20,
	Assign: 100,
}

type parsedNode struct {
	node     interface{}
	otherEnd int
}

// index for characters in the token string fall in the interval

type opToken struct {
	index int
	op    Operator
}

type OpHeap []opToken

func (o OpHeap) Len() int { return len(o) }
func (o OpHeap) Less(i, j int) bool {
	preceI := precedence[o[i].op]
	preceJ := precedence[o[j].op]
	if preceI == preceJ {
		return o[i].index > o[j].index
	} else {
		return preceI < preceJ
	}
}
func (o OpHeap) Swap(i, j int) { o[i], o[j] = o[j], o[i] }

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

func parseExpr(s string) (interface{}, error) {
	tokens := Tokenize(s)

	type bracketInfo struct {
		open int
		end  int
	}

	parenInfo := make([]bracketInfo, 0)
	openStack := make([]int, 0)
	for i, tok := range tokens {
		if tok == "(" {
			openStack = append(openStack, i)
		} else if tok == ")" {
			if len(openStack) == 0 {
				return nil, &ParseError{0, 0 /* tokenToCol(i)*/, `unmatched ")"`}
			}
			parenInfo = append(parenInfo, bracketInfo{openStack[len(openStack)-1], i})
			openStack = openStack[:len(openStack)-1]
		}
	}
	if len(openStack) != 0 {
		return nil, &ParseError{0, 0 /* tokenToCol(openStack[len(openStack)-1])*/, `unclosed "("`}
	}

	parsed := make(map[int]parsedNode)
	for _, paren := range parenInfo {
		node, err := parseExprUnit(parsed, tokens, paren.open+1, paren.end)
		if err != nil {
			return nil, err
		}
		parsed[paren.open] = parsedNode{node, paren.end}
		parsed[paren.end] = parsedNode{node, paren.open}
	}

	outterMost, found := parsed[0]
	if found && outterMost.otherEnd == len(tokens)-1 {
		return outterMost.node, nil
	}
	return parseExprUnit(parsed, tokens, 0, len(tokens))
}

// A unit does not contain unparsed parenthese
func parseExprUnit(parsed map[int]parsedNode, tokens []string, start int, end int) (interface{}, error) {
	var ops OpHeap
	heap.Init(&ops)

	if start == end {
		return nil, &ParseError{0, 0, "Empty expression"}
	}

	if end-start == 1 {
		return parseToken(tokens[start]), nil
	}

	i := start
	for i < end {
		parsed, found := parsed[i]
		if found {
			i = parsed.otherEnd + 1
			continue
		}
		tok := tokens[i]
		op, good := tokToOp[tok]
		if good {
			heap.Push(&ops, opToken{i, op})
		}
		i++
		if tok == "(" {
			fmt.Printf("%#v, i=%v", tokens, i)
			panic("parseExprUnit() shouldn't see any open parenthesis tokens")
		}
	}

	if len(ops) == 0 {
		parsed, found := parsed[start]
		if !found || parsed.otherEnd != end-1 {
			return nil, &ParseError{0, 0, "Expected an operator"}
		}
		return parsed.node, nil
	}

	var finalNode ExprNode
	for len(ops) > 0 {
		opTok := heap.Pop(&ops).(opToken)
		leftI := opTok.index - 1
		rightI := opTok.index + 1

		var newNode ExprNode
		newNode.Op = opTok.op
		if opTok.index > 0 {
			parsed, good := parsed[opTok.index-1]
			if good {
				newNode.Left = parsed.node
				leftI = parsed.otherEnd
			} else {
				newNode.Left = parseToken(tokens[opTok.index-1])
			}
		}
		if opTok.index < len(tokens)-1 {
			parsed, good := parsed[opTok.index+1]
			if good {
				newNode.Right = parsed.node
				rightI = parsed.otherEnd
			} else {
				newNode.Right = parseToken(tokens[opTok.index+1])
			}
		}

		finalNode = newNode

		if opTok.index > 0 {
			parsed[leftI] = parsedNode{newNode, rightI}
		}
		if opTok.index < len(tokens)-1 {
			parsed[rightI] = parsedNode{newNode, leftI}
		}
	}

	return finalNode, nil
}

func parseToken(s string) interface{} {
	// TODO: complete this
	if unicode.IsDigit(rune(s[0])) || s[0] == '-' {
		return Literal{Number, s}
	} else {
		return IdName(s)
	}
}
