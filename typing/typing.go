package typing

import (
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
	Procs map[string]ProcRecord
	Types map[string]TypeRecord
}

type Typer struct {
	Builtins []TypeRecord
}

func (t *Typer) checkAndInferOpt(env *EnvRecord, opt ir.Inst, typeTable []TypeRecord) error {
	bail := func(message string) {
		panic(parsing.ErrorFromNode(opt.GeneratedFrom, message))
	}
	bailLeft := func(message string) {
		expr := opt.GeneratedFrom.(parsing.ExprNode)
		panic(parsing.ErrorFromNode(expr.Left, message))
	}
	bailRight := func(message string) {
		expr := opt.GeneratedFrom.(parsing.ExprNode)
		panic(parsing.ErrorFromNode(expr.Right, message))
	}
	resolve := func(opt ir.Inst) (TypeRecord, TypeRecord) {
		l := typeTable[opt.Left()]
		r := typeTable[opt.Right()]
		return l, r
	}
	mustHaveType := func(vn int) TypeRecord {
		record := typeTable[vn]
		if record == nil {
			panic("ice: Type should be resolved at this point. Faulty Ir?")
		}
		return record
	}
	mustBeAssignable := func(target, value TypeRecord) {
		if !t.Assignable(target, value) {
			bail(fmt.Sprintf("Type mismatch: writing to a location with type %s using a value of type %s", target.Rep(), value.Rep()))
		}
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
			bail("Struct member access on non struct")
		}
		if baseIsStruct {
			field, ok := baseStruct.Members[fieldName]
			if !ok {
				bailRight(fmt.Sprintf("Not a member of struct %s", baseStruct.Name))
			}
			return field.Type
		} else if baseIsString {
			if fieldName != "data" && fieldName != "length" {
				bailRight("Strings don't have this field")
			}
			switch fieldName {
			case "length":
				return t.Builtins[IntIdx]
			case "data":
				return Pointer{ToWhat: t.Builtins[U8Idx]}
			}
		}
		panic("ice: encountered a struct member that we don't know how to find the type of")
		return nil
	}
	giveTypeOrVerify := func(target int, typeRecord TypeRecord) {
		currentType := typeTable[target]
		if currentType == nil {
			typeTable[target] = typeRecord
		} else {
			mustBeAssignable(currentType, typeRecord)
		}
	}
	switch opt.Type {
	case ir.AssignImm:
		finalType := t.typeImmediate(opt.Extra)
		if unresolved, isUnresolved := finalType.(Unresolved); isUnresolved {
			name := GrabUnresolvedName(unresolved)
			structRecord, ok := env.Types[name]
			if !ok {
				bail(fmt.Sprintf(`"%s" does not name a type`, name))
			}
			finalType = BuildRecordAccordingToUnresolved(structRecord, unresolved)
		}
		giveTypeOrVerify(opt.Out(), finalType)
	case ir.TakeAddress:
		varType := mustHaveType(opt.In())
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
		callee := extra.Name
		typeRecord, callToType := env.Types[callee]
		if callToType {
			switch record := typeRecord.(type) {
			case *StructRecord:
				// making a struct
				giveTypeOrVerify(out, record)
			default:
				// type casting
				if len(extra.ArgVars) != 1 {
					bail("Type casting only operates on one operand")
				}
				if typeRecord.Size() != typeTable[extra.ArgVars[0]].Size() {
					bail("Invalid cast: size of the types must match")
				}
				giveTypeOrVerify(out, typeRecord)
			}
		} else {
			procRecord, ok := env.Procs[callee]
			if !ok {
				bail("Call to undefined procedure " + extra.Name)
			}
			failed := false
			var message string
			if len(extra.ArgVars) != len(procRecord.Args) {
				failed = true
				message = "Wrong number of arguments"
			}
			for _, vn := range extra.ArgVars {
				mustHaveType(vn)
			}
			if !failed {
				for i, vn := range extra.ArgVars {
					if !t.Assignable(typeTable[vn], procRecord.Args[i]) {
						failed = true
						message = "Argument type mismatch"
					}
				}
			}
			if failed {
				passed := make([]TypeRecord, 0, len(extra.ArgVars))
				for _, vn := range extra.ArgVars {
					passed = append(passed, typeTable[vn])
				}
				widths := make([]int, 0, len(extra.ArgVars))
				for i := 0; i < len(procRecord.Args) && i < len(passed); i++ {
					max := len(passed[i].Rep())
					if other := len(procRecord.Args[i].Rep()); other > max {
						max = other
					}
					widths = append(widths, max)
				}
				message += "\n    want "
				message += RepForListOfTypes(procRecord.Args, widths)
				message += "\n    have "
				message += RepForListOfTypes(passed, widths)
				bail(message)
			}

			giveTypeOrVerify(out, *procRecord.Return)
			return nil
		}
	case ir.Compare:
		extra := opt.Extra.(ir.CompareExtra)
		l := mustHaveType(opt.In())
		r := mustHaveType(extra.Right)
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
				bail(fmt.Sprintf("Can't copmare %s with %s", l.Rep(), r.Rep()))
			}
		}
		typeTable[extra.Out] = t.Builtins[BoolIdx]
	case ir.IndirectWrite:
		// This ir is special in that it puts a variable that it doesn't mutate in MutateOperand.
		// If we start doing more sophisticated analysis we might want to change that.
		varType := mustHaveType(opt.Left())
		typeForData := mustHaveType(opt.Right())
		switch record := varType.(type) {
		case Pointer:
			if isVoidPointer(record) {
				bailLeft("Writing to a void pointer")
			}
			mustBeAssignable(record.ToWhat, typeForData)
		case StringDataPointer:
			bailLeft("Writing to a read-only field")
		default:
			bailLeft("Not a valid write target")
		}
	case ir.IndirectLoad:
		ptrType := mustHaveType(opt.In())
		switch record := ptrType.(type) {
		case StringDataPointer:
			giveTypeOrVerify(opt.Out(), Pointer{ToWhat: t.Builtins[U8Idx]})
		case Pointer:
			if isVoidPointer(record) {
				bail("Indirecting a void pointer")
			}
			giveTypeOrVerify(opt.Out(), record.ToWhat)
		default:
			bail("Indirecting a non pointer")
		}
	case ir.Assign:
		giveTypeOrVerify(opt.Left(), mustHaveType(opt.ReadOperand))
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
			bail("Array access on non array")
		}
	case ir.Add:
		l, r := resolve(opt)
		lPointer, lIsPointer := l.(Pointer)
		if !(lIsPointer && r.IsNumber()) {
			if !(l.IsNumber() && r.IsNumber()) {
				bail(fmt.Sprintf("Can't add to %s with %s", l.Rep(), r.Rep()))
			}
		}
		if lIsPointer && isVoidPointer(lPointer) {
			bail("Pointer arithmethic on void pointer")
		}
	case ir.Sub, ir.Mult, ir.Div:
		l, r := resolve(opt)
		if !(l.IsNumber() && r.IsNumber()) {
			bail("Operands must be numbers")
		}
	case ir.And, ir.Or:
		l, r := resolve(opt)
		_, lIsBool := l.(Boolean)
		_, rIsBool := r.(Boolean)
		if !lIsBool || !rIsBool {
			bail("Operands must be booleans")
		}
	case ir.Not:
		inT := typeTable[opt.In()]
		_, inIsPtr := inT.(Pointer)
		_, inIsBool := inT.(Boolean)
		if !inIsBool && !inIsPtr {
			bail("The not operator only works with booleans and pointers")
		}
		giveTypeOrVerify(opt.Out(), t.Builtins[BoolIdx])
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
		// fmt.Println("type checking ir line", i)

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
	panic("ice: Failed to find the type of a literal")
	return nil
}

