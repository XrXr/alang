package parsing

import (
	"fmt"
	"github.com/XrXr/alang/errors"
)

var _ = fmt.Printf

type parsingContext int

type Parser struct {
	OutBuffer       []statement
	incompleteStack []*ASTNode
	contextStack    []parsingContext
}

const (
	globalContext parsingContext = iota + 1
	structContext
)

type statement struct {
	IsComplete bool
	Node       *ASTNode
	Parent     *ASTNode
}

func (p *Parser) FeedLine(line string, lineNumber int) (int, error) {
	before := len(p.OutBuffer)
	err := p.processLine(line, lineNumber)
	if err != nil {
		if userError, isUserError := err.(*errors.UserError); isUserError {
			userError.Line = lineNumber
			return 0, userError
		}
		return 0, err
	}
	return len(p.OutBuffer) - before, nil
}

func (p *Parser) currentContext() parsingContext {
	return p.contextStack[len(p.contextStack)-1]
}

func (p *Parser) processLine(line string, lineNumber int) error {
	var parent *ASTNode
	getParent := func() *ASTNode {
		l := len(p.incompleteStack)
		if l >= 1 {
			return p.incompleteStack[l-1]
		}
		return nil
	}
	addOne := func(isComplete bool, nodePtr *ASTNode, parent *ASTNode) {
		p.OutBuffer = append(p.OutBuffer, statement{isComplete, nodePtr, parent})
	}
	startNewBlock := func(node *ASTNode) {
		parent = getParent()
		p.incompleteStack = append(p.incompleteStack, node)
	}
	tokens, indices, err := Tokenize(line)
	if err != nil {
		return err
	}
	// fmt.Printf("%#v\n", tokens) // Dump(tokens)
	if len(tokens) == 0 {
		return nil
	}
	if tokens[0] == "//" {
		return nil
	}
	lp := lineParse{
		tokens:     tokens,
		indices:    indices,
		lineNumber: lineNumber,
	}
	var n ASTNode
	if p.currentContext() == structContext {
		n, err = lp.parseInStructDeclContext()
	} else {
		n, err = lp.parseInStatementContext()
	}
	if err != nil {
		userError := err.(*errors.UserError)
		userError.StartColumn = indices[userError.StartColumn]
		userError.EndColumn = indices[userError.EndColumn] + len(tokens[userError.EndColumn]) - 1
		return err
	}
	// fmt.Printf("Line \"%s\" gave:\n", line)
	// Dump(n)
	switch t := n.(type) {
	case ExprNode:
		if t.Op == ConstDeclare {
			_, good := t.Right.(ProcDecl)
			if good {
				startNewBlock(&n)
				addOne(false, &n, parent)
				return nil
			}
		}
	case IfNode, Loop:
		startNewBlock(&n)
		addOne(false, &n, parent)
		return nil
	case ElseNode:
		if tokens[0] == "}" {
			l := len(p.incompleteStack)
			if l == 0 {
				return errors.MakeError(0, 0, "Unmatched closing brace")
			}
			top := p.incompleteStack[l-1]
			p.incompleteStack = p.incompleteStack[:l-1]
			var end ASTNode
			end = BlockEnd{lp.singleTokSourceLocation(0)}
			addOne(true, &end, top)
		}
		startNewBlock(&n)
		addOne(false, &n, parent)
		return nil
	case StructDeclare:
		p.contextStack = append(p.contextStack, structContext)
		addOne(false, &n, parent)
		startNewBlock(&n)
		return nil
	case Declaration:
	case BlockEnd:
		l := len(p.incompleteStack)
		if l == 0 {
			return errors.MakeError(0, 0, "Unmatched closing brace")
		}
		top := p.incompleteStack[l-1]
		p.incompleteStack = p.incompleteStack[:l-1]
		if _, parentIsStruct := (*top).(StructDeclare); parentIsStruct {
			p.contextStack = p.contextStack[:len(p.contextStack)-1]
		}
		addOne(true, &n, top)
		return nil
	}
	addOne(true, &n, getParent())
	return nil
}

func NewParser() *Parser {
	return &Parser{contextStack: []parsingContext{globalContext}}
}
