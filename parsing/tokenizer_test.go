package parsing

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
	"engage.jolly.cooperation":   {"engage", ".", "jolly", ".", "cooperation"},
	"minus.food.cat * pop":       {"minus", ".", "food", ".", "cat", "*", "pop"},
	"123.20 * pop":               {"123.20", "*", "pop"},
	"-3231.20 * pop":             {"-3231.20", "*", "pop"}, // negative in front
	"-3231 * pop":                {"-3231", "*", "pop"},
	"pop - -123.20":              {"pop", "-", "-123.20"}, // end in negative
	"pop - -12450":               {"pop", "-", "-12450"},
	"frac + -3231.20 * pop":      {"frac", "+", "-3231.20", "*", "pop"}, // negative in middle
	"29 + -3231 * pop":           {"29", "+", "-3231", "*", "pop"},
	"12.82 + foo * (bar - 1000)": {"12.82", "+", "foo", "*", "(", "bar", "-", "1000", ")"},
	"foo(joster, cat)":           {"foo", "(", "joster", ",", "cat", ")"},
	`12.82 + "fooser"`:           {"12.82", "+", `"fooser"`},
	`"fooser".cat`:               {`"fooser"`, ".", "cat"},
	`"fo\"oser\n".cat`:           {`"fo\"oser\n"`, ".", "cat"},
	"main :: proc () {":          {"main", "::", "proc", "(", ")", "{"},
	"proc () -> string {":        {"proc", "(", ")", "->", "string", "{"},
	"     if big {":              {"if", "big", "{"},
	"if big {":                   {"if", "big", "{"},
	"iffifodif":                  {"iffifodif"},
	"\t\t   {":                   {"{"},
}

func TestTokenizer(t *testing.T) {
	for in, expect := range fixture {
		result := Tokenize(in)
		if !reflect.DeepEqual(result, expect) {
			t.Errorf("Failed to tokenize %v. Got %#v", in, result)
		}
	}
}
