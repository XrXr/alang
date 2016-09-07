package parser

type Parser struct {
	incompleteStack []*interface{}
}

func (p *Parser) FeedLine(line string) (isComplete bool, node_ptr *interface{}, parent *interface{}, err error) {
	getParent := func() (parent *interface{}) {
		l := len(p.incompleteStack)
		if l >= 1 {
			parent = p.incompleteStack[l-1]
		}
		return
	}
	startNewBlock := func(node *interface{}) {
		parent = getParent()
		p.incompleteStack = append(p.incompleteStack, node)
	}
	n, err := ParseExpr(line)
	if err != nil {
		return false, nil, nil, err
	}
	switch t := n.(type) {
	case ExprNode:
		if t.Op == ConstDeclare {
			_, good := t.Right.(ProcNode)
			if good {
				startNewBlock(&n)
				return false, &n, parent, nil
			}
		}
	case IfNode:
		startNewBlock(&n)
		return false, &n, parent, nil
	case ElseNode:
		startNewBlock(&n)
		return false, &n, parent, nil
	case BlockEnd:
		l := len(p.incompleteStack)
		if l == 0 {
			return false, nil, nil, &ParseError{0, 0, "Unmatched closing brace"}
		}
		top := p.incompleteStack[l-1]
		p.incompleteStack = p.incompleteStack[:l-1]
		return true, &n, top, nil
	}
	return true, &n, getParent(), nil
}
