package fsm

import (
	"reflect"
	"runtime"
	"testing"
)

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

var validIds = []string{
	"abundant",
	"fo12o",
	"Live_unTil_55",
	"SomeName",
	"foodSource",
	"_",
	"__meta__",
	"collide_",
}

func TestIdentifierNames(t *testing.T) {
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

var validBinLiterals = []string{
	"0b0010010",
	"0b0",
	"0b1",
	"0b1111111",
	"0b11",
	"0b00",
}

var invalidBinLiterals = []string{
	"0b",
	"0B",
	"0b0123",
	"0B12",
}

var validHexLiterals = []string{
	"0x3fabcd",
	"0x3FACADE",
	"0xfA3CADE",
	"0x218912",
	"0xabcdef",
	"0xABCDEF",
	"0xaBCDeF",
}

var invalidHexLiterals = []string{
	"0x",
	"0X",
	"0xXXxxX",
	"0xabcg",
}

var validDecimalLiterals = []string{
	"12450",
	"0",
	"0000",
	"040506",
}

var validFloatLiterals = []string{
	"3.1415926",
	".7592",
	".0",
	"0.0",
	"0.00000",
}

var invalidFloatLiterals = []string{
	".",
	"..7592",
	"..0",
	"0..7592",
	"0..0",
	"0.123123.0",
	".123123.",
	"0.123123.",
}

func TestNumericLiterals(t *testing.T) {
	testMachine(t, BinaryLiteral, validBinLiterals, true)
	testMachine(t, BinaryLiteral, invalidBinLiterals, false)
	testMachine(t, BinaryLiteral, validFloatLiterals, false)
	testMachine(t, BinaryLiteral, validDecimalLiterals, false)

	testMachine(t, HexLiteral, validHexLiterals, true)
	testMachine(t, HexLiteral, invalidHexLiterals, false)
	testMachine(t, HexLiteral, validIds, false)
	testMachine(t, HexLiteral, validDecimalLiterals, false)

	testMachine(t, FloatLiteral, validFloatLiterals, true)
	testMachine(t, FloatLiteral, invalidFloatLiterals, false)
	testMachine(t, FloatLiteral, validDecimalLiterals, false)

	testMachine(t, DecimalLiteral, validDecimalLiterals, true)
	testMachine(t, DecimalLiteral, invalidFloatLiterals, false)
	testMachine(t, DecimalLiteral, validHexLiterals, false)
}

// Only works with ascii encoding. Why is this not in the standard lib?
func reverse(s string) string {
	arr := []byte(s)
	for i, j := 0, len(arr)-1; j > i; i, j = i+1, j-1 {
		arr[i], arr[j] = arr[j], arr[i]
	}
	return string(arr)
}

func testMachine(t *testing.T, description func() *DFADescription, fixtureList []string, expectedAcceptValue bool) {
	for _, fixture := range fixtureList {
		forward := NewForwardDFA(description())
		backward := NewBackwardNFA(description())

		for _, c := range fixture {
			forward.Feed(c)
		}
		for _, c := range reverse(fixture) {
			backward.Feed(c)
		}

		descName := runtime.FuncForPC(reflect.ValueOf(description).Pointer()).Name()
		verb := "accepted"
		if expectedAcceptValue == false {
			verb = "rejected"
		}

		if forward.Accepted() != expectedAcceptValue {
			t.Errorf(`"%s" should be %s by %s`, fixture, verb, descName)
		}
		if forward.Accepted() != expectedAcceptValue {
			t.Errorf(`"%s" should be %s by the backward NFA of %s`, fixture, verb, descName)
		}
	}
}
