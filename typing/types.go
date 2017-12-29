package typing

import (
	"github.com/XrXr/alang/parsing"
)

type TypeRecord interface {
	IsNumber() bool
}

type Void struct{ normalType }

type String struct{ normalType }

type Int struct{ integerType }

type S64 struct{ integerType }

type S32 struct{ integerType }

type S16 struct{ integerType }

type S8 struct{ integerType }

type U64 struct{ integerType }

type U32 struct{ integerType }

type U16 struct{ integerType }

type U8 struct{ integerType }

type Unresolved struct {
	normalType
	Ident parsing.IdName
}

type normalType struct{}

func (_ normalType) IsNumber() bool {
	return false
}

type integerType struct{}

func (_ integerType) IsNumber() bool {
	return true
}

// func BuiltinTypes() map[TypeName]bool {
// 	return map[TypeName]bool{
// 		"string": true,
// 		"void":   true,
// 		"int":    true,
// 		"s64":    true,
// 		"s32":    true,
// 		"s16":    true,
// 		"s8":     true,
// 		"u64":    true,
// 		"u32":    true,
// 		"u16":    true,
// 		"u8":     true,
// 	}
// }
