package parsing

import (
	"reflect"
	"testing"
)

var fixture = map[string][]string{
	"5+food":                       {"5", "+", "food"},
	"writting + parser":            {"writting", "+", "parser"},
	"(some + brackets)":            {"(", "some", "+", "brackets", ")"},
	"((willful)*saturation)":       {"(", "(", "willful", ")", "*", "saturation", ")"},
	"minus-5":                      {"minus", "-", "5"},
	"minus+-5":                     {"minus", "+", "-5"},
	"engage.jolly.cooperation":     {"engage", ".", "jolly", ".", "cooperation"},
	"minus.food.cat * pop":         {"minus", ".", "food", ".", "cat", "*", "pop"},
	"34..65":                       {"34", "..", "65"},
	"hap..jo":                      {"hap", "..", "jo"},
	"123.20 * pop":                 {"123.20", "*", "pop"},
	"-3231.20 * pop":               {"-3231.20", "*", "pop"}, // negative in front
	"-3231 * pop":                  {"-3231", "*", "pop"},
	"pop - -123.20":                {"pop", "-", "-123.20"}, // end in negative
	"pop - -12450":                 {"pop", "-", "-12450"},
	"frac + -3231.20 * pop":        {"frac", "+", "-3231.20", "*", "pop"}, // negative in middle
	"29 + -3231 * pop":             {"29", "+", "-3231", "*", "pop"},
	"12.82 + foo * (bar - 1000)":   {"12.82", "+", "foo", "*", "(", "bar", "-", "1000", ")"},
	"foo(joster, cat)":             {"foo", "(", "joster", ",", "cat", ")"},
	`12.82 + "fooser"`:             {"12.82", "+", `"fooser"`},
	`"fooser".cat`:                 {`"fooser"`, ".", "cat"},
	`"fo\"oser\n".cat`:             {`"fo\"oser\n"`, ".", "cat"},
	"main :: proc () {":            {"main", "::", "proc", "(", ")", "{"},
	"foo :: proc (a foo, b bar) {": {"foo", "::", "proc", "(", "a", "foo", ",", "b", "bar", ")", "{"},
	"proc () -> string {":          {"proc", "(", ")", "->", "string", "{"},
	"foreign proc () -> string {":  {"foreign", "proc", "(", ")", "->", "string", "{"},
	"     if big {":                {"if", "big", "{"},
	"if big {":                     {"if", "big", "{"},
	"var byte u8":                  {"var", "byte", "u8"},
	"iffifodif":                    {"iffifodif"},
	"nformats":                     {"nformats"},
	"return cats":                  {"return", "cats"},
	"return cats.dogs":             {"return", "cats", ".", "dogs"},
	` "progress 3/4" `:             {`"progress 3/4"`},
	` "progress 3+4" `:             {`"progress 3+4"`},
	` "progress" + 3 `:             {`"progress"`, `+`, `3`},
	"\t\t   {":                     {"{"},
}

func TestTokenizer(t *testing.T) {
	for in, expect := range fixture {
		tokens, indices, err := Tokenize(in)
		if err != nil {
			t.Errorf("Failed to tokenize %#v", in)
			continue
		}
		for i, token := range tokens {

			var tokenValueFromIdx string

			if idx := indices[i]; idx < len(in) && idx+len(token) <= len(in) {
				tokenValueFromIdx = in[idx : idx+len(token)]
			} else {
				tokenValueFromIdx = "##!!index out of bound!!##"
			}
			if tokenValueFromIdx != token {
				t.Errorf("Bad index for token %v. Value from that index is %#v instead of the claimed %#v", token, tokenValueFromIdx, token)
			}
		}
		if !reflect.DeepEqual(tokens, expect) {
			t.Errorf("Failed to tokenize [%v]. Got %#v", in, tokens)
		}
	}
}
