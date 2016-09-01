package parser

type Parser struct {
	blockStack []interface{}
}

func (p *Parser) FeedLine(line string) (isComplete bool, node interface{}, parent interface{}, err error) {
	//TODO double tokenize
	tokens := tokenize(line)
	// glboal function decl
	if len(p.blockStack) == 0 && len(tokens) >= 3 && tokens[1] == "::" &&
		tokens[len(tokens)-1] == "{" {

	}
}
