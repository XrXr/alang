package frontend

import (
	"fmt"
	"github.com/XrXr/alang/errors"
	"github.com/XrXr/alang/ir"
	"github.com/XrXr/alang/parsing"
	"sync"
)

type ProcWorkOrder struct {
	Out       chan OptBlock
	In        []*parsing.ASTNode
	Name      string
	ProcDecl  parsing.ProcDecl
	UserError chan *errors.UserError
}

type OptBlock struct {
	NumberOfVars     int
	NumberOfArgs     int
	Opts             []ir.Inst
	NonTemporaryVars []int
}

type procGen struct {
	opts             []ir.Inst
	nextVarNum       int
	rootScope        *scope
	labelGen         *LabelIdGen
	nodeStack        []*parsing.ASTNode // keep track of what node we are generating for
	nonTemporaryVars []int
}

func (p *procGen) addOpt(opt ir.Inst) {
	opt.GeneratedFrom = *p.nodeStack[len(p.nodeStack)-1]
	p.opts = append(p.opts, opt)
}

func (p *procGen) pushCurrentlyGenerating(node *parsing.ASTNode) {
	p.nodeStack = append(p.nodeStack, node)
}
func (p *procGen) popCurrentlyGenerating(node *parsing.ASTNode) {
	length := len(p.nodeStack)
	if p.nodeStack[length-1] == node {
		p.nodeStack = p.nodeStack[:length-1]
	} else {
		panic("ice: popping an unexpected node from nodeStack")
	}
}

type LabelIdGen struct {
	sync.Mutex
	availableId int
}

func (g *LabelIdGen) GenLabel(template string) (ret string) {
	g.Lock()
	ret = fmt.Sprintf(template, g.availableId)
	g.availableId++
	g.Unlock()
	return
}

type scope struct {
	gen             *procGen
	parentScope     *scope
	varTable        map[string]int
	loopLabel       string
	firstVarInScope int
	// keep track of mutation of variables that are not local to the scope
	outOfScopeMutations *[]int
}

func (s *scope) inherit() *scope {
	// take care that since we grab nextVarNum here, if you create new vars
	// in the parent scope after calling this, the `firstVarInScope` for the
	// child scope would not be correct.
	sub := scope{
		gen:             s.gen,
		parentScope:     s,
		varTable:        make(map[string]int),
		loopLabel:       s.loopLabel,
		firstVarInScope: s.gen.nextVarNum,
	}
	// #speed
	return &sub
}

func (s *scope) addOpt(opt ir.Inst) {
	if s.outOfScopeMutations != nil {
		// if the opt mutates a var that's outside the loop
		if mut := ir.FindMutationVar(&opt); mut > -1 && mut < s.firstVarInScope {
			*s.outOfScopeMutations = append(*s.outOfScopeMutations, mut)
		}
	}
	s.gen.addOpt(opt)
}

func (s *scope) resolve(name string) (int, bool) {
	cur := s
	for cur != nil {
		varNum, found := cur.varTable[name]
		if found {
			return varNum, found
		} else {
			cur = cur.parentScope
		}
	}
	return 0, false
}

func (s *scope) newVar() int {
	current := s.gen.nextVarNum
	s.gen.nextVarNum++
	return current
}

func (s *scope) newNamedVar(name string) int {
	varNum := s.newVar()
	// fmt.Println(name, "has vn", varNum)
	s.varTable[name] = varNum
	s.gen.nonTemporaryVars = append(s.gen.nonTemporaryVars, varNum)
	return varNum
}
