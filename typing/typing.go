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
	Register
)

type ProcRecord struct {
	Return            TypeRecord
	Args              []TypeRecord
	CallingConvention int
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
		l := typeTable[opt.Oprand1]
		r := typeTable[opt.Oprand2]
		return l, r
	}
	switch opt.Type {
	case ir.AssignImm:
		typeTable[opt.Oprand1] = t.typeImmediate(opt.Extra)
	case ir.TakeAddress:
		varType := typeTable[opt.In()]
		if varType == nil {
			panic("type should be resolved at this point")
		}
		typeTable[opt.Out()] = Pointer{ToWhat: varType}
	case ir.StructMemberPtr:
		baseType := typeTable[opt.In()]
		basePointer, baseIsPointer := baseType.(Pointer)
		if baseIsPointer {
			baseType = basePointer.ToWhat
		}
		baseStruct, baseIsStruct := baseType.(*StructRecord)
		if !baseIsStruct {
			panic("oprand is not a struct or pointer to a struct")
		}
		field, ok := baseStruct.Members[opt.Extra.(string)]
		if !ok {
			panic("--- is not a member of the struct")
		}
		typeTable[opt.Out()] = Pointer{ToWhat: field.Type}
	case ir.LoadStructMember:
		baseType := typeTable[opt.In()]
		basePointer, baseIsPointer := baseType.(Pointer)
		if baseIsPointer {
			baseType = basePointer.ToWhat
		}
		baseStruct, baseIsStruct := baseType.(*StructRecord)
		if !baseIsStruct {
			panic("oprand is not a struct or pointer to a struct")
		}
		field, ok := baseStruct.Members[opt.Extra.(string)]
		if !ok {
			panic("--- is not a member of the struct")
		}
		typeTable[opt.Out()] = field.Type
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
			typeTable[opt.Oprand1] = procRecord.Return
			return nil
		}
		// TODO Temporary hack for making a struct
		structRecord := typeRecord.(*StructRecord)
		typeTable[opt.Oprand1] = structRecord
	case ir.Compare:
		l := typeTable[opt.Left()]
		r := typeTable[opt.Right()]
		extra := opt.Extra.(ir.CompareExtra)
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
		l, r := resolve(opt)
		if r == nil {
			panic("type should be resolved at this point")
		}
		if l != r {
			if l == nil {
				typeTable[opt.Left()] = r
			} else {
				return errors.New("incompatible types")
			}
		}
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
	case ir.BoolAnd, ir.BoolOr:
		l, r := resolve(opt)
		_, lIsBool := l.(Boolean)
		_, rIsBool := r.(Boolean)
		if !lIsBool || !rIsBool {
			return errors.New("operands must be booleans")
		}
	case ir.Not:
		l, r := resolve(opt)
		_, lIsBool := l.(Boolean)
		_, lIsPtr := l.(Pointer)
		_, rIsBool := r.(Boolean)
		if !lIsBool && !lIsPtr {
			return errors.New("The not operator works booleans and pointers")
		}
		if r == nil {
			typeTable[opt.Out()] = t.Builtins[BoolIdx]
		}
		if r != nil && !rIsBool {
			panic("out var of ir.Not has a type and it's not boolean")
		}
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
		return t.ConstructTypeRecord(val)
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
	}
	return nil
}

func BuildArray(contained TypeRecord, nesting []int) TypeRecord {
	if len(nesting) == 0 {
		return contained
	}
	return Array{Nesting: nesting, OfWhat: contained}
}

func BuildPointer(base TypeRecord, level int) TypeRecord {
	if level == 0 {
		return base
	}
	current := Pointer{ToWhat: base}
	for i := 1; i < level; i++ {
		current = Pointer{ToWhat: current}
	}
	return current
}

func (t *Typer) ConstructTypeRecord(decl parsing.TypeDecl) TypeRecord {
	var base TypeRecord
	if decl.ArrayBase != nil {
		base = t.ConstructTypeRecord(*decl.ArrayBase)
	} else {
		base = t.mapToBuiltinType(decl.Base)
	}
	if base == nil {
		return Unresolved{Decl: decl}
	}
	base = BuildArray(base, decl.ArraySizes)
	return BuildPointer(base, decl.LevelOfIndirection)
}

const (
	VoidIdx int = iota
	StringIdx
	IntIdx
	BoolIdx
	U8Idx
)

func NewTyper() *Typer {
	var typer Typer
	typer.Builtins = []TypeRecord{
		Void{},
		String{},
		Int{},
		Boolean{},
		U8{},
	}
	return &typer
}

func NewEnvRecord(typer *Typer) *EnvRecord {
	boolType := typer.Builtins[BoolIdx]
	return &EnvRecord{
		Types: make(map[parsing.IdName]TypeRecord),
		Procs: map[parsing.IdName]ProcRecord{
			"exit":      {Return: boolType, CallingConvention: Register},
			"puts":      {Return: boolType, CallingConvention: Register},
			"print_int": {Return: boolType, CallingConvention: Register},
			"testbit":   {Return: boolType, CallingConvention: Register},
			"binToDecTable": {
				Return:            BuildPointer(typer.Builtins[IntIdx], 1),
				CallingConvention: Register,
			},
		},
	}
}
