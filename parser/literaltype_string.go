// Code generated by "stringer -type=LiteralType"; DO NOT EDIT

package parser

import "fmt"

const _LiteralType_name = "NumberArray"

var _LiteralType_index = [...]uint8{0, 6, 11}

func (i LiteralType) String() string {
	i -= 1
	if i < 0 || i >= LiteralType(len(_LiteralType_index)-1) {
		return fmt.Sprintf("LiteralType(%d)", i+1)
	}
	return _LiteralType_name[_LiteralType_index[i]:_LiteralType_index[i+1]]
}
