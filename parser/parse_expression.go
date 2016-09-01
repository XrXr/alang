package parser

import (
	"container/heap"
	"fmt"
	"unicode"
)

var _ = fmt.Printf // for debugging. remove when done

var tokToOp = map[string]Operator{
	"+": Plus,
	"/": Divide,
	"*": Star,
	"-": Minus,
	".": Dot,
	"=": Assign,
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

func ParseExpr(s string) (interface{}, error) {
	tokens := Tokenize(s)
	parsed := make(map[int]parsedNode)
	return parseExprWithParen(parsed, tokens, 0, len(tokens))
}

func parseExprWithParen(parsed map[int]parsedNode, tokens []string, start int, end int) (interface{}, error) {
	type bracketInfo struct {
		open int
		end  int
	}

	parenInfo := make([]bracketInfo, 0)
	openStack := make([]int, 0)
	i := start
	for i < end {
		tok := tokens[i]
		if tok == "(" {
			openStack = append(openStack, i)
		} else if tok == ")" {
			if len(openStack) == 0 {
				return nil, &ParseError{0, 0 /* tokenToCol(i)*/, `unmatched ")"`}
			}
			parenInfo = append(parenInfo, bracketInfo{openStack[len(openStack)-1], i})
			openStack = openStack[:len(openStack)-1]
		}
		i++
	}
	if len(openStack) != 0 {
		return nil, &ParseError{0, 0 /* tokenToCol(openStack[len(openStack)-1])*/, `unclosed "("`}
	}

	for _, paren := range parenInfo {
		// it's a call
		if paren.open-1 >= 0 && tokenIsId(tokens[paren.open-1]) {
			i := paren.open + 1

			args := make([]interface{}, 0)
			for j := i; j < end && j <= paren.end; j++ {
				if paren.open+1 == paren.end { // call with no arguments
					break
				}
				if i >= start && i < end {
					parsedInfo, found := parsed[i]
					if found {
						args = append(args, parsedInfo.node)
						i = parsedInfo.otherEnd + 2 // +1 would step on ","
						j = i
						continue
					}
				}
				tok := tokens[j]
				if tok == "," || j == paren.end {
					node, err := parseExprWithParen(parsed, tokens, i, j)
					if err != nil {
						return nil, err
					}
					i = j + 1
					args = append(args, node)
				}
			}
			idNode, good := parseToken(tokens[paren.open-1]).(IdName)
			if !good {
				return nil, &ParseError{0, 0, `expected an identifier`}
			}
			call := ProcCall{Callee: idNode, Args: args}
			parsed[paren.open-1] = parsedNode{call, paren.end}
			parsed[paren.end] = parsedNode{call, paren.open - 1}
		} else {
			node, err := parseExprUnit(parsed, tokens, paren.open+1, paren.end)
			if err != nil {
				return nil, err
			}
			parsed[paren.open] = parsedNode{node, paren.end}
			parsed[paren.end] = parsedNode{node, paren.open}
		}
	}

	outterMost, found := parsed[0]
	if found && outterMost.otherEnd == len(tokens)-1 {
		return outterMost.node, nil
	}
	return parseExprUnit(parsed, tokens, start, end)
}

// A unit does not contain unparsed parentheses
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
	// TODO: array literals
	if unicode.IsDigit(rune(s[0])) || s[0] == '-' {
		return Literal{Number, s}
	} else if s[0] == '"' {
		return Literal{String, s[1 : len(s)-1]}
	} else {
		return IdName(s)
	}
}

func tokenIsOperator(token string) bool {
	_, found := tokToOp[token]
	return found
}

func tokenIsId(token string) bool {
	firstCharGood := unicode.IsLetter(rune(token[0]))
	if !firstCharGood {
		return false
	}
	for _, c := range token {
		if !(unicode.IsLetter(c) || unicode.IsDigit(c)) {
			return false
		}
	}
	return true
}
