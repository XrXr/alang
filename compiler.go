package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"github.com/XrXr/alang/frontend"
	"github.com/XrXr/alang/ir"
	"github.com/XrXr/alang/parsing"
	"github.com/XrXr/alang/typing"
	"io"
	"log"
	"os"
	"os/exec"
)

const (
	Proc = iota + 1
	If
	Else
)

func backendDebug(framesize int, typeTable []typing.TypeRecord, offsetTable []int) {
	fmt.Println("framesize", framesize)
	if len(typeTable) != len(offsetTable) {
		panic("what?")
	}
	for i, typeRecord := range typeTable {
		fmt.Printf("var %d type: %#v offset %d\n", i, typeRecord, offsetTable[i])
	}
}

func backendForOptBlock(out io.Writer, staticDataBuf *bytes.Buffer, labelGen *frontend.LabelIdGen, block frontend.OptBlock, typeTable []typing.TypeRecord, env *typing.EnvRecord, typer *typing.Typer) {
	addLine := func(line string) {
		io.WriteString(out, line)
	}
	varOffset := make([]int, block.NumberOfVars)

	if block.NumberOfArgs > 0 {
		// we push rbp in the prologue and call pushes the return address
		varOffset[0] = -16
		for i := 1; i < block.NumberOfArgs; i++ {
			varOffset[i] = varOffset[i-1] - typeTable[i-1].Size()
		}
	}

	firstLocal := block.NumberOfArgs
	if firstLocal < block.NumberOfVars {
		varOffset[firstLocal] = typeTable[firstLocal].Size()
		for i := firstLocal + 1; i < block.NumberOfVars; i++ {
			varOffset[i] = varOffset[i-1] + typeTable[i].Size()
		}
	}
	// Take note that not everything uses these. Namely indirect read/write
	qwordVarToStack := func(varNum int) string {
		return fmt.Sprintf("qword [rbp-%d]", varOffset[varNum])
	}
	wordVarToStack := func(varNum int) string {
		return fmt.Sprintf("dword [rbp-%d]", varOffset[varNum])
	}
	byteVarToStack := func(varNum int) string {
		return fmt.Sprintf("byte [rbp-%d]", varOffset[varNum])
	}
	simpleCopy := func(sourceVarNum int, dest string) {
		soruceType := typeTable[sourceVarNum]
		switch soruceType.Size() {
		case 1:
			addLine(fmt.Sprintf("\tmov al, %s\n", byteVarToStack(sourceVarNum)))
			addLine(fmt.Sprintf("\tmov %s, al\n", dest))
		case 4:
			addLine(fmt.Sprintf("\tmov eax, %s\n", wordVarToStack(sourceVarNum)))
			addLine(fmt.Sprintf("\tmov %s, eax\n", dest))
		case 8:
			addLine(fmt.Sprintf("\tmov rax, %s\n", qwordVarToStack(sourceVarNum)))
			addLine(fmt.Sprintf("\tmov %s, rax\n", dest))
		default:
			// TODO not panicing right now because we assign structs to unused vars in ir
			// panic("need a complex copy")
		}
	}

	framesize := 0
	for _, typeRecord := range typeTable {
		framesize += typeRecord.Size()
	}
	// backendDebug(framesize, typeTable, varOffset)
	for i, opt := range block.Opts {
		fmt.Fprintf(out, ";ir line %d\n", i)
		switch opt.Type {
		case ir.Assign:
			var dest string
			switch typeTable[opt.Left()].Size() {
			case 1:
				dest = byteVarToStack(opt.Left())
			case 4:
				dest = wordVarToStack(opt.Left())
			case 8:
				dest = qwordVarToStack(opt.Left())
			}
			simpleCopy(opt.Right(), dest)
		case ir.AssignImm:
			switch value := opt.Extra.(type) {
			case int64, uint64, int:
				addLine(fmt.Sprintf("\tmov %s, %d\n", qwordVarToStack(opt.Oprand1), opt.Extra))
			case bool:
				val := 0
				if value == true {
					val = 1
				}
				addLine(fmt.Sprintf("\tmov %s, %d\n", byteVarToStack(opt.Oprand1), val))
			case string:
				var buf bytes.Buffer
				buf.WriteString("\tdb\t")
				byteCount := 0
				i := 0
				needToStartQuote := true
				for ; i < len(value); i++ {
					if needToStartQuote {
						buf.WriteRune('"')
						needToStartQuote = false
					}
					if value[i] == '\\' && value[i+1] == 'n' {
						buf.WriteString(`",10,`)
						needToStartQuote = true
						i++
					} else {
						buf.WriteString(string(value[i]))
					}
					byteCount++
				}
				// end the string
				if !needToStartQuote {
					buf.WriteRune('"')
				}

				labelName := labelGen.GenLabel("label%d")
				staticDataBuf.WriteString(fmt.Sprintf("%s:\n", labelName))
				staticDataBuf.WriteString(fmt.Sprintf("\tdq\t%d\n", byteCount))
				staticDataBuf.ReadFrom(&buf)
				staticDataBuf.WriteRune('\n')
				addLine(fmt.Sprintf("\tmov %s, %s\n", qwordVarToStack(opt.Oprand1), labelName))
			case parsing.TypeDecl:
				// TODO zero out decl
			default:
				panic("unknown immediate value type")
			}
		case ir.Add:
			addLine(fmt.Sprintf("\tmov rax, %s\n", qwordVarToStack(opt.Right())))
			if pointer, leftIsPointer := typeTable[opt.Left()].(typing.Pointer); leftIsPointer {
				addLine(fmt.Sprintf("\timul rax, %d\n", pointer.ToWhat.Size()))
			}
			addLine(fmt.Sprintf("\tadd %s, rax\n", qwordVarToStack(opt.Left())))
		case ir.Increment:
			addLine(fmt.Sprintf("\tinc %s\n", qwordVarToStack(opt.In())))
		case ir.Decrement:
			addLine(fmt.Sprintf("\tdec %s\n", qwordVarToStack(opt.In())))
		case ir.Sub:
			addLine(fmt.Sprintf("\tmov rax, %s\n", qwordVarToStack(opt.Right())))
			addLine(fmt.Sprintf("\tsub %s, rax\n", qwordVarToStack(opt.Left())))
		case ir.Mult:
			addLine(fmt.Sprintf("\tmov r8, %s\n", qwordVarToStack(opt.Left())))
			addLine(fmt.Sprintf("\tmov r9, %s\n", qwordVarToStack(opt.Right())))
			addLine("\timul r8, r9\n")
			addLine(fmt.Sprintf("\tmov %s, r8\n", qwordVarToStack(opt.Left())))
		case ir.Div:
			addLine("\txor rdx, rdx\n")
			addLine(fmt.Sprintf("\tmov rax, %s\n", qwordVarToStack(opt.Left())))
			addLine(fmt.Sprintf("\tmov r8, %s\n", qwordVarToStack(opt.Right())))
			addLine("\tidiv r8\n")
			addLine(fmt.Sprintf("\tmov %s, rax\n", qwordVarToStack(opt.Left())))
		case ir.JumpIfFalse:
			addLine(fmt.Sprintf("\tmov al, %s\n", byteVarToStack(opt.In())))
			addLine("\tcmp al, 0\n")
			addLine(fmt.Sprintf("\tjz %s\n", opt.Extra.(string)))
		case ir.JumpIfTrue:
			addLine(fmt.Sprintf("\tmov al, %s\n", byteVarToStack(opt.In())))
			addLine("\tcmp al, 0\n")
			addLine(fmt.Sprintf("\tjnz %s\n", opt.Extra.(string)))
		case ir.Call:
			extra := opt.Extra.(ir.CallExtra)
			if _, isStruct := env.Types[parsing.IdName(extra.Name)]; isStruct {
				// TODO: code to zero the members
			} else {
				// TODO: this can be done once
				totalArgSize := 0
				for _, arg := range extra.ArgVars {
					totalArgSize += typeTable[arg].Size()
				}

				procRecord := env.Procs[parsing.IdName(extra.Name)]
				switch procRecord.CallingConvention {
				case typing.Cdecl:
					addLine(fmt.Sprintf("\tsub rsp, %d\n", totalArgSize))
					offset := 0

					for _, arg := range extra.ArgVars {
						thisArgSize := typeTable[arg].Size()
						var dest string
						switch thisArgSize {
						case 1:
							dest = fmt.Sprintf("byte [rsp+%d]", offset)
						case 4:
							dest = fmt.Sprintf("word [rsp+%d]", offset)
						case 8:
							dest = fmt.Sprintf("qword [rsp+%d]", offset)
						}
						simpleCopy(arg, dest)
						offset += thisArgSize
					}
				case typing.Register:
					regOrder := [...]string{"rax", "rbx", "rcx"}
					for i, arg := range extra.ArgVars {
						addLine(fmt.Sprintf("\tmov %s, %s\n", regOrder[i], qwordVarToStack(arg)))
					}
				}
				addLine(fmt.Sprintf("\tcall proc_%s\n", extra.Name))
				switch procRecord.CallingConvention {
				case typing.Register:
					switch typeTable[opt.Oprand1].Size() {
					case 1:
						addLine(fmt.Sprintf("\tmov %s, al\n", byteVarToStack(opt.Oprand1)))
					case 4:
						addLine(fmt.Sprintf("\tmov %s, eax\n", wordVarToStack(opt.Oprand1)))
					case 8:
						addLine(fmt.Sprintf("\tmov %s, rax\n", qwordVarToStack(opt.Oprand1)))
					}
				case typing.Cdecl:
					addLine(fmt.Sprintf("\tadd rsp, %d\n", totalArgSize))
					if typeTable[opt.Oprand1].Size() > 0 {
						returnType := procRecord.Return
						addLine(fmt.Sprintf("\tmov rbx, %d\n", returnType.Size()))
						addLine("\tmov rcx, rbp\n")
						addLine(fmt.Sprintf("\tsub rcx, %d\n", varOffset[opt.Oprand1]))
						addLine("\tcall _intrinsic_memcpy\n")
					}
				}
			}
		case ir.Jump:
			addLine(fmt.Sprintf("\tjmp %s\n", opt.Extra.(string)))
		case ir.Label:
			addLine(fmt.Sprintf("%s:\n", opt.Extra.(string)))
		case ir.StartProc:
			addLine(fmt.Sprintf("proc_%s:\n", opt.Extra.(string)))
			addLine("\tpush rbp\n")
			addLine("\tmov rbp, rsp\n")
			addLine(fmt.Sprintf("\tsub rsp, %d\n", framesize))
		case ir.EndProc:
			addLine("\tmov rsp, rbp\n")
			addLine("\tpop rbp\n")
			addLine("\tret\n")
		case ir.Compare:
			extra := opt.Extra.(ir.CompareExtra)
			lt := typeTable[opt.Left()]
			rt := typeTable[opt.Right()]
			smaller := lt
			if rt.Size() < lt.Size() {
				smaller = rt
			}
			if ls := lt.Size(); !(ls == 8 || ls == 4 || ls == 1) {
				// array & struct compare
				panic("Not yet")
			}

			var lReg string
			var rReg string
			switch smaller.Size() {
			case 1:
				lReg = "al"
				rReg = "bl"
				addLine(fmt.Sprintf("\tmov %s, %s\n", lReg, byteVarToStack(opt.Left())))
				addLine(fmt.Sprintf("\tmov %s, %s\n", rReg, byteVarToStack(opt.Right())))
			case 4:
				lReg = "eax"
				rReg = "ebx"
				addLine(fmt.Sprintf("\tmov %s, %s\n", lReg, wordVarToStack(opt.Left())))
				addLine(fmt.Sprintf("\tmov %s, %s\n", rReg, wordVarToStack(opt.Right())))
			case 8:
				lReg = "rax"
				rReg = "rbx"
				addLine(fmt.Sprintf("\tmov %s, %s\n", lReg, qwordVarToStack(opt.Left())))
				addLine(fmt.Sprintf("\tmov %s, %s\n", rReg, qwordVarToStack(opt.Right())))
			}
			addLine(fmt.Sprintf("\tmov %s, 1\n", byteVarToStack(extra.Out)))
			addLine(fmt.Sprintf("\tcmp %s, %s\n", lReg, rReg))
			labelName := labelGen.GenLabel("cmp%d")
			switch extra.How {
			case ir.Greater:
				addLine(fmt.Sprintf("\tjg %s\n", labelName))
			case ir.Lesser:
				addLine(fmt.Sprintf("\tjl %s\n", labelName))
			case ir.GreaterOrEqual:
				addLine(fmt.Sprintf("\tjge %s\n", labelName))
			case ir.LesserOrEqual:
				addLine(fmt.Sprintf("\tjle %s\n", labelName))
			case ir.AreEqual:
				addLine(fmt.Sprintf("\tje %s\n", labelName))
			case ir.NotEqual:
				addLine(fmt.Sprintf("\tjne %s\n", labelName))
			}
			addLine(fmt.Sprintf("\tmov %s, 0\n", byteVarToStack(extra.Out)))
			addLine(fmt.Sprintf("%s:\n", labelName))
		case ir.Transclude:
			panic("Transcludes should be gone by now")
		case ir.TakeAddress:
			dest := qwordVarToStack(opt.Out())
			addLine(fmt.Sprintf("\tmov %s, rbp\n", dest))
			addLine(fmt.Sprintf("\tsub %s, %d\n", dest, varOffset[opt.In()]))
		case ir.ArrayToPointer:
			dest := qwordVarToStack(opt.Out())
			switch typeTable[opt.In()].(type) {
			case typing.Array:
				addLine(fmt.Sprintf("\tmov %s, rbp\n", dest))
				addLine(fmt.Sprintf("\tsub %s, %d\n", dest, varOffset[opt.In()]))
			case typing.Pointer:
				simpleCopy(opt.In(), qwordVarToStack(opt.Out()))
			default:
				panic("must be array or pointer to an array")
			}
		case ir.IndirectWrite:
			addLine(fmt.Sprintf("\tmov rax, %s\n", qwordVarToStack(opt.Left())))
			addLine(fmt.Sprintf("\tmov rbx, %s\n", qwordVarToStack(opt.Right())))
			var prefix string
			var register string
			switch typeTable[opt.Left()].(typing.Pointer).ToWhat.Size() {
			case 1:
				prefix = "byte"
				register = "bl"
			case 4:
				prefix = "dword"
				register = "ebx"
			case 8:
				prefix = "qword"
				register = "rbx"
			}
			addLine(fmt.Sprintf("\tmov %s [rax], %s\n", prefix, register))
		case ir.IndirectLoad:
			addLine(fmt.Sprintf("\tmov rax, %s\n", qwordVarToStack(opt.In())))
			var prefix string
			var register string
			switch typeTable[opt.In()].(typing.Pointer).ToWhat.Size() {
			case 1:
				prefix = "byte"
				register = "al"
			case 4:
				prefix = "dword"
				register = "eax"
			case 8:
				prefix = "qword"
				register = "rax"
			}
			addLine(fmt.Sprintf("\tmov %s, %s [rax]\n", register, prefix))
			addLine(fmt.Sprintf("\tmov %s [rbp-%d], %s\n", prefix, varOffset[opt.Out()], register))
		case ir.StructMemberPtr:
			baseType := typeTable[opt.In()]
			switch baseType := baseType.(type) {
			case typing.Pointer:
				record := baseType.ToWhat.(*typing.StructRecord)
				addLine(fmt.Sprintf("\tmov rax, %s\n", qwordVarToStack(opt.In())))
				addLine(fmt.Sprintf("\tadd rax, %d\n", record.Members[opt.Extra.(string)].Offset))
			case *typing.StructRecord:
				addLine("\tmov rax, rbp\n")
				addLine(fmt.Sprintf("\tsub rax, %d\n", varOffset[opt.In()]))
				addLine(fmt.Sprintf("\tadd rax, %d\n", baseType.Members[opt.Extra.(string)].Offset))
			default:
				panic("Type checker didn't do its job")
			}
			addLine(fmt.Sprintf("\tmov %s, rax\n", qwordVarToStack(opt.Out())))
		case ir.LoadStructMember:
			// TODO does not account for size of that member atm
			baseType := typeTable[opt.In()]
			switch baseType := baseType.(type) {
			case typing.Pointer:
				record := baseType.ToWhat.(*typing.StructRecord)
				addLine(fmt.Sprintf("\tmov rax, %s\n", qwordVarToStack(opt.In())))
				addLine(fmt.Sprintf("\tmov rax, [rax+%d]\n", record.Members[opt.Extra.(string)].Offset))
				addLine(fmt.Sprintf("\tmov %s, rax\n", qwordVarToStack(opt.Out())))
			case *typing.StructRecord:
				offset := varOffset[opt.In()] - baseType.Members[opt.Extra.(string)].Offset
				if offset < 0 {
					println(opt.In())
					println(baseType.Members[opt.Extra.(string)].Offset)
					panic("bad struct member offset")
				}
				addLine(fmt.Sprintf("\tmov rax, [rbp-%d]\n", offset))
				addLine(fmt.Sprintf("\tmov %s, rax\n", qwordVarToStack(opt.Out())))
			default:
				panic("Type checker didn't do its job")
			}
		case ir.BoolAnd:
			addLine(fmt.Sprintf("\tmov al, %s\n", byteVarToStack(opt.Right())))
			addLine(fmt.Sprintf("\tand %s, al\n", byteVarToStack(opt.Left())))
		case ir.BoolOr:
			addLine(fmt.Sprintf("\tmov al, %s\n", byteVarToStack(opt.Right())))
			addLine(fmt.Sprintf("\tor %s, al\n", byteVarToStack(opt.Left())))
		case ir.Return:
			returnExtra := opt.Extra.(ir.ReturnExtra)
			addLine("\tmov rax, rbp\n")
			addLine(fmt.Sprintf("\tsub rax, %d\n", varOffset[returnExtra.Values[0]]))
			addLine("\tmov rsp, rbp\n")
			addLine("\tpop rbp\n")
			addLine("\tret\n")
		default:
			panic(opt)
		}
	}
}

