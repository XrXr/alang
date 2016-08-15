package parser

import (
	"fmt"
	"unicode"
	"unicode/utf8"
)

var _ = fmt.Printf // for debugging. remove when done

func Parse(s string) (interface{}, error) {
	return nil, &ParseError{0, 0, "Not implemented"}
}

/*
   Walk over runes in a string, return index of first occurance where
   `pred` returns false. If pred returns true for all runes, `len(s)` is
   returned
*/
func walkUntilFalse(s []byte, pred func(c rune) bool) int {
	var i int
	var c byte
	found := false
	for i, c = range s {
		if !pred(rune(c)) {
			found = true
			break
		}
	}

	if !found {
		return len(s)
	}
	return i
}

func parseExpr(s []byte) (interface{}, error) {
	var oprandBuf [2]interface{}
	var operator Operator
	bufOffset := 0

	// set current operator and consolidate oprandBuf if necessary
	putOperator := func(op Operator) {
		if bufOffset == 2 {
			node := new(ExprNode)
			*node = ExprNode{operator, oprandBuf[0], oprandBuf[1]}
			oprandBuf[0] = *node
			bufOffset = 0
		} else if bufOffset == 0 { // unary operator
			bufOffset = 1
		}
		operator = op
	}

	putOprand := func(oprand interface{}) {
		oprandBuf[bufOffset] = oprand
		bufOffset++
	}

	for len(s) > 0 {
		r, rSize := utf8.DecodeRune(s)

		switch {
		case r == '+':
			putOperator(PLUS)
			s = s[rSize:]
		case r == '-':
			putOperator(MINUS)
			s = s[rSize:]
		case r == '*':
			putOperator(MULTIPLY)
			s = s[rSize:]
		// case '(':
		//     j := i + 1
		//     for ; j < len(s); j++ {
		//         if s[j] == ')' {
		//             node, err := parseExpr(s[i:j + 1])
		//             if err != nil {
		//                 return nil, err
		//             }
		//             if j == i + 1 {
		//                 operator = CALL
		//             } else {
		//                 oprandBuf[bufOffset] = node
		//                 bufOffset++
		//                 i = j + 1
		//             }
		//             break
		//         }
		//     }
		//     if j == len(s) {
		//         return nil, &ParseError{0, i, "Open parathesis not closed"}
		//     }
		case unicode.IsLetter(r):
			i := walkUntilFalse(s, func(c rune) bool {
				return unicode.IsLetter(c) || unicode.IsDigit(c)
			})

			putOprand(IdName(s[:i]))
			s = s[i:]
		case unicode.IsDigit(r):
			seenDot := false
			i := walkUntilFalse(s, func(c rune) bool {
				if !seenDot && c == '.' {
					seenDot = true
					return true
				}
				return unicode.IsDigit(c)
			})

			putOprand(Literal{NUMBER, string(s[:i])})
			s = s[i:]
		default:
			s = s[rSize:]
		}
	}

	return ExprNode{operator, oprandBuf[0], oprandBuf[1]}, nil
}
