package parser

import (
	"reflect"
	"testing"
)

var fixture = map[string][]string{
	"5+food":                 {"5", "+", "food"},
	"writting + parser":      {"writting", "+", "parser"},
	"(some + brackets)":      {"(", "some", "+", "brackets", ")"},
	"((willful)*saturation)": {"(", "(", "willful", ")", "*", "saturation", ")"},
	"minus-5":                {"minus", "-", "5"},
	"minus+-5":               {"minus", "+", "-", "5"},
	"minus*+5":               {"minus", "*", "+", "5"},
}

func TestTokenizer(t *testing.T) {
	for in, expect := range fixture {
		result := Tokenize(in)
		if !reflect.DeepEqual(result, expect) {
			t.Errorf("Failed to tokenize %v to %v. Got %v", in, expect, result)
		}
	}
}
