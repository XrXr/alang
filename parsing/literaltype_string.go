// Code generated by "stringer -type=LiteralType"; DO NOT EDIT.

package parsing

import "strconv"

const _LiteralType_name = "NumberStringArrayBooleanNilPtr"

var _LiteralType_index = [...]uint8{0, 6, 12, 17, 24, 30}

func (i LiteralType) String() string {
	i -= 1
	if i < 0 || i >= LiteralType(len(_LiteralType_index)-1) {
		return "LiteralType(" + strconv.FormatInt(int64(i+1), 10) + ")"
	}
	return _LiteralType_name[_LiteralType_index[i]:_LiteralType_index[i+1]]
}
