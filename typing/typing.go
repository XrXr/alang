package typing

import (
	"errors"
	"fmt"
	"github.com/XrXr/alang/frontend"
	"github.com/XrXr/alang/ir"
	"github.com/XrXr/alang/parsing"
)

const (
	Cdecl int = iota + 1
	SystemV
)

type ProcRecord struct {
	Return            *TypeRecord
	Args              []TypeRecord
	CallingConvention int
	IsForeign         bool
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
				panic("--- is not a member of the struct")
			}
		} else if baseIsString {
			if fieldName != "data" && fieldName != "length" {
				panic("--- is not a member of the struct")
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
			structRecord, ok := env.Types[unresolved.Decl.Base]
			if !ok {
				panic(unresolved.Decl.Base + " does not name a type")
			}
			finalType = BuildRecordWithIndirection(structRecord, unresolved.Decl.LevelOfIndirection)
		}
		giveTypeOrVerify(opt.Out(), finalType)
	case ir.TakeAddress:
		varType := typeTable[opt.In()]
		if varType == nil {
			panic("type should be resolved at this point")
		}
		typeTable[opt.Out()] = Pointer{ToWhat: varType}
	case ir.StructMemberPtr:
		outType := checkAndFindStructMemberType(opt.In(), opt.Extra.(string))
		outType = Pointer{ToWhat: outType}
		giveTypeOrVerify(opt.Out(), outType)
	case ir.LoadStructMember:
		outType := checkAndFindStructMemberType(opt.In(), opt.Extra.(string))
		giveTypeOrVerify(opt.Out(), outType)
	case ir.Call:
		extra := opt.Extra.(ir.CallExtra)
		typeRecord, ok := env.Types[parsing.IdName(extra.Name)]
		if !ok {
			procRecord, ok := env.Procs[parsing.IdName(extra.Name)]
			if !ok {
				println(parsing.IdName(extra.Name))
				panic("Call to undefined procedure")
			}
			//TODO check arg types
			if typeTable[opt.Out()] == nil {
				typeTable[opt.Out()] = *procRecord.Return
				// TODO checking oppotunity
			}
			return nil
		}
		// TODO Temporary hack for making a struct
		structRecord := typeRecord.(*StructRecord)
		typeTable[opt.Out()] = structRecord
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
		if pointer.ToWhat != typeForData {
			if !(pointer.ToWhat == t.Builtins[U8Idx] && typeForData == t.Builtins[IntIdx]) {
				parsing.Dump(pointer.ToWhat)
				parsing.Dump(typeForData)
				panic("Type mismatch")
			}
		}
	case ir.IndirectLoad:
		ptrType := typeTable[opt.In()]
		if ptrType == nil {
			panic("type should be resolved at this point")
		}
		pointer, isPointer := ptrType.(Pointer)
		if !isPointer {
			panic("Can't indirect a non pointer")
		}
		typeForOut := typeTable[opt.Out()]
		if typeForOut == nil {
			typeTable[opt.Out()] = pointer.ToWhat
			typeForOut = pointer.ToWhat
		}
		if pointer.ToWhat != typeForOut {
			panic("Type mismatch")
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
			return errors.New(" must be an array")
		}
	case ir.Add:
		l, r := resolve(opt)
		if _, lIsPointer := l.(Pointer); !(lIsPointer && r.IsNumber()) {
			if !(l.IsNumber() && r.IsNumber()) {
				fmt.Printf("%#v %#v\n", l, r)
				return errors.New("add not available for these types")
			}
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

func (t *Typer) typeImmediate(val interface{}) TypeRecord {
	switch val := val.(type) {
	case int64, uint64, int:
		return t.Builtins[IntIdx]
	case string:
		return t.Builtins[StringIdx]
	case bool:
		return t.Builtins[BoolIdx]
	case parsing.TypeDecl:
		return t.TypeRecordFromDecl(val)
	}
	panic("this must work")
	return nil
}

func (t *Typer) mapToBuiltinType(name parsing.IdName) TypeRecord {
	switch name {
	case "void":
		return t.Builtins[VoidIdx]
	case "string":
		return t.Builtins[StringIdx]
	case "int":
		return t.Builtins[IntIdx]
	case "bool":
		return t.Builtins[BoolIdx]
	case "u8":
		return t.Builtins[U8Idx]
	case "s8":
		return t.Builtins[S8Idx]
	case "s32":
		return t.Builtins[S32Idx]
	case "u32":
		return t.Builtins[U32Idx]
	case "s64":
		return t.Builtins[S64Idx]
	case "u64":
		return t.Builtins[U64Idx]
	}
	return nil
}

func (t *Typer) TypesCompatible(a TypeRecord, b TypeRecord) bool {
	return (a == b) || (a.IsNumber() && b.IsNumber())
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

func (t *Typer) TypeRecordFromDecl(decl parsing.TypeDecl) TypeRecord {
	var base TypeRecord
	if decl.ArrayBase != nil {
		base = t.TypeRecordFromDecl(*decl.ArrayBase)
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
	StringIdx
	IntIdx
	BoolIdx
	S8Idx
	U8Idx
	S32Idx
	U32Idx
	S64Idx
	U64Idx
)

func NewTyper() *Typer {
	var typer Typer
	typer.Builtins = []TypeRecord{
		Void{},
		String{},
		Int{},
		Boolean{},
		S8{},
		U8{},
		S32{},
		U32{},
		S64{},
		U64{},
	}
	return &typer
}

func NewEnvRecord(typer *Typer) *EnvRecord {
	boolType := &typer.Builtins[BoolIdx]
	voidType := &typer.Builtins[VoidIdx]
	binTableReturn := BuildRecordWithIndirection(typer.Builtins[IntIdx], 1)
	return &EnvRecord{
		Types: make(map[parsing.IdName]TypeRecord),
		Procs: map[parsing.IdName]ProcRecord{
			"exit":      {Return: voidType, CallingConvention: SystemV},
			"puts":      {Return: voidType, CallingConvention: SystemV},
			"print_int": {Return: voidType, CallingConvention: SystemV},
			"testbit":   {Return: boolType, CallingConvention: SystemV},
			"binToDecTable": {
				Return:            &binTableReturn,
				CallingConvention: SystemV,
			},
		},
	}
}
