package parsing

import (
	"container/heap"
	"fmt"
	"github.com/XrXr/alang/errors"
	"strconv"
	"unicode"
)

var _ = fmt.Printf // for debugging. remove when done
const invalidDeclNameMessage = "Invalid name"

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

// Errors from here are all based token index. The caller is responsible for translating the indices to source column numbers
func ParseExpr(tokens []string) (interface{}, error) {
	parsed := make(map[int]parsedNode)
	if tokens[0] == "if" {
		if tokens[len(tokens)-1] != "{" {
			return nil, errors.MakeError(0, 0, "if statement must end in \"{\"")
		}
		if len(tokens) < 3 {
			return nil, errors.MakeError(0, 0, "if statements need to have an expression")
		}
		parsed, err := parseExprWithParen(parsed, tokens, 1, len(tokens)-1)
		if err != nil {
			return nil, err
		}
		return IfNode{
			Condition: parsed,
		}, nil
	} else if tokens[0] == "return" {
		if len(tokens) == 1 {
			return ReturnNode{}, nil
		}
		parsed, err := parseExprWithParen(parsed, tokens, 1, len(tokens))
		if err != nil {
			return nil, err
		}
		return ReturnNode{
			Values: []interface{}{parsed},
		}, nil
	} else if tokens[0] == "for" {
		if tokens[len(tokens)-1] != "{" {
			return nil, errors.MakeError(0, 0, "loop header must end in \"{\"")
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
		if len(tokens) < 3 {
			return nil, errors.MakeError(0, len(tokens)-1, "Incomplete declaration")
		}
		return parseDecl(tokens, 1, len(tokens))
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
			node := ExprNode{Op: op, Left: left, Right: right}
			err = checkLeftRight(&node, index)
			if err != nil {
				return nil, err
			}
			return node, nil
		}
	}
	return parseExprWithParen(parsed, tokens, 0, len(tokens))
}

func checkLeftRight(node *ExprNode, opTokIdx int) error {
	if node.Right == nil {
		return errors.MakeError(opTokIdx, opTokIdx, "This operator needs an operand to the right")
	}
	if !isUnary[node.Op] && node.Left == nil {
		return errors.MakeError(opTokIdx, opTokIdx, "This operator needs an operand to the left")
	}
	return nil
}

func parseDecl(tokens []string, start, end int) (interface{}, error) {
	typeDecl, err := parseTypeDecl(tokens, start+1, end)
	if err != nil {
		return nil, err
	}
	if !tokenIsId(tokens[start]) {
		return nil, errors.MakeError(start, start, invalidDeclNameMessage)
	}
	return Declaration{Name: IdName(tokens[start]), Type: typeDecl}, nil
}

func parseStructMembers(tokens []string) (interface{}, error) {
	if len(tokens) == 1 && tokens[0] == "}" {
		return BlockEnd(0), nil
	}
	return parseDecl(tokens, 0, len(tokens))
}

