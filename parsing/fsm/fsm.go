package fsm

// This package implements a restricted form of DFA and NFA. The machines
// are only allowed one accept state. NFAs can only be constructed by
// making a DFA right-to-left.

type StateMachine interface {
	Feed(rune)
	Accepted() bool
}

type DFA struct {
	state       int // signed for the fail state
	rules       map[transCond]uint8
	acceptState int
}

type NFA struct {
	state       []uint8
	rules       map[transCond][]uint8
	acceptState uint8
}

type TransitionRule struct {
	Current uint8
	Char    rune
	To      uint8
}

type transCond struct {
	Current uint8
	Char    rune
}

type DFADescription struct {
	Rules       []TransitionRule
	AcceptState uint8
}

func (d *DFA) Feed(c rune) {
	if d.state == FailState {
		return
	}
	newState, found := d.rules[transCond{uint8(d.state), c}]
	if !found {
		d.state = FailState
	} else {
		d.state = int(newState)
	}
}

func (d *DFA) Accepted() bool {
	return d.state == d.acceptState
}

func (d DFA) State() int {
	return d.state
}

func NewForwardDFA(desc *DFADescription) DFA {
	var dfa DFA
	dfa.rules = make(map[transCond]uint8, len(desc.Rules))
	for _, rule := range desc.Rules {
		dfa.rules[transCond{rule.Current, rule.Char}] = rule.To
	}
	dfa.acceptState = int(desc.AcceptState)
	return dfa
}

func (m *NFA) Feed(c rune) {
	var newState []uint8
	for _, s := range m.state {
		ns, _ := m.rules[transCond{s, c}]
		newState = append(newState, ns...)
	}
	m.state = newState
}

func (m *NFA) Accepted() bool {
	for _, s := range m.state {
		if s == m.acceptState {
			return true
		}
	}
	return false
}

func NewBackwardNFA(desc *DFADescription) NFA {
	var dfa NFA
	dfa.state = []uint8{desc.AcceptState}
	dfa.rules = make(map[transCond][]uint8)
	for _, rule := range desc.Rules {
		cond := transCond{rule.To, rule.Char}
		dfa.rules[cond] = append(dfa.rules[cond], rule.Current)
	}
	dfa.acceptState = 0
	return dfa
}

func AdvanceAll(c rune, dfas ...StateMachine) int {
	accepted := -1
	for i, e := range dfas {
		e.Feed(c)
		if accepted == -1 && e.Accepted() {
			accepted = i
		}
	}
	return accepted
}

// A state in every DFA that traps and is not an accept state.
const FailState = -1
