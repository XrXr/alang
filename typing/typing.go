package typing

import (
	"errors"
	"fmt"
	"github.com/XrXr/alang/frontend"
	"github.com/XrXr/alang/ir"
	"github.com/XrXr/alang/parsing"
	"reflect"
)

type ProcRecord struct {
	Return    *TypeRecord
	Args      []TypeRecord
	IsForeign bool
}

type EnvRecord struct {
	Procs map[parsing.IdName]ProcRecord
	Types map[parsing.IdName]TypeRecord
}

type Typer struct {
	Builtins []TypeRecord
}

func (t *Typer) checkAndInferOpt(env *EnvRecord, opt ir.Inst, typeTable []TypeRecord) error {
	resolve := func(opt ir.Inst) (TypeRecord, TypeRecord) {
		l := typeTable[opt.Left()]
		r := typeTable[opt.Right()]
		return l, r
	}
	checkAndFindStructMemberType := func(baseVn int, fieldName string) TypeRecord {
		baseType := typeTable[baseVn]
		basePointer, baseIsPointer := baseType.(Pointer)
		if baseIsPointer {
			baseType = basePointer.ToWhat
		}
		baseStruct, baseIsStruct := baseType.(*StructRecord)
		baseIsString := baseType == t.Builtins[StringIdx]

		if !baseIsStruct && !baseIsString {
			panic("operand is not a struct or pointer to a struct")
		}
		var field *StructField
		if baseIsStruct {
			var ok bool
			field, ok = baseStruct.Members[fieldName]
			if !ok {
				panic(string(fieldName) + " is not a member of the struct")
			}
		} else if baseIsString {
			if fieldName != "data" && fieldName != "length" {
				panic(string(fieldName) + " is not a member of the struct")
			}
		}
		if baseIsStruct {
			return field.Type
		} else if baseIsString {
			switch fieldName {
			case "length":
				return t.Builtins[IntIdx]
			case "data":
				return Pointer{ToWhat: t.Builtins[U8Idx]}
			}
		}
		panic("should be exhaustive")
		return nil
	}
	giveTypeOrVerify := func(target int, typeRecord TypeRecord) {
		currentType := typeTable[target]
		if currentType == nil {
			typeTable[target] = typeRecord
		} else {
			if !t.TypesCompatible(currentType, typeRecord) {
				parsing.Dump(currentType)
				parsing.Dump(typeRecord)
				panic("type a is incompatible with type b")
			}
		}
	}
	switch opt.Type {
	case ir.AssignImm:
		finalType := t.typeImmediate(opt.Extra)
		if unresolved, isUnresolved := finalType.(Unresolved); isUnresolved {
			name := GrabUnresolvedName(unresolved)
			structRecord, ok := env.Types[name]
			if !ok {
				panic(string(name + " does not name a type"))
			}
			finalType = BuildRecordAccordingToUnresolved(structRecord, unresolved)
		}
		giveTypeOrVerify(opt.Out(), finalType)
	case ir.TakeAddress:
		varType := typeTable[opt.In()]
		if varType == nil {
			panic("type should be resolved at this point")
		}
		typeTable[opt.Out()] = Pointer{ToWhat: varType}
	case ir.PeelStruct:
		fieldName := opt.Extra.(string)
		fieldType := checkAndFindStructMemberType(opt.In(), fieldName)
		_, fieldIsPointer := fieldType.(Pointer)
		var outType TypeRecord
		if fieldIsPointer {
			outType = fieldType
		} else {
			outType = Pointer{ToWhat: fieldType}
		}
		giveTypeOrVerify(opt.Out(), outType)
	case ir.StructMemberPtr:
		getDoublePtrToStringData := false
		if opt.Extra.(string) == "data" {
			switch inType := typeTable[opt.In()].(type) {
			case Pointer:
				_, ptrToString := inType.ToWhat.(String)
				getDoublePtrToStringData = ptrToString
			case String:
				getDoublePtrToStringData = true
			}
		}
		if getDoublePtrToStringData {
			giveTypeOrVerify(opt.Out(), StringDataPointer{})
		} else {
			outType := checkAndFindStructMemberType(opt.In(), opt.Extra.(string))
			outType = Pointer{ToWhat: outType}
			giveTypeOrVerify(opt.Out(), outType)
		}
	case ir.Call:
		out := opt.Out()
		extra := opt.Extra.(ir.CallExtra)
		callee := parsing.IdName(extra.Name)
		typeRecord, callToType := env.Types[callee]
		if callToType {
			switch record := typeRecord.(type) {
			case *StructRecord:
				// making a struct
				giveTypeOrVerify(out, record)
			default:
				// type casting
				if len(extra.ArgVars) != 1 {
					panic("Type casts can only operate on one variable")
				}
				if typeRecord.Size() != typeTable[extra.ArgVars[0]].Size() {
					panic("Invalid cast: size of the types must match")
				}
				giveTypeOrVerify(out, typeRecord)
			}
		} else {
			procRecord, ok := env.Procs[callee]
			if !ok {
				panic("Call to undefined procedure " + extra.Name)
			}
			if len(extra.ArgVars) != len(procRecord.Args) {
				panic("Wrong number of argument for call to " + extra.Name)
			}
			for i, vn := range extra.ArgVars {
				if !t.TypesCompatible(typeTable[vn], procRecord.Args[i]) {
					parsing.Dump(typeTable[vn])
					parsing.Dump(procRecord.Args[i])
					panic(fmt.Sprintf("Argument %d of call to %s has incompatible type", i, extra.Name))
				}
			}

			giveTypeOrVerify(out, *procRecord.Return)
			return nil
		}
	case ir.Compare:
		extra := opt.Extra.(ir.CompareExtra)
		l := typeTable[opt.In()]
		r := typeTable[extra.Right]
		if !(l.IsNumber() && r.IsNumber()) {
			good := false
			if extra.How == ir.AreEqual || extra.How == ir.NotEqual {
				_, lIsBool := l.(Boolean)
				_, rIsBool := r.(Boolean)
				good = lIsBool && rIsBool
				_, lIsPointer := l.(Pointer)
				_, rIsPointer := r.(Pointer)
				good = good || (lIsPointer && rIsPointer)
			}
			if !good {
				parsing.Dump(l)
				parsing.Dump(r)
				return errors.New("operands must be numbers")
			}
		}
		typeTable[extra.Out] = t.Builtins[BoolIdx]
	case ir.IndirectWrite:
		varType := typeTable[opt.Left()]
		if varType == nil {
			panic("type should be resolved at this point")
		}
		typeForData := typeTable[opt.Right()]
		if typeForData == nil {
			panic("type should be resolved at this point")
		}
		pointer, varIsPointer := varType.(Pointer)
		if !varIsPointer {
			panic("That's not a pointer what are you doing")
		}
		if isVoidPointer(pointer) {
			panic("Can't indirect a void pointer")
		}
		if !t.TypesCompatible(pointer.ToWhat, typeForData) {
			parsing.Dump(pointer.ToWhat)
			parsing.Dump(typeForData)
			panic("Type mismatch")
		}
	case ir.IndirectLoad:
		ptrType := typeTable[opt.In()]
		if ptrType == nil {
			panic("type should be resolved at this point")
		}
		switch pointer := ptrType.(type) {
		case StringDataPointer:
			giveTypeOrVerify(opt.Out(), Pointer{ToWhat: t.Builtins[U8Idx]})
		case Pointer:
			pointer, isPointer := ptrType.(Pointer)
			if !isPointer {
				panic("Can't indirect a non pointer")
			}
			if isVoidPointer(pointer) {
				panic("Can't indirect a void pointer")
			}
			giveTypeOrVerify(opt.Out(), pointer.ToWhat)
		}
	case ir.Assign:
		_, r := resolve(opt)
		if r == nil {
			parsing.Dump(typeTable)
			panic("type should be resolved at this point")
		}
		giveTypeOrVerify(opt.Left(), r)
	case ir.ArrayToPointer:
		good := false
		switch array := typeTable[opt.In()].(type) {
		case Array:
			good = true
			typeTable[opt.Out()] = Pointer{ToWhat: array.OfWhat}
		case Pointer:
			if array, isArray := array.ToWhat.(Array); isArray {
				good = true
				typeTable[opt.Out()] = Pointer{ToWhat: array.OfWhat}
			}
		}
		if !good {
			return errors.New("must be an array")
		}
	case ir.Add:
		l, r := resolve(opt)
		lPointer, lIsPointer := l.(Pointer)
		if !(lIsPointer && r.IsNumber()) {
			if !(l.IsNumber() && r.IsNumber()) {
				fmt.Printf("%#v %#v\n", l, r)
				return errors.New("add not available for these types")
			}
		}
		if lIsPointer && isVoidPointer(lPointer) {
			panic("Pointer arithmethic not allowed on void pointers")
		}
	case ir.Sub:
		l, r := resolve(opt)
		if !(l.IsNumber() && r.IsNumber()) {
			return errors.New("operands must be numbers")
		}
	case ir.Mult:
		l, r := resolve(opt)
		if !(l.IsNumber() && r.IsNumber()) {
			return errors.New("operands must be numbers")
		}
	case ir.And, ir.Or:
		l, r := resolve(opt)
		_, lIsBool := l.(Boolean)
		_, rIsBool := r.(Boolean)
		if !lIsBool || !rIsBool {
			return errors.New("operands must be booleans")
		}
	case ir.Not:
		inT := typeTable[opt.In()]
		_, inIsPtr := inT.(Pointer)
		_, inIsBool := inT.(Boolean)
		if !inIsBool && !inIsPtr {
			return errors.New("The not operator works booleans and pointers")
		}
		giveTypeOrVerify(opt.Out(), t.Builtins[BoolIdx])
	case ir.Div:
		l, r := resolve(opt)
		if !(l.IsNumber() && r.IsNumber()) {
			parsing.Dump(l)
			parsing.Dump(r)
			return errors.New("operands must be numbers")
		}
		// TODO: issue warning for addition between float and ints
		// and different signedness
	}
	return nil
}

