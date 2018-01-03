package frontend

import (
	"fmt"
	"github.com/XrXr/alang/parsing"
	"sync"
)

type ProcWorkOrder struct {
	Out      chan OptBlock
	In       []*interface{}
	Name     parsing.IdName
	ProcDecl parsing.ProcDecl
}

type OptBlock struct {
	NumberOfVars int
	NumberOfArgs int
	Opts         []interface{}
}

type procGen struct {
	opts       []interface{}
	nextVarNum int
	rootScope  *scope
}

func (p *procGen) addOpt(opts ...interface{}) {
	p.opts = append(p.opts, opts...)
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
	gen         *procGen
	parentScope *scope
	varTable    map[parsing.IdName]int
}

func (s *scope) inherit() *scope {
	sub := scope{
		gen:         s.gen,
		parentScope: s,
		varTable:    make(map[parsing.IdName]int)}
	// #speed
	return &sub
}

func (s *scope) resolve(name parsing.IdName) (int, bool) {
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

func (s *scope) newNamedVar(name parsing.IdName) int {
	varNum := s.newVar()
	s.varTable[name] = varNum
	return varNum
}
