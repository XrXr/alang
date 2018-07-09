package parsing

import (
	"github.com/XrXr/alang/errors"
	"strings"
	"unicode"
)

// since the match happens from top to bottom, longer ones should come first
var bounderies = [...]string{
	"//",
	"[]",
	"->",
	"+=",
	"-=",
	"==",
	"+",
	"<=",
	"<",
	">=",
	"!=",
	"&&",
	"||",
	":=",
	"::",
	"..",
	".",
	">",
	"-",
	"*",
	"/",
	"#",
	"&",
	"^",
	"%",
	"$",
	"|",
	"@",
	"(",
	")",
	",",
	"=",
	"!",
	"[",
	"]",
	"{",
	"}",
}

func Tokenize(in string) ([]string, []int, error) {
	if in[len(in)-1] != '\n' {
		in = in + "\n"
	}
	var tokenList []string
	var indexList []int
	i := 0
	var tokenStart int
	startNewToken := true
	addToken := func(start int, end int) {
		tokenList = append(tokenList, in[start:end])
		indexList = append(indexList, start)
		startNewToken = true
		i = end
	}
tokenize:
	for {
		if startNewToken {
			i = iAfterWs(in, i)
			tokenStart = i
			startNewToken = false
		}
		if i >= len(in) {
			break
		}
		char := in[i]
		if char == '\n' {
			addToken(tokenStart, i)
			break
		}
		if unicode.IsSpace(rune(char)) {
			addToken(tokenStart, i)
			continue
		}
		if i == tokenStart {
			if char == '-' && isDigit(safeCharAt(in, i+1)) {
				numLitEnd := iAfterNumLiteral(in, i+1)
				addToken(tokenStart, numLitEnd)
				continue tokenize
			}
			if char == '"' {
				k := i + 1
				for k < len(in) {
					this := in[k]
					if this == '\\' {
						//TODO: multi-character escape sequence
						k += 2
						continue
					}
					if this == '"' {
						addToken(tokenStart, k+1)
						continue tokenize
					}
					k++
				}
				return nil, nil, errors.MakeError(i, i, "unmatched \"")
			}
		}
		if char == '.' && isDigit(safeCharAt(in, i+1)) {
			for j := tokenStart; j < i; j++ {
				if !isDigit(in[j]) {
					return nil, nil, errors.MakeError(i+1, i+1, "No struct member name can begin with a number")
				}
			}
			k := i + 1
			for ; k < len(in); k++ {
				if !unicode.IsDigit(rune(in[k])) {
					break
				}
			}
			addToken(tokenStart, k)
			continue tokenize
		}
		for _, bound := range bounderies {
			if strings.HasPrefix(in[i:], bound) {
				if i == tokenStart {
					addToken(tokenStart, tokenStart+len(bound))
				} else {
					addToken(tokenStart, i)
					addToken(i, i+len(bound))
				}
				continue tokenize
			}
		}
		i++
	}
	return tokenList, indexList, nil
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