func parseTypeDecl(tokens []string, start int, end int) (TypeDecl, error) {
	indirect := 0
	var sizes []int
	i := start
	for i < end {
		tok := tokens[i]
		if tok == "*" {
			indirect++
		} else {
			if i+2 < end && tok == "[" && tokens[i+2] == "]" {
				arraySize, err := strconv.Atoi(tokens[i+1])
				if err != nil {
					return TypeDecl{}, errors.MakeError(i, i+2, "Array size must be an integer literal")
				}
				// three because that's the number of tokens [323] parses to
				if i+3 >= end {
					return TypeDecl{}, errors.MakeError(i, end-1, "Arrays must contain some type")
				}
				sizes = append(sizes, arraySize)
				if tokens[i+3] == "[" {
					i = i + 3
					continue
				}
				containedType, err := parseTypeDecl(tokens, i+3, end)
				if err != nil {
					return TypeDecl{}, err
				}
				return TypeDecl{
					LevelOfIndirection: indirect,
					ArraySizes:         sizes,
					ArrayBase:          &containedType,
				}, nil
			} else if i != end-1 {
				return TypeDecl{}, errors.MakeError(start, end-1, "This needs to be a type declaration")
			}
		}
		i++
	}
	base := tokens[end-1]
	if !tokenIsId(base) {
		return TypeDecl{}, errors.MakeError(end-1, end-1, invalidDeclNameMessage)
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
				return nil, errors.MakeError(0, 0, fmt.Sprintf(`unmatched "%s"`, tok))
			}
			if tok == ")" && tokens[openStack[len(openStack)-1]] != "(" {
				return nil, errors.MakeError(0, 0, `unmatched ")"`)
			}
			if tok == "]" && tokens[openStack[len(openStack)-1]] != "[" {
				return nil, errors.MakeError(0, 0, `unmatched "]"`)
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
		return nil, errors.MakeError(openStack[0], openStack[0], `unclosed bracket`)
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
				panic("ice: asked to parse an expression that starts with [. What?")
			}
			node, err := parseExprUnit(parsed, tokens, paren.open+1, paren.end)
			if err != nil {
				return nil, err
			}
			parsed[paren.open] = parsedNode{node, paren.end}
			parsed[paren.end] = parsedNode{node, paren.open}
		} else if paren.kind == round && paren.open-1 >= start && tokenIsId(tokens[paren.open-1]) {
			// it's a call or a proc expression
			var node interface{}
			var isForeignProc bool
			parsedStart := paren.open - 1
			parsedEnd := paren.end
			if tokens[paren.open-1] == "proc" {
				isForeignProc = paren.open-2 >= start && tokens[paren.open-2] == "foreign"
				proc, endOfProcExpr, err := parseProcExpr(tokens, parsed, paren, !isForeignProc)
				if err != nil {
					return nil, err
				}
				proc.IsForeign = isForeignProc
				node = *proc
				parsedEnd = endOfProcExpr
			} else {
				call, err := parseCallList(tokens, parsed, paren)
				if err != nil {
					return nil, err
				}
				node = *call
			}
			if isForeignProc {
				parsedStart = paren.open - 2
				println(parsedStart == start, parsedEnd == end-1, parsedEnd, end-1)

			}
			parsed[parsedStart] = parsedNode{node, parsedEnd}
			parsed[parsedEnd] = parsedNode{node, paren.open - 1}
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
	if found && outterMost.otherEnd == end-1 {
		return outterMost.node, nil
	}
	return parseExprUnit(parsed, tokens, start, end)
}

// A unit does not contain unparsed parentheses
func parseExprUnit(parsed map[int]parsedNode, tokens []string, start int, end int) (interface{}, error) {
	if start == end {
		return nil, nil
	}
	if end-start == 1 {
		return parseToken(tokens[start]), nil
	}
	var ops OpHeap
	heap.Init(&ops)

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
		if oneToLeftValid && !isUnary[newNode.Op] {
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

		err := checkLeftRight(&newNode, opTok.index)
		if err != nil {
			return nil, err
		}

		parsed[leftI] = parsedNode{newNode, rightI}
		parsed[rightI] = parsedNode{newNode, leftI}
	}
	orphanOperandMessage := "This operand isn't consumed by any operator"
	if info, startIsParsed := parsed[start]; startIsParsed {
		if info.otherEnd == end-1 {
			return info.node, nil
		} else {
			for i := start; i < end; i++ {
				_, parsed := parsed[i]
				if !parsed {
					return nil, errors.MakeError(i, i, orphanOperandMessage)
				}
			}
			panic(fmt.Sprintf("ice: inconsistency in parsed map: whole range isn't covered but a node that isn't parsed can't be found in the map"))
		}
	} else {
		return nil, errors.MakeError(start, start, orphanOperandMessage)
	}
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
	// caller needs to make sure that this works
	idNode := parseToken(tokens[paren.open-1]).(IdName)

	return &ProcCall{Callee: idNode, Args: args}, nil
}

func parseProcExpr(tokens []string, parsed map[int]parsedNode, paren bracketInfo, requireBlock bool) (*ProcNode, int, error) {
	declEnd := -1
	if requireBlock {
		for i := paren.end + 1; i < len(tokens); i++ {
			if tokens[i] == "{" {
				declEnd = i
				break
			}
		}
		if declEnd == -1 {
			return nil, 0, errors.MakeError(paren.open-1, paren.end, "Proc expressions must have a body")
		}
	} else {
		declEnd = len(tokens)
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
				return nil, 0, errors.MakeError(j-1, j-1, `This should be a type declaration`)
			} else {
				parsed, err := parseDecl(tokens, leftBoundary, j)
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
		if paren.end+2 >= len(tokens) || tokens[paren.end+2] == "{" {
			return nil, 0, errors.MakeError(paren.end+1, paren.end+1, "A return type should come after this")
		}
		var err error
		returnType, err = parseTypeDecl(tokens, paren.end+2, declEnd)
		if err != nil {
			return nil, 0, err
		}
	}
	if !requireBlock {
		declEnd = len(tokens) - 1
	}
	return &ProcNode{
		ProcDecl: ProcDecl{Return: returnType, Args: args}}, declEnd, nil
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
