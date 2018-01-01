package typing

import (
	"errors"
	"github.com/XrXr/alang/frontend"
	"github.com/XrXr/alang/ir"
	"github.com/XrXr/alang/parsing"
)

type FuncType struct {
	Return TypeRecord
	Args   []TypeRecord
}

type VarType struct {
	Base       TypeRecord
	Parameters []TypeRecord
}

type FuncRecord map[parsing.IdName]FuncType
type VarRecord map[parsing.IdName]VarType

type EnvRecord struct {
	Funcs FuncRecord
	Vars  VarRecord
	Types map[parsing.IdName]TypeRecord
}

func (e *EnvRecord) contains(decl parsing.IdName) bool {
	_, found := e.Vars[decl]
	if found {
		return true
	}
	_, found = e.Funcs[decl]
	return found
}

type Typer struct {
	builtins []TypeRecord
}

func (t *Typer) checkAndInferOpt(env *EnvRecord, opt interface{}, typeTable []TypeRecord) error {
	resolve := func(opt ir.BinaryVarOpt) (TypeRecord, TypeRecord) {
		l := typeTable[opt.Left]
		r := typeTable[opt.Right]
		return l, r
	}
	switch opt := opt.(type) {
	case ir.AssignImm:
		typeTable[opt.Var] = t.typeImmediate(opt.Val)
	case ir.TakeAddress:
		varType := typeTable[opt.Var]
		if varType == nil {
			panic("type should be resolved at this point")
		}
		typeTable[opt.Out] = Pointer{ToWhat: varType}
	case ir.StructMemberPtr:
		baseType := typeTable[opt.Base]
		basePointer, baseIsPointer := baseType.(Pointer)
		if baseIsPointer {
			baseType = basePointer.ToWhat
		}
		baseStruct, baseIsStruct := baseType.(StructRecord)
		if !baseIsStruct {
			panic("oprand is not a struct or pointer to a struct")
		}
		field, ok := baseStruct.Members[opt.Member]
		if !ok {
			panic("--- is not a member of the struct")
		}
		typeTable[opt.Out] = Pointer{ToWhat: field.Type}
	case ir.LoadStructMember:
		baseType := typeTable[opt.Base]
		basePointer, baseIsPointer := baseType.(Pointer)
		if baseIsPointer {
			baseType = basePointer.ToWhat
		}
		baseStruct, baseIsStruct := baseType.(StructRecord)
		if !baseIsStruct {
			panic("oprand is not a struct or pointer to a struct")
		}
		field, ok := baseStruct.Members[opt.Member]
		if !ok {
			panic("--- is not a member of the struct")
		}
		typeTable[opt.Out] = field.Type
	case ir.Call:
		typeRecord, ok := env.Types[parsing.IdName(opt.Label)]
		if !ok {
			// TODO
			typeTable[opt.Out] = Unresolved{}
			return nil
		}
		// Temporary
		structRecord := typeRecord.(StructRecord)
		typeTable[opt.Out] = structRecord
	case ir.IndirectWrite:
		varType := typeTable[opt.Pointer]
		if varType == nil {
			panic("type should be resolved at this point")
		}
		typeForData := typeTable[opt.Data]
		if varType == nil {
			panic("type should be resolved at this point")
		}
		pointer, varIsPointer := varType.(Pointer)
		if !varIsPointer {
			panic("That's not a pointer what are you doing")
		}
		if pointer.ToWhat != typeForData {
			panic("Type mismatch")
		}
	case ir.IndirectLoad:
		ptrType := typeTable[opt.Pointer]
		if ptrType == nil {
			panic("type should be resolved at this point")
		}
		pointer, isPointer := ptrType.(Pointer)
		if !isPointer {
			panic("Can't indirect a non pointer")
		}
		typeForOut := typeTable[opt.Out]
		if typeForOut == nil {
			typeTable[opt.Out] = pointer.ToWhat
			typeForOut = pointer.ToWhat
		}
		if pointer.ToWhat != typeForOut {
			panic("Type mismatch")
		}
	case ir.Assign:
		l, r := resolve(ir.BinaryVarOpt(opt))
		if r == nil {
			panic("type should be resolved at this point")
		}
		if l != r {
			if l == nil {
				typeTable[opt.Left] = r
			} else {
				return errors.New("incompatible types")
			}
		}
	case ir.Add:
		l, r := resolve(ir.BinaryVarOpt(opt))
		if !(l.IsNumber() && r.IsNumber()) {
			return errors.New("operands must be numbers")
		}
	case ir.Sub:
		l, r := resolve(ir.BinaryVarOpt(opt))
		if !(l.IsNumber() && r.IsNumber()) {
			return errors.New("operands must be numbers")
		}
	case ir.Mult:
		l, r := resolve(ir.BinaryVarOpt(opt))
		if !(l.IsNumber() && r.IsNumber()) {
			return errors.New("operands must be numbers")
		}
	case ir.Div:
		l, r := resolve(ir.BinaryVarOpt(opt))
		if !(l.IsNumber() && r.IsNumber()) {
			return errors.New("operands must be numbers")
		}
		// TODO: issue warning for addition between float and ints
		// and different signedness
	}
	return nil
}

func (t *Typer) InferAndCheck(env *EnvRecord, toCheck *frontend.OptBlock) ([]TypeRecord, error) {
	typeTable := make([]TypeRecord, toCheck.NumberOfVars)
	for _, opt := range toCheck.Opts {
		err := t.checkAndInferOpt(env, opt, typeTable)
		if err != nil {
			return nil, err
		}
	}

	return typeTable, nil
}

func (t *Typer) typeImmediate(val interface{}) TypeRecord {
	switch val.(type) {
	case int:
		return t.builtins[intIdx]
	case string:
		return t.builtins[stringIdx]
	}
	panic("this must work")
	return nil
}

func (t *Typer) ResolveBuiltinType(name string) TypeRecord {
	switch name {
	case "void":
		return t.builtins[voidIdx]
	case "string":
		return t.builtins[stringIdx]
	case "int":
		return t.builtins[intIdx]
	}
	return nil
}

const (
	voidIdx int = iota
	stringIdx
	intIdx
)

func NewTyper() *Typer {
	var typer Typer
	typer.builtins = []TypeRecord{
		Void{},
		String{},
		Int{},
	}
	return &typer
}

func NewEnvRecord() *EnvRecord {
	return &EnvRecord{
		Types: make(map[parsing.IdName]TypeRecord),
	}
}