var builtinTypes map[string]int = map[string]int{
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

func (t *Typer) mapToBuiltinType(name string) TypeRecord {
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

func RepForListOfTypes(types []TypeRecord, widths []int) string {
	result := "("
	length := len(types)
	for i, record := range types {
		rep := record.Rep()
		result += rep
		if i < length-1 {
			if i < len(widths) {
				width := widths[i]
				for j := 0; j < width-len(rep); j++ {
					result += " "
				}
			}
			result += ", "
		}
	}
	result += ")"
	return result
}

func (t *Typer) Assignable(target, value TypeRecord) bool {
	if reflect.DeepEqual(target, value) || (target.IsNumber() && value.IsNumber()) {
		return true
	}
	targetAsPointer, targetIsPointer := target.(Pointer)
	valueAsPointer, valueIsPointer := value.(Pointer)
	if targetIsPointer && valueIsPointer {
		if isVoidPointer(targetAsPointer) || isVoidPointer(valueAsPointer) {
			return true
		} else if pointerArrayConsistent(targetAsPointer, valueAsPointer) || pointerArrayConsistent(valueAsPointer, targetAsPointer) {
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
	if unresolved.Decl.Base.Name == "" {
		record = BuildRecordWithIndirection(base, unresolved.Decl.ArrayBase.LevelOfIndirection)
		record = BuildArray(record, unresolved.Decl.ArraySizes)
	} else {
		record = base
	}
	return BuildRecordWithIndirection(record, unresolved.Decl.LevelOfIndirection)
}

func GrabUnresolvedName(unresolved Unresolved) string {
	name := unresolved.Decl.Base.Name
	if name == "" {
		name = unresolved.Decl.ArrayBase.Base.Name
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
		base = t.mapToBuiltinType(decl.Base.Name)
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
		Types: make(map[string]TypeRecord),
		Procs: map[string]ProcRecord{
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