func (t *Typer) InferAndCheck(env *EnvRecord, toCheck *frontend.OptBlock, procDecl ProcRecord) ([]TypeRecord, error) {
	typeTable := make([]TypeRecord, toCheck.NumberOfVars)
	for i, arg := range procDecl.Args {
		typeTable[i] = arg
	}

	for i, opt := range toCheck.Opts {
		_ = i

		err := t.checkAndInferOpt(env, opt, typeTable)
		if err != nil {
			return nil, err
		}
	}

	return typeTable, nil
}

func (t *Typer) IsUnsigned(record TypeRecord) bool {
	switch record {
	case t.Builtins[U8Idx], t.Builtins[U32Idx], t.Builtins[U16Idx], t.Builtins[U64Idx]:
		return true
	}
	return false
}

func (t *Typer) typeImmediate(val interface{}) TypeRecord {
	switch val := val.(type) {
	case int64, uint64, int:
		return t.Builtins[IntIdx]
	case string:
		return t.Builtins[StringIdx]
	case bool:
		return t.Builtins[BoolIdx]
	case parsing.LiteralType:
		if val == parsing.NilPtr {
			return t.Builtins[VoidPtrIdx]
		}
	case parsing.TypeDecl:
		return t.TypeRecordFromDecl(val)
	}
	panic("this must work")
	return nil
}