func backend(out io.Writer, labelGen *frontend.LabelIdGen, opts []frontend.OptBlock, typeTable []typing.TypeRecord, globalEnv *typing.EnvRecord, typer *typing.Typer) {
	var staticDataBuf bytes.Buffer
	for _, block := range opts {
		backendForOptBlock(out, &staticDataBuf, labelGen, block, typeTable, globalEnv, typer)
	}
	io.WriteString(out, "; ---static data segment start---\n")
	staticDataBuf.WriteTo(out)
}

// resolve all the type of members in structs and build the global environment
func buildGlobalEnv(typer *typing.Typer, env *typing.EnvRecord, nodeToStruct map[*interface{}]*typing.StructRecord, workOrders []*frontend.ProcWorkOrder) error {
	notDone := make(map[parsing.IdName][]*typing.StructField)
	type argsMut struct {
		argTypes []typing.TypeRecord
		idx      int
	}
	argsNotDone := make(map[parsing.IdName][]argsMut)
	for _, structRecord := range nodeToStruct {
		for _, field := range structRecord.Members {
			unresolved, isUnresolved := field.Type.(typing.Unresolved)
			name := unresolved.Decl.Base
			if isUnresolved {
				notDone[name] = append(notDone[name], field)
			}
		}
	}
	for _, order := range workOrders {
		argRecords := make([]typing.TypeRecord, len(order.ProcDecl.Args))
		for i, argDecl := range order.ProcDecl.Args {
			record := typer.ConstructTypeRecord(argDecl.Type)
			if _, recordIsUnresolved := record.(typing.Unresolved); recordIsUnresolved {
				if argDecl.Type.ArrayBase != nil {
					panic("Not implemented")
				}
				argsNotDone[argDecl.Type.Base] = append(argsNotDone[argDecl.Type.Base], argsMut{
					argRecords,
					i,
				})
			}
			argRecords[i] = record
		}
		env.Procs[order.Name] = typing.ProcRecord{
			typer.ConstructTypeRecord(order.ProcDecl.Return),
			argRecords,
			typing.Cdecl,
		}
	}

	numResolved := 0
	numArgResolved := 0
	for node, structRecord := range nodeToStruct {
		structNode := (*node).(parsing.StructDeclare)
		name := structNode.Name
		increment := 0
		argIncrement := 0
		for _, field := range notDone[name] {
			increment = 1
			// safe because we checked above
			unresolved := field.Type.(typing.Unresolved)
			field.Type = typing.BuildPointer(structRecord, unresolved.Decl.LevelOfIndirection)
		}
		for _, mutRecord := range argsNotDone[name] {
			argIncrement = 1
			// safe because we checked above
			unresolved := mutRecord.argTypes[mutRecord.idx].(typing.Unresolved)
			mutRecord.argTypes[mutRecord.idx] = typing.BuildPointer(structRecord, unresolved.Decl.LevelOfIndirection)
		}
		numResolved += increment
		numArgResolved += argIncrement
		env.Types[structNode.Name] = structRecord
	}
	for _, structRecord := range nodeToStruct {
		structRecord.ResolveSizeAndOffset()
	}
	if len(notDone) != numResolved {
		for typeName := range notDone {
			return fmt.Errorf("%s does not name a type", typeName)
		}
	}
	if len(argsNotDone) != numArgResolved {
		for typeName := range argsNotDone {
			return fmt.Errorf("%s does not name a type", typeName)
		}
	}

	return nil
}

