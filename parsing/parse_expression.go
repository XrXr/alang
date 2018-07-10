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
	node     ASTNode
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

type lineParse struct {
	tokens     []string
	indices    []int
	lineNumber int
}

// source location helpers
func (l *lineParse) makeLoc() sourceLocation {
	return sourceLocation{line: l.lineNumber}
}

func (l *lineParse) startOfTok(tokIdx int) int {
	return l.indices[tokIdx]
}

func (l *lineParse) endOfTok(tokIdx int) int {
	return l.indices[tokIdx] + len(l.tokens[tokIdx]) - 1
}

func (l *lineParse) singleTokSourceLocation(tokIdx int) sourceLocation {
	info := l.makeLoc()
	info.startColumn = l.startOfTok(tokIdx)
	info.endColumn = l.endOfTok(tokIdx)
	return info
}

func (l *lineParse) singleTokError(idx int, message string) error {
	return errors.MakeError(l.startOfTok(1), l.endOfTok(1), message)
}

func (l *lineParse) makeIdent(idx int) IdName {
	return IdName{l.singleTokSourceLocation(idx), l.tokens[idx]}
}

// Errors from here are all based token index. The caller is responsible for translating the indices to source column numbers
func (l *lineParse) parseInStatementContext() (ASTNode, error) {
	parsed := make(map[int]parsedNode)
	tokens := l.tokens
	firstToken := tokens[0] // caller makes sure that there is at least one token
	nTokens := len(tokens)
	switch {
	case firstToken == "if":
		if l.tokens[nTokens-1] != "{" {
			return nil, l.singleTokError(0, "if statement must end in \"{\"")
		}
		if nTokens < 3 {
			return nil, l.singleTokError(0, "if statements need to have an expression")
		}
		parsed, err := l.parseExprWithParen(parsed, 1, nTokens-1)
		if err != nil {
			return nil, err
		}
		return IfNode{
			Condition: parsed,
		}, nil
	case firstToken == "return":
		if nTokens == 1 {
			return ReturnNode{sourceLocation: l.singleTokSourceLocation(0)}, nil
		}
		parsed, err := l.parseExprWithParen(parsed, 1, nTokens)
		if err != nil {
			return nil, err
		}
		return ReturnNode{
			Values: []ASTNode{parsed},
		}, nil
	case firstToken == "for":
		if tokens[nTokens-1] != "{" {
			return nil, l.singleTokError(0, "Loop header must end in \"{\"")
		}
		if nTokens == 2 {
			// "for {"
			return Loop{}, nil
		}
		parsed, err := l.parseExprWithParen(parsed, 1, nTokens-1)
		if err != nil {
			return nil, err
		}
		return Loop{
			Expression: parsed,
		}, nil
	case (firstToken == "else" && nTokens == 2 && tokens[1] == "{") ||
		(firstToken == "}" && nTokens == 3 && tokens[1] == "else" && tokens[2] == "{"):
		return ElseNode{}, nil
	case firstToken == "struct" && nTokens == 3 && tokens[2] == "{":
		if tokenIsId(tokens[1]) {
			loc := l.makeLoc()
			loc.startColumn = l.startOfTok(0)
			loc.endColumn = l.endOfTok(2)
			return StructDeclare{sourceLocation: loc, Name: l.makeIdent(1)}, nil
		} else {
			return nil, l.singleTokError(1, invalidDeclNameMessage)
		}
	case firstToken == "var":
		if nTokens < 3 {
			return nil, errors.MakeError(0, nTokens-1, "Incomplete declaration")
		}
		return l.parseDecl(1, nTokens)
	case firstToken == "break" && nTokens == 1:
		return BreakNode{l.singleTokSourceLocation(0)}, nil
	case firstToken == "continue" && nTokens == 1:
		return ContinueNode{l.singleTokSourceLocation(0)}, nil
	case firstToken == "}" && nTokens == 1:
		return BlockEnd{l.singleTokSourceLocation(0)}, nil
	}
	for index, tok := range tokens {
		op := tokToOp[tok]
		if op == Declare || op == ConstDeclare || op == Assign || op == PlusEqual || op == MinusEqual {
			left, err := l.parseExprWithParen(parsed, 0, index)
			if err != nil {
				return nil, err
			}
			right, err := l.parseExprWithParen(parsed, index+1, nTokens)
			if err != nil {
				return nil, err
			}
			node := ExprNode{Op: op, Left: left, Right: right}
			err = finishExprNode(&node, index)
			if err != nil {
				return nil, err
			}
			return node, nil
		}
	}
	return l.parseExprWithParen(parsed, 0, nTokens)
}