var builtinTypes map[parsing.IdName]int = map[parsing.IdName]int{
	"void":   VoidIdx,
	"string": StringIdx,
	"int":    IntIdx,
	"bool":   BoolIdx,
	"u8":     U8Idx,
	"s8":     S8Idx,
	"u16":    U16Idx,
	"s16":    S16Idx,
	"u32":    U32Idx,
	"s32":    S32Idx,
	"u64":    U64Idx,
	"s64":    S64Idx,
}

func (t *Typer) mapToBuiltinType(name parsing.IdName) TypeRecord {
	idx, ok := builtinTypes[name]
	if ok {
		return t.Builtins[idx]
	} else {
		return nil
	}
}

func pointerArrayConsistent(a Pointer, b Pointer) bool {
	aArray, aPointsToArray := a.ToWhat.(Array)
	return aPointsToArray && aArray.OfWhat == b.ToWhat
}

func (t *Typer) TypesCompatible(a TypeRecord, b TypeRecord) bool {
	if reflect.DeepEqual(a, b) || (a.IsNumber() && b.IsNumber()) {
		return true
	}
	aPointer, aIsPointer := a.(Pointer)
	bPointer, bIsPointer := b.(Pointer)
	if aIsPointer && bIsPointer {
		if isVoidPointer(aPointer) || isVoidPointer(bPointer) {
			return true
		} else if pointerArrayConsistent(aPointer, bPointer) || pointerArrayConsistent(bPointer, aPointer) {
			return true
		}
	}
	return false
}

