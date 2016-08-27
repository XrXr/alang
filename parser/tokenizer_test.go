package parser

import (
	"reflect"
	"testing"
)

var fixture = map[string][]string{
	"5+food":                     {"5", "+", "food"},
	"writting + parser":          {"writting", "+", "parser"},
	"(some + brackets)":          {"(", "some", "+", "brackets", ")"},
	"((willful)*saturation)":     {"(", "(", "willful", ")", "*", "saturation", ")"},
	"minus-5":                    {"minus", "-", "5"},
	"minus+-5":                   {"minus", "+", "-", "5"},
	"minus*+5":                   {"minus", "*", "+", "5"},
	"minus.food.cat * pop":       {"minus", ".", "food", ".", "cat", "*", "pop"},
	"123.20 * pop":               {"123.20", "*", "pop"},
	"-3231.20 * pop":             {"-3231.20", "*", "pop"}, // negative in front
	"-3231 * pop":                {"-3231", "*", "pop"},
	"pop - -123.20":              {"pop", "-", "-123.20"}, // end in negative
	"pop - -12450":               {"pop", "-", "-12450"},
	"frac + -3231.20 * pop":      {"frac", "+", "-3231.20", "*", "pop"}, // negative in middle
	"29 + -3231 * pop":           {"29", "+", "-3231", "*", "pop"},
	"12.82 + foo * (bar - 1000)": {"12.82", "+", "foo", "*", "(", "bar", "-", "1000", ")"},
}

func TestTokenizer(t *testing.T) {
	for in, expect := range fixture {
		result := Tokenize(in)
		if !reflect.DeepEqual(result, expect) {
			t.Errorf("Failed to tokenize %v. Got %#v", in, result)
		}
	}
}
