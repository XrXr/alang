package typing

import (
	"fmt"
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

type S8 struct{ integerType }

func (_ S8) Size() int {
	return 1
}

type U8 struct{ integerType }

func (_ U8) Size() int {
	return 1
}

type S32 struct{ integerType }

func (_ S32) Size() int {
	return 4
}

type S16 struct{ integerType }

func (_ S16) Size() int {
	return 2
}

type U16 struct{ integerType }

func (_ U16) Size() int {
	return 2
}

type S64 struct{ integerType }

func (_ S64) Size() int {
	return 8
}

type U64 struct{ integerType }

func (_ U64) Size() int {
	return 8
}

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
	Name                   string
	Members                map[string]*StructField
	MemberOrder            []*StructField
	SizeAndOffsetsResolved bool
	size                   int
	normalType
}

func (s StructRecord) Size() int {
	return s.size
}

func (s *StructRecord) ResolveSizeAndOffset() {
	if s.SizeAndOffsetsResolved {
		return
	}
	s.MemberOrder[0].Offset = 0
	s.size = 0
	var biggestAlignment int
	for i, field := range s.MemberOrder {
		fieldSize := field.Type.Size()
		alignment := fieldSize
		if arrType, isArray := field.Type.(Array); isArray {
			alignment = arrType.OfWhat.Size()
		}
		if i > 0 {
			if (s.size % alignment) == 0 {
				s.MemberOrder[i].Offset = s.size
			} else {
				// cloest alignment
				s.MemberOrder[i].Offset = s.size - (s.size % alignment) + alignment
			}
		}
		s.size = s.MemberOrder[i].Offset + fieldSize
		if alignment > biggestAlignment {
			biggestAlignment = alignment
		}
	}
	if (s.size % biggestAlignment) != 0 {
		s.size = s.size - (s.size % biggestAlignment) + biggestAlignment
	}

	s.SizeAndOffsetsResolved = true
	s.PrintLayout()
}

func (s *StructRecord) PrintLayout() {
	fmt.Printf("struct \"%s\", size: %d\n", s.Name, s.size)
	for _, field := range s.MemberOrder {
		var name string
		for _name, _field := range s.Members {
			if field == _field {
				name = _name
			}
		}
		fmt.Printf("\tMember %s offset: %d\n", name, field.Offset)
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
