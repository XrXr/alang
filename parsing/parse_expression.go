package parsing

import (
	"container/heap"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

var _ = fmt.Printf // for debugging. remove when done

type parsedNode struct {
	node     interface{}
	otherEnd int
}

type bracketType int

const (
	round bracketType = iota
	square
)

type bracketInfo struct { //index to the opening bracket and the closing
	kind bracketType
	open int
	end  int
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
		if isUnary[o[i].op] {
			return o[i].index > o[j].index
		} else {
			return o[i].index < o[j].index
		}
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

func ParseExpr(tokens []string) (interface{}, error) {
	parsed := make(map[int]parsedNode)
	if tokens[0] == "if" {
		if tokens[len(tokens)-1] != "{" {
			return nil, &ParseError{0, 0, "if statement must end in {"}
		}
		if len(tokens) < 3 {
			return nil, &ParseError{0, 0, "Missing conditonal for if statement"}
		}
		parsed, err := parseExprWithParen(parsed, tokens, 1, len(tokens)-1)
		if err != nil {
			return nil, err
		}
		return IfNode{
			Condition: parsed,
		}, nil
	} else if tokens[0] == "return" {
		parsed, err := parseExprWithParen(parsed, tokens, 1, len(tokens))
		if err != nil {
			return nil, err
		}
		return ReturnNode{
			Values: []interface{}{parsed},
		}, nil
	} else if tokens[0] == "for" {
		if tokens[len(tokens)-1] != "{" {
			return nil, &ParseError{0, 0, "loop header must end in {"}
		}
		if len(tokens) == 2 {
			// "for {"
			return Loop{}, nil
		}
		parsed, err := parseExprWithParen(parsed, tokens, 1, len(tokens)-1)
		if err != nil {
			return nil, err
		}
		return Loop{
			Expression: parsed,
		}, nil
	} else if (tokens[0] == "else" && len(tokens) == 2 && tokens[1] == "{") ||
		(tokens[0] == "}" && len(tokens) == 3 && tokens[1] == "else" && tokens[2] == "{") {
		return ElseNode{}, nil
	} else if tokens[0] == "struct" && len(tokens) == 3 && tokens[2] == "{" {
		return StructDeclare{Name: IdName(tokens[1])}, nil
	} else if tokens[0] == "var" {
		return parseDecl(tokens[1:])
	} else if tokens[0] == "break" && len(tokens) == 1 {
		return BreakNode{}, nil
	} else if tokens[0] == "continue" && len(tokens) == 1 {
		return ContinueNode{}, nil
	}
	for index, tok := range tokens {
		op := tokToOp[tok]
		if op == Declare || op == ConstDeclare || op == Assign || op == PlusEqual || op == MinusEqual {
			left, err := parseExprWithParen(parsed, tokens, 0, index)
			if err != nil {
				return nil, err
			}
			right, err := parseExprWithParen(parsed, tokens, index+1, len(tokens))
			if err != nil {
				return nil, err
			}
			return ExprNode{Op: op, Left: left, Right: right}, nil
		}
	}
	return parseExprWithParen(parsed, tokens, 0, len(tokens))
}

func parseDecl(tokens []string) (interface{}, error) {
	typeDecl, err := parseTypeDecl(tokens[1:])
	if err != nil {
		return nil, err
	}
	return Declaration{Name: IdName(tokens[0]), Type: typeDecl}, nil
}

func parseStructMembers(tokens []string) (interface{}, error) {
	if len(tokens) == 1 && tokens[0] == "}" {
		return BlockEnd(0), nil
	}
	if len(tokens) >= 2 {
		return parseDecl(tokens)
	}
	return nil, &ParseError{0, 0, "Malformed type declaration. Expected a name and a type"}
}

func parseTypeDecl(tokens []string) (TypeDecl, error) {
	indirect := 0
	var base string
	for i, tok := range tokens {
		if tok == "*" {
			indirect++
		} else {
			if i+2 < len(tokens)-1 && tok == "[" && tokens[i+2] == "]" {
				arraySize, err := strconv.Atoi(tokens[i+1])
				if err != nil {
					return TypeDecl{}, &ParseError{0, 0, "Not a valid array size. Must be an integer"}
				}
				containedSegment := tokens[i+3:]
				if len(containedSegment) == 0 {
					return TypeDecl{}, &ParseError{0, 0, "Arrays must contain some type"}
				}
				containedType, err := parseTypeDecl(containedSegment)
				if err != nil {
					return TypeDecl{}, err
				}
				return TypeDecl{
					LevelOfIndirection: indirect,
					ArraySizes:         []int{arraySize},
					ArrayBase:          &containedType,
				}, nil
			} else if i != len(tokens)-1 {
				return TypeDecl{}, &ParseError{0, 0, "Junk after the pointed-to type name"}
			} else {
				base = tok
			}
		}
	}
	return TypeDecl{LevelOfIndirection: indirect, Base: IdName(base)}, nil
}

func genParenInfo(tokens []string, start int, end int) ([]bracketInfo, error) {
	parenInfo := make([]bracketInfo, 0)
	openStack := make([]int, 0)
	i := start
	for i < end {
		tok := tokens[i]
		switch tok {
		case "(", "[":
			openStack = append(openStack, i)
		case ")", "]":
			if len(openStack) == 0 {
				return nil, &ParseError{0, 0 /* tokenToCol(i)*/, fmt.Sprintf(`unmatched "%s"`, tok)}
			}
			if tok == ")" && tokens[openStack[len(openStack)-1]] != "(" {
				return nil, &ParseError{0, 0 /* tokenToCol(i)*/, `unmatched ")"`}
			}
			if tok == "]" && tokens[openStack[len(openStack)-1]] != "[" {
				return nil, &ParseError{0, 0 /* tokenToCol(i)*/, `unmatched "]"`}
			}
			kind := round
			if tok == "]" {
				kind = square
			}
			parenInfo = append(parenInfo, bracketInfo{kind, openStack[len(openStack)-1], i})
			openStack = openStack[:len(openStack)-1]
		}
		i++
	}
	if len(openStack) != 0 {
		return nil, &ParseError{0, 0 /* tokenToCol(openStack[len(openStack)-1])*/, `unclosed "("`}
	}
	return parenInfo, nil
}

func parseExprWithParen(parsed map[int]parsedNode, tokens []string, start int, end int) (interface{}, error) {
	parenInfo, err := genParenInfo(tokens, start, end)
	if err != nil {
		return nil, err
	}

	for _, paren := range parenInfo {
		if paren.kind == square {
			if paren.open == start {
				return nil, &ParseError{0, 0 /* tokenToCol(i)*/, `this bracket doesn't make sense here`}
			}
			// left, alreadyParsed := parsed[paren.open-1]
			// if !alreadyParsed {
			// 	if !tokenIsId(tokens[paren.open-1]) {
			// 		// I can't think of a situation where the left of the square bracket wouldn't already be parsed
			// 		return nil, &ParseError{0, 0 /* tokenToCol(i)*/, `this bracket doesn't make sense here`}
			// 	}
			// 	left.node = parseToken(tokens[paren.open-1])
			// 	left.otherEnd = paren.open - 1
			// }
			node, err := parseExprUnit(parsed, tokens, paren.open+1, paren.end)
			if err != nil {
				return nil, err
			}
			// arrayAccessNode := ExprNode{ArrayAccess, left.node, node}
			parsed[paren.open] = parsedNode{node, paren.end}
			parsed[paren.end] = parsedNode{node, paren.open}
		} else if paren.kind == round && paren.open-1 >= start && tokenIsId(tokens[paren.open-1]) {
			// it's a call or a proc expression
			var node interface{}
			var isForeignProc bool
			end := paren.end
			if tokens[paren.open-1] == "proc" {
				isForeignProc = paren.open-2 >= start && tokens[paren.open-2] == "foreign"
				proc, blockStart, err := parseProcExpr(tokens, parsed, paren, !isForeignProc)
				if err != nil {
					return nil, err
				}
				proc.IsForeign = isForeignProc
				node = *proc
				end = blockStart
			} else {
				call, err := parseCallList(tokens, parsed, paren)
				if err != nil {
					return nil, err
				}
				node = *call
			}
			start := paren.open - 1
			if isForeignProc {
				start--
			}
			parsed[start] = parsedNode{node, end}
			parsed[end] = parsedNode{node, paren.open - 1}
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
		if found && tokens[i] != "[" {
			i = parsed.otherEnd + 1
			continue
		}
		tok := tokens[i]
		op, good := tokToOp[tok]
		if good {
			if _, hasPrecendence := precedence[op]; !hasPrecendence {
				println(op.String())
				panic("need precedence info for this to work")
			}
			heap.Push(&ops, opToken{i, op})
		}
		i++
		if tok == "(" {
			panic("parseExprUnit() shouldn't see any open parenthesis tokens")
		}
		if tok == "[" {
			if !found {
				panic("stuff inside [] should always be parsed already")
			}
			i = parsed.otherEnd + 1
		}
	}

	if len(ops) == 0 {
		parsed, found := parsed[start]
		if !found || parsed.otherEnd < end-1 {
			fmt.Printf("%v, %v\n", tokens, tokens[start:end])
			return nil, &ParseError{start, end, "Expected an operator"}
		}
		return parsed.node, nil
	}

	var finalNode ExprNode
	for len(ops) > 0 {
		opTok := heap.Pop(&ops).(opToken)
		leftI := opTok.index - 1
		rightI := opTok.index + 1

		if isUnary[opTok.op] {
			// We store the parsed node at the boundary index of the expression.
			// The boundary for a unary operator is the op token.
			leftI = opTok.index
		}

		var newNode ExprNode
		newNode.Op = opTok.op
		oneToLeftValid := opTok.index > 0 && opTok.index > start
		oneToRightValid := opTok.index < len(tokens)-1 && opTok.index < end
		if oneToLeftValid && opTok.op != Dereference {
			parsed, good := parsed[opTok.index-1]
			if good {
				newNode.Left = parsed.node
				leftI = parsed.otherEnd
			} else {
				newNode.Left = parseToken(tokens[leftI])
			}
		}
		if oneToRightValid {
			rightIndex := opTok.index + 1
			if opTok.op == ArrayAccess {
				rightIndex = opTok.index
			}
			parsed, good := parsed[rightIndex]
			if opTok.op == ArrayAccess && !good {
				panic("things inside [] should be parsed")
			}
			if good {
				newNode.Right = parsed.node
				rightI = parsed.otherEnd
			} else {
				newNode.Right = parseToken(tokens[opTok.index+1])
			}
		}

		finalNode = newNode

		if oneToLeftValid {
			parsed[leftI] = parsedNode{newNode, rightI}
		}
		if oneToRightValid {
			parsed[rightI] = parsedNode{newNode, leftI}
		}
	}
	return finalNode, nil
}

func parseCallList(tokens []string, parsed map[int]parsedNode, paren bracketInfo) (*ProcCall, error) {
	i := paren.open + 1

	args := make([]interface{}, 0)
	for j := i; j <= paren.end; j++ {
		if paren.open+1 == paren.end { // call with no arguments
			break
		}
		parsedInfo, found := parsed[i]
		if found {
			args = append(args, parsedInfo.node)
			i = parsedInfo.otherEnd + 2 // +1 would step on ","
			j = i
			continue
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

	return &ProcCall{Callee: idNode, Args: args}, nil
}

func parseProcExpr(tokens []string, parsed map[int]parsedNode, paren bracketInfo, requireBlock bool) (*ProcNode, int, error) {
	blockStart := -1
	if requireBlock {
		for i := paren.end + 1; i < len(tokens); i++ {
			if tokens[i] == "{" {
				blockStart = i
				break
			}
		}
		if blockStart == -1 {
			return nil, 0, &ParseError{0, 0, "Proc expressions must have a block"}
		}
	} else {
		blockStart = len(tokens)
	}
	i := paren.open + 1

	var args []Declaration
	leftBoundary := i
	for j := i; j <= paren.end; j++ {
		tok := tokens[j]
		if paren.open+1 == paren.end { // no arguments
			break
		}
		tokIsComma := tok == ","
		if tokIsComma || j == paren.end {
			var decl Declaration
			if j-leftBoundary == 1 {
				last := tokens[j-1]
				spaceIdx := strings.IndexRune(last, ' ')
				if spaceIdx == -1 {
					return nil, 0, &ParseError{0, 0, `expected a type declaration`}
				}
				decl = Declaration{
					Type: TypeDecl{Base: IdName(last[spaceIdx+1:])},
					Name: IdName(last[:spaceIdx]),
				}
			} else {
				parsed, err := parseDecl(tokens[leftBoundary:j])
				if err != nil {
					return nil, 0, err
				}
				decl = parsed.(Declaration)
			}
			args = append(args, decl)
		}
		if tokIsComma {
			leftBoundary = j + 1
		}
	}
	returnType := TypeDecl{Base: IdName("void")}
	if paren.end+1 < len(tokens) && tokens[paren.end+1] == "->" {
		if paren.end+2 >= len(tokens) {
			return nil, 0, &ParseError{0, 0, "Expected a type"}
		}
		var err error
		returnType, err = parseTypeDecl(tokens[paren.end+2 : blockStart])
		if err != nil {
			return nil, 0, err
		}
	}

	return &ProcNode{
		ProcDecl: ProcDecl{Return: returnType, Args: args}}, blockStart, nil
}

func parseToken(s string) interface{} {
	if tokenIsOperator(s) {
		return nil
	} else if s == "}" {
		return BlockEnd(0)
	} else if unicode.IsDigit(rune(s[0])) || s[0] == '-' {
		return Literal{Number, s}
	} else if s == "true" || s == "false" {
		return Literal{Boolean, s}
	} else if s[0] == '"' {
		return Literal{String, s[1 : len(s)-1]}
	} else if s == "nil" {
		return Literal{Type: NilPtr}
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
		if !(unicode.IsLetter(c) || unicode.IsDigit(c) || c == '_') {
			return false
		}
	}
	return true
}
