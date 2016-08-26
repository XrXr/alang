package parser

import (
	"strings"
)

var bounderies = [...]string{
	"+",
	"-",
	"*",
	"/",
	"#",
	"&",
	"^",
	"%",
	"$",
	"|",
	"(",
	")",
}

func Tokenize(in string) []string {
	var result []string
	var lastFoundIdx int
	i := 0
	for i < len(in) {
		found := false
	outter:
		for j := i; j < len(in); j++ {
			for _, bound := range bounderies {
				if strings.HasPrefix(in[j:], bound) {
					strPart := strings.TrimSpace(in[i:j])
					if len(strPart) > 0 {
						result = append(result, strPart, bound)
					} else {
						result = append(result, bound)
					}
					i = j + len(bound)
					lastFoundIdx = i
					found = true
					break outter
				}
			}
		}
		if !found {
			i++
		}
	}
	endTok := strings.TrimSpace(in[lastFoundIdx:])
	if len(endTok) > 0 {
		result = append(result, endTok)
	}
	return result
}