func finishExprNode(node *ExprNode, opTokIdx int) error {
	if node.Right == nil {
		return errors.MakeError(opTokIdx, opTokIdx, "This operator needs an operand to the right")
	}
	unary := isUnary[node.Op]
	if !unary && node.Left == nil {
		return errors.MakeError(opTokIdx, opTokIdx, "This operator needs an operand to the left")
	}
	if !unary {
		// caller sets it in this case
		node.startColumn = node.Left.GetStartColumn()
	}
	node.endColumn = node.Right.GetEndColumn()
	return nil
}

func (l *lineParse) parseDecl(start, end int) (ASTNode, error) {
	tokens := l.tokens
	typeDecl, err := l.parseTypeDecl(start+1, end)
	if err != nil {
		return nil, err
	}
	if !tokenIsId(tokens[start]) {
		return nil, errors.MakeError(start, start, invalidDeclNameMessage)
	}
	return Declaration{Name: l.makeIdent(start), Type: typeDecl}, nil
}

func (l *lineParse) parseInStructDeclContext() (ASTNode, error) {
	tokens := l.tokens
	if len(tokens) == 1 && tokens[0] == "}" {
		return BlockEnd{l.singleTokSourceLocation(0)}, nil
	}
	return l.parseDecl(0, len(tokens))
}

func (l *lineParse) parseTypeDecl(start, end int) (TypeDecl, error) {
	tokens := l.tokens
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
				containedType, err := l.parseTypeDecl(i+3, end)
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
		return TypeDecl{}, l.singleTokError(end-1, invalidDeclNameMessage)
	}
	return TypeDecl{LevelOfIndirection: indirect, Base: l.makeIdent(end - 1)}, nil
}