func isVoidPointer(pointer Pointer) bool {
	_, ok := pointer.ToWhat.(Void)
	return ok
}

func BuildArray(contained TypeRecord, nesting []int) TypeRecord {
	if len(nesting) == 0 {
		return contained
	}
	return Array{Nesting: nesting, OfWhat: contained}
}

func BuildRecordWithIndirection(base TypeRecord, level int) TypeRecord {
	if level == 0 {
		return base
	}
	current := Pointer{ToWhat: base}
	for i := 1; i < level; i++ {
		current = Pointer{ToWhat: current}
	}
	return current
}

func BuildRecordAccordingToUnresolved(base TypeRecord, unresolved Unresolved) TypeRecord {
	var record TypeRecord
	if unresolved.Decl.Base == "" {
		record = BuildRecordWithIndirection(base, unresolved.Decl.ArrayBase.LevelOfIndirection)
		record = BuildArray(record, unresolved.Decl.ArraySizes)
	} else {
		record = base
	}
	return BuildRecordWithIndirection(record, unresolved.Decl.LevelOfIndirection)
}

func GrabUnresolvedName(unresolved Unresolved) parsing.IdName {
	name := unresolved.Decl.Base
	if name == "" {
		name = unresolved.Decl.ArrayBase.Base
		if name == "" {
			panic("ice: nested array decl not parsed into proper format")
		}
	}
	return name
}

func (t *Typer) TypeRecordFromDecl(decl parsing.TypeDecl) TypeRecord {
	var base TypeRecord
	if decl.ArrayBase != nil {
		base = t.TypeRecordFromDecl(*decl.ArrayBase)
		_, baseIsUnresolved := base.(Unresolved)
		if baseIsUnresolved {
			return Unresolved{Decl: decl}
		}
	} else {
		base = t.mapToBuiltinType(decl.Base)
	}
	if base == nil {
		return Unresolved{Decl: decl}
	}
	base = BuildArray(base, decl.ArraySizes)
	return BuildRecordWithIndirection(base, decl.LevelOfIndirection)
}

const (
	VoidIdx int = iota
	VoidPtrIdx
	StringIdx
	IntIdx
	BoolIdx
	U8Idx
	S8Idx
	U16Idx
	S16Idx
	U32Idx
	S32Idx
	U64Idx
	S64Idx
)

func NewTyper() *Typer {
	var typer Typer
	typer.Builtins = []TypeRecord{
		Void{},
		nil, // void pointer. Filled in later.
		String{},
		Int{},
		Boolean{},
		U8{},
		S8{},
		U16{},
		S16{},
		U32{},
		S32{},
		U64{},
		S64{},
	}
	typer.Builtins[1] = BuildRecordWithIndirection(typer.Builtins[0], 1)
	return &typer
}

func NewEnvRecord(typer *Typer) *EnvRecord {
	boolType := &typer.Builtins[BoolIdx]
	voidType := &typer.Builtins[VoidIdx]
	binTableReturn := BuildRecordWithIndirection(typer.Builtins[IntIdx], 1)
	u8Ptr := BuildRecordWithIndirection(typer.Builtins[U8Idx], 1)
	env := EnvRecord{
		Types: make(map[parsing.IdName]TypeRecord),
		Procs: map[parsing.IdName]ProcRecord{
			"exit":      {Return: voidType, Args: []TypeRecord{typer.Builtins[IntIdx]}},
			"puts":      {Return: voidType, Args: []TypeRecord{typer.Builtins[StringIdx]}},
			"writes":    {Return: voidType, Args: []TypeRecord{u8Ptr, typer.Builtins[IntIdx]}},
			"print_int": {Return: voidType, Args: []TypeRecord{typer.Builtins[IntIdx]}},
			"testbit":   {Return: boolType, Args: []TypeRecord{typer.Builtins[U64Idx], typer.Builtins[IntIdx]}},
			"binToDecTable": {
				Return: &binTableReturn,
			},
		},
	}
	for name := range builtinTypes {
		env.Types[name] = typer.mapToBuiltinType(name)
	}
	return &env
}
