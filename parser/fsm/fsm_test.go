package fsm

import (
	"testing"
)

var validIds = [...]string{
	"abundant",
	"fo12o",
	"Live_unTil_55",
	"SomeName",
	"foodSource",
	"_",
	"__meta__",
	"collide_",
}

func TestAcceptedNames(t *testing.T) {
	for _, name := range validIds {
		forward := NewForwardDFA(IdentifierName())
		backward := NewBackwardNFA(IdentifierName())
		forwardBad := NewForwardDFA(IdentifierName())
		backwardBad := NewBackwardNFA(IdentifierName())

		forwardBad.Feed('0')
		for _, c := range name {
			forward.Feed(c)
			forwardBad.Feed(c)
		}
		for _, c := range reverse(name) {
			backward.Feed(c)
			backwardBad.Feed(c)
		}
		backwardBad.Feed('0')

		if !forward.Accepted() {
			t.Errorf(`"%s" rejected as id name`, name)
		}
		if !backward.Accepted() {
			backward.Feed('a')
			t.Errorf(`"%s" rejected as id name by backward NFA`, name)
		}

		badName := "0" + name
		if forwardBad.Accepted() {
			t.Errorf(`"%s" acceped as id name`, badName)
		}
		if backwardBad.Accepted() {
			t.Errorf(`"%s" accepted by backward NFA`, badName)
		}
	}
}

func TestDFA(t *testing.T) {
	fixture := DFADescription{
		Rules: []TransitionRule{
			{0, 'a', 1},
			{1, 'b', 0},
			{1, 'a', 2},
		},
		AcceptState: 2,
	}
	dfa := NewForwardDFA(&fixture)
	dfa.Feed('a')
	dfa.Feed('a')
	if !dfa.Accepted() || dfa.State() != 2 {
		t.Error()
		return
	}
	dfa.Feed('a')
	if dfa.Accepted() {
		t.Error()
		return
	}

	dfa = NewForwardDFA(&fixture)
	dfa.Feed('b')
	if dfa.State() != FailState {
		t.Error()
		return
	}
}

// Only works with ascii encoding. Why is this not in the standard lib?
func reverse(s string) string {
	arr := []byte(s)
	for i, j := 0, len(arr)-1; j > i; i, j = i+1, j-1 {
		arr[i], arr[j] = arr[j], arr[i]
	}
	return string(arr)
}