func (l *lineParse) genParenInfo(start int, end int) ([]bracketInfo, error) {
	tokens := l.tokens
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
				return nil, errors.MakeError(i, i, fmt.Sprintf(`unmatched "%s"`, tok))
			}
			if tok == ")" && tokens[openStack[len(openStack)-1]] != "(" {
				return nil, errors.MakeError(i, i, `unmatched ")"`)
			}
			if tok == "]" && tokens[openStack[len(openStack)-1]] != "[" {
				return nil, errors.MakeError(i, i, `unmatched "]"`)
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

func (l *lineParse) parseExprWithParen(parsed map[int]parsedNode, start, end int) (ASTNode, error) {
	tokens := l.tokens
	parenInfo, err := l.genParenInfo(start, end)
	if err != nil {
		return nil, err
	}

	for _, paren := range parenInfo {
		if paren.kind == square {
			if paren.open == start {
				panic("ice: asked to parse an expression that starts with [. What?")
			}
			node, err := l.parseExprUnit(parsed, paren.open+1, paren.end)
			if err != nil {
				return nil, err
			}
			parsed[paren.open] = parsedNode{node, paren.end}
			parsed[paren.end] = parsedNode{node, paren.open}
		} else if paren.kind == round && paren.open-1 >= start && tokenIsId(tokens[paren.open-1]) {
			// it's a call or a proc expression
			var node ASTNode
			var isForeignProc bool
			parsedStart := paren.open - 1
			parsedEnd := paren.end
			if tokens[paren.open-1] == "proc" {
				isForeignProc = paren.open-2 >= start && tokens[paren.open-2] == "foreign"
				proc, endOfProcExpr, err := l.parseProcExpr(parsed, paren, !isForeignProc)
				if err != nil {
					return nil, err
				}
				proc.IsForeign = isForeignProc
				node = *proc
				parsedEnd = endOfProcExpr
			} else {
				call, err := l.parseCallList(parsed, paren)
				if err != nil {
					return nil, err
				}
				node = *call
			}
			if isForeignProc {
				parsedStart = paren.open - 2
			}
			parsed[parsedStart] = parsedNode{node, parsedEnd}
			parsed[parsedEnd] = parsedNode{node, paren.open - 1}
		} else {
			node, err := l.parseExprUnit(parsed, paren.open+1, paren.end)
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
	return l.parseExprUnit(parsed, start, end)
}

// A unit does not contain unparsed parentheses
func (l *lineParse) parseExprUnit(parsed map[int]parsedNode, start, end int) (ASTNode, error) {
	tokens := l.tokens
	if start == end {
		return nil, nil
	}
	if end-start == 1 {
		return l.parseToken(start), nil
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
				newNode.Left = l.parseToken(leftI)
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
				newNode.Right = l.parseToken(opTok.index + 1)
			}
		}

		err := finishExprNode(&newNode, opTok.index)
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

func (l *lineParse) parseCallList(parsed map[int]parsedNode, paren bracketInfo) (*ProcCall, error) {
	tokens := l.tokens
	i := paren.open + 1

	args := make([]ASTNode, 0)
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
			node, err := l.parseExprWithParen(parsed, i, j)
			if err != nil {
				return nil, err
			}
			i = j + 1
			args = append(args, node)
		}
	}
	// caller checks whether this is valid ident
	idNode := l.makeIdent(paren.open - 1)
	loc := l.makeLoc()
	loc.startColumn = l.startOfTok(paren.open - 1)
	loc.endColumn = l.endOfTok(paren.end)

	return &ProcCall{Callee: idNode, Args: args}, nil
}

func (l *lineParse) parseProcExpr(parsed map[int]parsedNode, paren bracketInfo, requireBlock bool) (*ProcDecl, int, error) {
	tokens := l.tokens
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
				return nil, 0, l.singleTokError(j-1, `This should be a type declaration`)
			} else {
				parsed, err := l.parseDecl(leftBoundary, j)
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
	returnType := TypeDecl{Base: IdName{Name: "void"}}
	if paren.end+1 < len(tokens) && tokens[paren.end+1] == "->" {
		if paren.end+2 >= len(tokens) || tokens[paren.end+2] == "{" {
			return nil, 0, errors.MakeError(paren.end+1, paren.end+1, "A return type should come after this")
		}
		var err error
		returnType, err = l.parseTypeDecl(paren.end+2, declEnd)
		if err != nil {
			return nil, 0, err
		}
	}
	if !requireBlock {
		declEnd = len(tokens) - 1
	}
	return &ProcDecl{Return: returnType, Args: args}, declEnd, nil
}

func (l *lineParse) parseToken(tokIdx int) ASTNode {
	token := l.tokens[tokIdx]
	switch {
	case unicode.IsDigit(rune(token[0])) || token[0] == '-':
		return Literal{l.singleTokSourceLocation(tokIdx), Number, token}
	case token == "true" || token == "false":
		return Literal{l.singleTokSourceLocation(tokIdx), Boolean, token}
	case token[0] == '"':
		return Literal{l.singleTokSourceLocation(tokIdx), String, token[1 : len(token)-1]}
	case token == "nil":
		return Literal{sourceLocation: l.singleTokSourceLocation(tokIdx), Type: NilPtr}
	case tokenIsId(token):
		return l.makeIdent(tokIdx)
	}
	return nil
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
