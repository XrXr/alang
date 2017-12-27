package parsing

type Parser struct {
	OutBuffer       []statement
	incompleteStack []*interface{}
}

type statement struct {
	IsComplete bool
	Node       *interface{}
	Parent     *interface{}
}

func (p *Parser) FeedLine(line string) (int, error) {
	before := len(p.OutBuffer)
	err := p.processLine(line)
	if err != nil {
		return 0, err
	}
	return len(p.OutBuffer) - before, nil
}

func (p *Parser) processLine(line string) error {
	var parent *interface{}
	getParent := func() *interface{} {
		l := len(p.incompleteStack)
		if l >= 1 {
			return p.incompleteStack[l-1]
		}
		return nil
	}
	addOne := func(isComplete bool, nodePtr *interface{}, parent *interface{}) {
		p.OutBuffer = append(p.OutBuffer, statement{isComplete, nodePtr, parent})
	}
	startNewBlock := func(node *interface{}) {
		parent = getParent()
		p.incompleteStack = append(p.incompleteStack, node)
	}
	tokens := Tokenize(line)
	n, err := ParseExpr(tokens)
	if err != nil {
		return err
	}
	switch t := n.(type) {
	case ExprNode:
		if t.Op == ConstDeclare {
			_, good := t.Right.(ProcNode)
			if good {
				startNewBlock(&n)
				addOne(false, &n, parent)
				return nil
			}
		}
	case IfNode:
		startNewBlock(&n)
		addOne(false, &n, parent)
		return nil
	case ElseNode:
		if tokens[0] == "}" {
			l := len(p.incompleteStack)
			if l == 0 {
				return &ParseError{0, 0, "Unmatched closing brace"}
			}
			top := p.incompleteStack[l-1]
			p.incompleteStack = p.incompleteStack[:l-1]
			var end interface{}
			end = BlockEnd(0)
			addOne(true, &end, top)
		}
		startNewBlock(&n)
		addOne(false, &n, parent)
		return nil
	case BlockEnd:
		l := len(p.incompleteStack)
		if l == 0 {
			return &ParseError{0, 0, "Unmatched closing brace"}
		}
		top := p.incompleteStack[l-1]
		p.incompleteStack = p.incompleteStack[:l-1]
		addOne(true, &n, top)
		return nil
	}
	addOne(true, &n, getParent())
	return nil
}
