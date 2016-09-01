package parser

import (
	"strings"
	"unicode"
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
	",",
	":=",
	"=",
	"::",
	"{",
	"}",
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
				if in[j] == '-' && isDigit(safeCharAt(in, j+1)) &&
					(j == 0 || isSpace(safeCharAt(in, j-1))) {
					numLitEnd := iAfterNumLiteral(in, j+1)
					result = append(result, in[j:numLitEnd])
					i = iAfterWs(in, numLitEnd)
					found = true
					break outter
				}
				if in[j] == '\n' {
					result = append(result, in[i:j])
					i = j + 1
					found = true
					break outter
				}
				if strings.HasPrefix(in[j:], bound) {
					if i == j {
						result = append(result, bound)
					} else {
						result = append(result, strings.TrimSpace(in[i:j]), bound)
					}
					i = iAfterWs(in, j+len(bound))
					found = true
					break outter
				}
				if in[j] == '.' {
					found = true
					if isDigit(safeCharAt(in, j+1)) {
						// TODO this function should return error for "asd.123"
						k := j + 1
						for ; k < len(in); k++ {
							if !unicode.IsDigit(rune(in[k])) {
								break
							}
						}
						result = append(result, in[i:k])
						i = iAfterWs(in, k)

					} else {
						if i == j {
							result = append(result, ".")
						} else {
							result = append(result, strings.TrimSpace(in[i:j]), ".")
						}
						i = j + 1

					}
					break outter
				}
			}
		}
		if found {
			lastFoundIdx = i
		} else {
			i++
		}
	}
	endTok := strings.TrimSpace(in[lastFoundIdx:])
	if len(endTok) > 0 {
		result = append(result, endTok)
	}
	return result
}

func safeCharAt(s string, i int) byte {
	if i >= 0 && i < len(s) {
		return s[i]
	} else {
		return 0
	}
}

func iAfterWs(s string, i int) int {
	for i < len(s) && unicode.IsSpace(rune(s[i])) {
		i++
	}
	return i
}

func iAfterNumLiteral(s string, i int) int {
	seenDot := false
	for ; i < len(s); i++ {
		if s[i] == '.' {
			if seenDot {
				break
			} else {
				seenDot = true
			}
			continue
		}
		if !unicode.IsDigit(rune(s[i])) {
			break
		}
	}
	return i
}

func isSpace(b byte) bool {
	return unicode.IsSpace(rune(b))
}

func isDigit(b byte) bool {
	return unicode.IsDigit(rune(b))
}