func main() {
	outputPath := flag.String("o", "a.out", "path to the binary")
	flag.Parse()
	args := flag.Args()
	if len(args) < 1 {
		log.Fatal("No input file specified")
	}
	sourcePath := args[0]

	source, err := os.Open(sourcePath)
	if err != nil {
		fmt.Printf("Could not open \"%s\"\n", sourcePath)
		os.Exit(1)
	}
	defer source.Close()

	scanner := bufio.NewScanner(source)
	var workOrders []*frontend.ProcWorkOrder
	var labelGen frontend.LabelIdGen
	parser := parsing.NewParser()
	typer := typing.NewTyper()
	var currentProc *interface{}
	var nodesForProc []*interface{}
	env := typing.NewEnvRecord(typer)
	structs := make(map[*interface{}]*typing.StructRecord)

	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			continue
		}
		numNewEntries, err := parser.FeedLine(line)
		if err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
		}
		if numNewEntries == 0 {
			continue
		}

		last := len(parser.OutBuffer) - 1
		isComplete := parser.OutBuffer[last].IsComplete
		node := parser.OutBuffer[last].Node
		parent := parser.OutBuffer[last].Parent

		exprNode, isExpr := (*node).(parsing.ExprNode)
		if !isComplete && isExpr && exprNode.Op == parsing.ConstDeclare {
			_, isProc := exprNode.Right.(parsing.ProcNode)
			if isProc {
				currentProc = node
			}
			continue
		}

		if _, isEnd := (*node).(parsing.BlockEnd); isEnd && parent == currentProc {
			procDeclare := (*currentProc).(parsing.ExprNode)
			procNode := procDeclare.Right.(parsing.ProcNode)
			procName := procDeclare.Left.(parsing.IdName)
			order := frontend.ProcWorkOrder{
				Out:      make(chan frontend.OptBlock),
				In:       nodesForProc,
				Name:     procName,
				ProcDecl: procNode.ProcDecl,
			}
			workOrders = append(workOrders, &order)
			go frontend.GenForProc(&labelGen, &order)
			nodesForProc = nil
			currentProc = nil
			continue
		}

		if structDeclare, isStructDeclare := (*node).(parsing.StructDeclare); isStructDeclare {
			newStruct := typing.StructRecord{
				Name:    string(structDeclare.Name),
				Members: make(map[string]*typing.StructField),
			}
			structs[node] = &newStruct
		}

		if typeDeclare, isDecl := (*node).(parsing.Declaration); isDecl {
			parentStruct, found := structs[parent]
			if found {
				newField := &typing.StructField{
					Type: typer.ConstructTypeRecord(typeDeclare.Type),
				}
				parentStruct.MemberOrder = append(parentStruct.MemberOrder, newField)
				parentStruct.Members[string(typeDeclare.Name)] = newField
			}
		}
		if currentProc != nil {
			for i := numNewEntries; i > 0; i-- {
				nodesForProc = append(nodesForProc, parser.OutBuffer[len(parser.OutBuffer)-i].Node)
			}
		}
		// fmt.Println("Line ", line)
		// fmt.Println("Gave: ")
		// parsing.Dump(parser.OutBuffer[len(parser.OutBuffer)-numNewEntries:])
	}
	out, err := os.Create("a.asm")
	if err != nil {
		fmt.Printf("Could not create temporary asm file\n")
		os.Exit(1)
	}
	defer out.Close()

	writeAssemblyPrologue(out)

	err = buildGlobalEnv(typer, env, structs, workOrders)
	if err != nil {
		panic(err)
	}
	// fmt.Printf("%#v\n", env.Types)

	for _, workOrder := range workOrders {
		ir := <-workOrder.Out
		// frontend.DumpIr(ir)
		frontend.Prune(&ir)
		frontend.DumpIr(ir)
		// parsing.Dump(env)
		typeTable, err := typer.InferAndCheck(env, &ir, env.Procs[workOrder.Name])
		if err != nil {
			panic(err)
		}
		for i, e := range typeTable {
			if e == nil {
				println(i)
				panic("Bug in typer -- not all vars have types!")
			}
		}
		backend(out, &labelGen, []frontend.OptBlock{ir}, typeTable, env, typer)
	}
	writeDecimalTable(out)

	cmd := exec.Command("nasm", "-felf64", "a.asm")
	err = cmd.Start()
	if err != nil {
		fmt.Printf("Could not start nasm\n")
		os.Exit(1)
	}
	err = cmd.Wait()
	if err != nil {
		fmt.Printf("nasm call failed %v\n", err)
		os.Exit(1)
	}
	cmd = exec.Command("ld", "-o", *outputPath, "a.o")
	err = cmd.Start()
	if err != nil {
		fmt.Printf("Could not start ld\n")
		os.Exit(1)
	}
	err = cmd.Wait()
	if err != nil {
		fmt.Printf("ld call failed\n")
		os.Exit(1)
	}
}
