package typing

import (
	"github.com/XrXr/alang/parsing"
)

type TypeRecord interface {
	IsNumber() bool
	Size() int
}

type String struct{ normalType }

func (_ String) Size() int {
	return 8
}

type Int struct{ integerType }

func (_ Int) Size() int {
	return 8
}

type Void struct{ normalType }

func (_ Void) Size() int {
	return 0
}

type Boolean struct{ normalType }

func (_ Boolean) Size() int {
	return 1
}

type U8 struct{ integerType }

func (_ U8) Size() int {
	return 1
}

// type S64 struct{ integerType }

type S32 struct{ integerType }

func (_ S32) Size() int {
	return 4
}

// type S16 struct{ integerType }

// type S8 struct{ integerType }

// type U64 struct{ integerType }

type U32 struct{ integerType }

func (_ U32) Size() int {
	return 4
}

// type U16 struct{ integerType }

type StructField struct {
	Type   TypeRecord
	Offset int
}

type StructRecord struct {
	Name        string
	Members     map[string]*StructField
	MemberOrder []*StructField
	size        int
	normalType
}

func (s StructRecord) Size() int {
	return s.size
}

func (s *StructRecord) ResolveSizeAndOffset() {
	s.size = 0
	for _, field := range s.MemberOrder {
		s.size += field.Type.Size()
	}
	s.MemberOrder[0].Offset = 0
	for i := 1; i < len(s.MemberOrder); i++ {
		last := s.MemberOrder[i-1]
		s.MemberOrder[i].Offset = last.Offset + last.Type.Size()
	}
}

type Unresolved struct {
	normalType
	Decl parsing.TypeDecl
}

func (_ Unresolved) Size() int {
	return -999999999999
}

type Pointer struct {
	normalType
	ToWhat TypeRecord
}

func (_ Pointer) Size() int {
	return 8
}

type Array struct {
	normalType
	Nesting []int
	OfWhat  TypeRecord
}

func (a Array) Size() int {
	product := 1
	for _, size := range a.Nesting {
		product *= size
	}
	return product * a.OfWhat.Size()
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
