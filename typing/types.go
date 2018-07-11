package typing

import (
	"fmt"
	"github.com/XrXr/alang/parsing"
)

type TypeRecord interface {
	IsNumber() bool
	Size() int
	Rep() string
}

type String struct{ normalType }

func (_ String) Size() int {
	return 8
}

func (_ String) Rep() string {
	return "string"
}

type StringDataPointer struct{ normalType }

func (_ StringDataPointer) Size() int {
	return 8
}
func (_ StringDataPointer) Rep() string {
	return "pointer-to-string-data"
}

type Int struct{ integerType }

func (_ Int) Size() int {
	return 8
}
func (_ Int) Rep() string {
	return "int"
}

type Void struct{ normalType }

func (_ Void) Size() int {
	return 0
}
func (_ Void) Rep() string {
	return "void"
}

type Boolean struct{ normalType }

func (_ Boolean) Rep() string {
	return "boolean"
}
func (_ Boolean) Size() int {
	return 1
}

type U8 struct{ integerType }

func (_ U8) Size() int {
	return 1
}
func (_ U8) Rep() string {
	return "u8"
}

type S8 struct{ integerType }

func (_ S8) Rep() string {
	return "s8"
}
func (_ S8) Size() int {
	return 1
}

type U16 struct{ integerType }

func (_ U16) Size() int {
	return 2
}
func (_ U16) Rep() string {
	return "u16"
}

type S16 struct{ integerType }

func (_ S16) Size() int {
	return 2
}
func (_ S16) Rep() string {
	return "s16"
}

type U32 struct{ integerType }

func (_ U32) Size() int {
	return 4
}
func (_ U32) Rep() string {
	return "u32"
}

type S32 struct{ integerType }

func (_ S32) Size() int {
	return 4
}
func (_ S32) Rep() string {
	return "s32"
}

type U64 struct{ integerType }

func (_ U64) Size() int {
	return 8
}
func (_ U64) Rep() string {
	return "u64"
}

type S64 struct{ integerType }

func (_ S64) Size() int {
	return 8
}
func (_ S64) Rep() string {
	return "s64"
}

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
	alignment              int
	normalType
}

func (s StructRecord) Size() int {
	return s.size
}
func (s StructRecord) Rep() string {
	return s.Name
}

func (s *StructRecord) ResolveSizeAndOffset() {
	if s.SizeAndOffsetsResolved {
		return
	}
	s.size = 0
	if len(s.MemberOrder) == 0 {
		s.SizeAndOffsetsResolved = true
		return
	}
	s.MemberOrder[0].Offset = 0
	var biggestAlignment int
	for i, field := range s.MemberOrder {
		fieldSize := field.Type.Size()
		alignment := fieldSize
		switch fieldType := field.Type.(type) {
		case Array:
			switch arrContentType := fieldType.OfWhat.(type) {
			case *StructRecord:
				alignment = arrContentType.alignment
			default:
				alignment = arrContentType.Size()
			}
		case *StructRecord:
			alignment = fieldType.alignment
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
	s.alignment = biggestAlignment

	s.SizeAndOffsetsResolved = true
	s.PrintLayout()
}

func (s *StructRecord) PrintLayout() {
	fmt.Printf("struct \"%s\", size: %d, alignment: %d\n", s.Name, s.size, s.alignment)
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

func (u Unresolved) Rep() string {
	return fmt.Sprintf("(Unresolved %#v)", u.Decl)
}

type Pointer struct {
	normalType
	ToWhat TypeRecord
}

func (_ Pointer) Size() int {
	return 8
}

func (p Pointer) Rep() string {
	return "*" + p.ToWhat.Rep()
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

func (a Array) Rep() string {
	return fmt.Sprintf("[%d]%s", a.Nesting[0], a.OfWhat.Rep())
}

type normalType struct{}

func (_ normalType) IsNumber() bool {
	return false
}

type integerType struct{}

func (_ integerType) IsNumber() bool {
	return true
}
