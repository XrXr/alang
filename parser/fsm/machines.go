package fsm

import (
	"strings"
	"unicode"
)

var digits = []rune("01234567890")

var identifierNameCache *DFADescription

func IdentifierName() *DFADescription {
	if identifierNameCache != nil {
		return identifierNameCache
	}
	lowerAlpha := "abcdefghijklnmopqrstuvwxyz"
	alphabet := []rune(lowerAlpha)
	upperAlphabet := []rune(strings.ToUpper(lowerAlpha))

	alphaOnly := append(alphabet, upperAlphabet...)
	// firstChar is alpha or underscore
	startRules := make([]TransitionRule, len(alphaOnly)+1)
	startRules[0] = TransitionRule{0, '_', 1}
	i := 1
	for _, c := range alphaOnly {
		startRules[i] = TransitionRule{0, c, 1}
		i++
	}

	// alpha or ditgit or underscore after the first char
	fullList := append(alphaOnly, digits...)
	afterFirst := make([]TransitionRule, len(fullList)+1)
	afterFirst[0] = TransitionRule{1, '_', 1}
	i = 1

	for _, c := range fullList {
		afterFirst[i] = TransitionRule{1, c, 1}
		i++
	}

	identifierNameCache = new(DFADescription)
	identifierNameCache.Rules = append(startRules, afterFirst...)
	identifierNameCache.AcceptState = 1
	return identifierNameCache
}

func BinaryLiteral() *DFADescription {
	desc := new(DFADescription)
	desc.Rules = []TransitionRule{
		{0, '0', 1},
		{1, 'b', 2},
		{2, '0', 3},
		{2, '1', 3},
		{3, '0', 3},
		{3, '1', 3},
	}
	desc.AcceptState = 3
	return desc
}

var hexLiteralCache *DFADescription

func HexLiteral() *DFADescription {
	if hexLiteralCache != nil {
		return hexLiteralCache
	}
	var desc DFADescription
	hexDigits := []rune("abcdef")
	desc.Rules = make([]TransitionRule, 2+2*(2*len(hexDigits)+len(digits)))

	desc.Rules[0] = TransitionRule{0, '0', 1}
	desc.Rules[1] = TransitionRule{1, 'x', 2}
	i := 2

	for _, c := range digits {
		desc.Rules[i] = TransitionRule{2, c, 3}
		i++
		desc.Rules[i] = TransitionRule{3, c, 3}
		i++
	}
	for _, c := range hexDigits {
		desc.Rules[i] = TransitionRule{2, c, 3}
		i++
		desc.Rules[i] = TransitionRule{3, c, 3}
		i++
		upper := unicode.To(unicode.UpperCase, c)
		desc.Rules[i] = TransitionRule{2, upper, 3}
		i++
		desc.Rules[i] = TransitionRule{3, upper, 3}
		i++
	}
	desc.AcceptState = 3
	hexLiteralCache = &desc
	return &desc
}

var floatLiteralCache *DFADescription

func FloatLiteral() *DFADescription {
	if floatLiteralCache != nil {
		return floatLiteralCache
	}
	desc := new(DFADescription)
	desc.Rules = make([]TransitionRule, 3*len(digits)+1)
	desc.Rules[0] = TransitionRule{0, '.', 1}
	i := 1
	for _, c := range digits {
		desc.Rules[i] = TransitionRule{0, c, 0}
		i++
		desc.Rules[i] = TransitionRule{1, c, 2}
		i++
		desc.Rules[i] = TransitionRule{2, c, 2}
		i++
	}
	desc.AcceptState = 2

	floatLiteralCache = desc
	return desc
}

var decimalLiteralCache *DFADescription

func DecimalLiteral() *DFADescription {
	if decimalLiteralCache != nil {
		return decimalLiteralCache
	}
	desc := new(DFADescription)
	desc.Rules = make([]TransitionRule, 2*len(digits))
	i := 0
	for _, c := range digits {
		desc.Rules[i] = TransitionRule{0, c, 1}
		i++
		desc.Rules[i] = TransitionRule{1, c, 1}
		i++
	}
	desc.AcceptState = 1
	decimalLiteralCache = desc
	return desc
}
