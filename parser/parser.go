package parser

type Parser struct {
	incompleteStack []interface{}
}

func (p *Parser) FeedLine(line string) (isComplete bool, node interface{}, parent interface{}, err error) {
	getParent := func() (parent interface{}) {
		l := len(p.incompleteStack)
		if l >= 1 {
			parent = p.incompleteStack[l-1]
		}
		return
	}
	node, err = ParseExpr(line)
	if err != nil {
		return false, nil, nil, err
	}
	switch t := node.(type) {
	case ExprNode:
		if t.Op == ConstDeclare {
			_, good := t.Right.(ProcNode)
			if good {
				parent = getParent()
				p.incompleteStack = append(p.incompleteStack, t)

				return false, t, parent, nil
			}
		}
	case BlockEnd:
		l := len(p.incompleteStack)
		top := p.incompleteStack[l-1]
		p.incompleteStack = p.incompleteStack[:l-1]
		return true, top, getParent(), nil
	}
	return true, node, getParent(), nil
}
