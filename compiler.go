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

func backendForOptBlock(out io.Writer, staticDataBuf *bytes.Buffer, labelGen *frontend.LabelIdGen, block frontend.OptBlock, typeTable []typing.TypeRecord, env *typing.EnvRecord) {
	addLine := func(line string) {
		io.WriteString(out, line)
	}
	varOffset := make([]int, block.NumberOfVars)
	varOffset[0] = typeTable[0].Size()
	for i := 1; i < block.NumberOfVars; i++ {
		varOffset[i] = varOffset[i-1] + typeTable[i].Size()
	}
	varToStack := func(varNum int) string {
		return fmt.Sprintf("qword [rbp-%d]", varOffset[varNum])
	}
	framesize := 0
	for _, typeRecord := range typeTable {
		framesize += typeRecord.Size()
	}
	backendDebug(framesize, typeTable, varOffset)
	for i, opt := range block.Opts {
		fmt.Fprintf(out, ";ir line %d\n", i)
		switch opt := opt.(type) {
		case ir.Assign:
			addLine(fmt.Sprintf("\tmov rax, %s\n", varToStack(opt.Right)))
			addLine(fmt.Sprintf("\tmov %s, rax\n", varToStack(opt.Left)))
		case ir.AssignImm:
			switch value := opt.Val.(type) {
			case int:
				addLine(fmt.Sprintf("\tmov %s, %v\n", varToStack(opt.Var), opt.Val))
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
				addLine(fmt.Sprintf("\tmov %s, %s\n", varToStack(opt.Var), labelName))
			}
		case ir.Add:
			addLine(fmt.Sprintf("\tmov rax, %s\n", varToStack(opt.Right)))
			if pointer, leftIsPointer := typeTable[opt.Left].(typing.Pointer); leftIsPointer {
				addLine(fmt.Sprintf("\timul rax, %d\n", pointer.ToWhat.Size()))
			}
			addLine(fmt.Sprintf("\tadd %s, rax\n", varToStack(opt.Left)))
		case ir.Increment:
			addLine(fmt.Sprintf("\tinc %s\n", varToStack(opt.Var)))
		case ir.Decrement:
			addLine(fmt.Sprintf("\tdec %s\n", varToStack(opt.Var)))
		case ir.Sub:
			addLine(fmt.Sprintf("\tmov rax, %s\n", varToStack(opt.Right)))
			addLine(fmt.Sprintf("\tsub %s, rax\n", varToStack(opt.Left)))
		case ir.Mult:
			addLine(fmt.Sprintf("\tmov r8, %s\n", varToStack(opt.Left)))
			addLine(fmt.Sprintf("\tmov r9, %s\n", varToStack(opt.Right)))
			addLine("\timul r8, r9\n")
			addLine(fmt.Sprintf("\tmov %s, r8\n", varToStack(opt.Left)))
		case ir.Div:
			addLine("\txor rdx, rdx\n")
			addLine(fmt.Sprintf("\tmov rax, %s\n", varToStack(opt.Left)))
			addLine(fmt.Sprintf("\tmov r8, %s\n", varToStack(opt.Right)))
			addLine("\tidiv r8\n")
			addLine(fmt.Sprintf("\tmov %s, rax\n", varToStack(opt.Left)))
		case ir.JumpIfFalse:
			addLine(fmt.Sprintf("\tmov al, byte %s\n", varToStack(opt.VarToCheck)))
			addLine("\tcmp al, 0\n")
			addLine(fmt.Sprintf("\tjz %s\n", opt.Label))
		case ir.Call:
			if _, isStruct := env.Types[parsing.IdName(opt.Label)]; isStruct {
				// TODO: code to zero the members
			} else if opt.Label == "cast" {
				// TODO: HACK!!!!
				addLine(fmt.Sprintf("\tmov rax, %s\n", varToStack(opt.ArgVars[0])))
				addLine(fmt.Sprintf("\tmov %s, rax\n", varToStack(opt.Out)))
			} else {
				// TODO: temporary
				addLine(fmt.Sprintf("\tmov rax, %s\n", varToStack(opt.ArgVars[0])))
				addLine(fmt.Sprintf("\tcall %s\n", opt.Label))
			}
		case ir.Jump:
			addLine(fmt.Sprintf("\tjmp %s\n", opt.Label))
		case ir.Label:
			addLine(fmt.Sprintf("%s:\n", opt.Name))
		case ir.StartProc:
			addLine(fmt.Sprintf("%s:\n", opt.Name))
			addLine("\tpush rbp\n")
			addLine("\tmov rbp, rsp\n")
			addLine(fmt.Sprintf("\tsub rsp, %d\n", framesize))
		case ir.EndProc:
			addLine("\tmov rsp, rbp\n")
			addLine("\tpop rbp\n")
			addLine("\tret\n")
		case ir.Compare:
			addLine(fmt.Sprintf("\tmov rax, %s\n", varToStack(opt.Left)))
			addLine(fmt.Sprintf("\tmov rbx, %s\n", varToStack(opt.Right)))
			addLine(fmt.Sprintf("\tmov byte %s, 1\n", varToStack(opt.Out)))
			addLine("\tcmp rax, rbx\n")
			labelName := labelGen.GenLabel("cmp%d")
			switch opt.How {
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
			addLine(fmt.Sprintf("\tmov byte %s, 0\n", varToStack(opt.Out)))
			addLine(fmt.Sprintf("%s:\n", labelName))
		case ir.Transclude:
			panic("Transcludes should be gone by now")
		case ir.TakeAddress:
			dest := varToStack(opt.Out)
			addLine(fmt.Sprintf("\tmov %s, rbp\n", dest))
			addLine(fmt.Sprintf("\tsub %s, %d\n", dest, varOffset[opt.Var]))
		case ir.IndirectWrite:
			addLine(fmt.Sprintf("\tmov rax, %s\n", varToStack(opt.Pointer)))
			addLine(fmt.Sprintf("\tmov rbx, %s\n", varToStack(opt.Data)))
			// TODO not all writes are qword
			addLine("\tmov qword [rax], rbx\n")
		case ir.IndirectLoad:
			addLine(fmt.Sprintf("\tmov rax, %s\n", varToStack(opt.Pointer)))
			addLine("\tmov rax, [rax]\n")
			addLine(fmt.Sprintf("\tmov %s, rax\n", varToStack(opt.Out)))
		case ir.StructMemberPtr:
			baseType := typeTable[opt.Base]
			switch baseType := baseType.(type) {
			case typing.Pointer:
				record := baseType.ToWhat.(*typing.StructRecord)
				addLine(fmt.Sprintf("\tmov rax, %s\n", varToStack(opt.Base)))
				addLine(fmt.Sprintf("\tadd rax, %d\n", record.Members[opt.Member].Offset))
			case *typing.StructRecord:
				addLine("\tmov rax, rbp\n")
				addLine(fmt.Sprintf("\tsub rax, %d\n", varOffset[opt.Base]))
				addLine(fmt.Sprintf("\tadd rax, %d\n", baseType.Members[opt.Member].Offset))
			default:
				panic("Type checker didn't do its job")
			}
			addLine(fmt.Sprintf("\tmov %s, rax\n", varToStack(opt.Out)))
		case ir.LoadStructMember:
			// TODO does not account for size of that member atm
			baseType := typeTable[opt.Base]
			switch baseType := baseType.(type) {
			case typing.Pointer:
				record := baseType.ToWhat.(*typing.StructRecord)
				addLine(fmt.Sprintf("\tmov rax, %s\n", varToStack(opt.Base)))
				addLine(fmt.Sprintf("\tmov rax, [rax+%d]\n", record.Members[opt.Member].Offset))
				addLine(fmt.Sprintf("\tmov %s, rax\n", varToStack(opt.Out)))
			case *typing.StructRecord:
				offset := varOffset[opt.Base] - baseType.Members[opt.Member].Offset
				if offset < 0 {
					println(opt.Base)
					println(baseType.Members[opt.Member].Offset)
					panic("bad struct member offset")
				}
				addLine(fmt.Sprintf("\tmov rax, [rbp-%d]\n", offset))
				addLine(fmt.Sprintf("\tmov %s, rax\n", varToStack(opt.Out)))
			default:
				panic("Type checker didn't do its job")
			}
		default:
			panic(opt)
		}
	}
}

func backend(out io.Writer, labelGen *frontend.LabelIdGen, opts []frontend.OptBlock, typeTable []typing.TypeRecord, globalEnv *typing.EnvRecord) {
	var staticDataBuf bytes.Buffer
	for _, block := range opts {
		backendForOptBlock(out, &staticDataBuf, labelGen, block, typeTable, globalEnv)
	}
	io.WriteString(out, "; ---static data segment start---\n")
	staticDataBuf.WriteTo(out)
}

// resolve all the type of members in structs and build the global environment
func buildGlobalEnv(typer *typing.Typer, env *typing.EnvRecord, nodeToStruct map[*interface{}]*typing.StructRecord) error {
	notDone := make(map[parsing.IdName][]*typing.StructField)
	for _, structRecord := range nodeToStruct {
		for _, field := range structRecord.Members {
			unresolved, isUnresolved := field.Type.(typing.Unresolved)
			name := unresolved.Decl.Base
			if isUnresolved {
				notDone[name] = append(notDone[name], field)
			}
		}
	}

	numResolved := 0
	for node, structRecord := range nodeToStruct {
		structNode := (*node).(parsing.StructDeclare)
		name := structNode.Name
		increment := 0
		for _, field := range notDone[name] {
			increment = 1
			// safe because we checked above
			unresolved := field.Type.(typing.Unresolved)
			field.Type = typing.BuildPointer(structRecord, unresolved.Decl.LevelOfIndirection)
		}
		numResolved += increment
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
	blocks := make(map[*interface{}]*frontend.ProcWorkOrder)
	var labelGen frontend.LabelIdGen
	parser := parsing.NewParser()
	typer := typing.NewTyper()
	var mainProc *interface{}
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

		last := len(parser.OutBuffer) - 1
		isComplete := parser.OutBuffer[last].IsComplete
		node := parser.OutBuffer[last].Node
		parent := parser.OutBuffer[last].Parent

		exprNode, isExpr := (*node).(parsing.ExprNode)
		if !isComplete && isExpr && exprNode.Op == parsing.ConstDeclare {
			_, isProc := exprNode.Right.(parsing.ProcNode)
			if isProc {
				currentProc = node
				if exprNode.Left.(parsing.IdName) == "main" {
					mainProc = node
				}
			}
			continue
		}

		if _, isEnd := (*node).(parsing.BlockEnd); isEnd && parent == currentProc {
			procName := string((*currentProc).(parsing.ExprNode).Left.(parsing.IdName))
			label := "proc_" + procName
			order := frontend.ProcWorkOrder{
				Out:   make(chan frontend.OptBlock),
				In:    nodesForProc,
				Label: label,
			}
			blocks[currentProc] = &order
			go frontend.GenForProc(&labelGen, &order)
			nodesForProc = nil
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
	}
	out, err := os.Create("a.asm")
	if err != nil {
		fmt.Printf("Could not create temporary asm file\n")
		os.Exit(1)
	}
	defer out.Close()

	fmt.Fprintln(out, "\tglobal _start")
	fmt.Fprintln(out, "\tsection .text")

	fmt.Fprintln(out, `_start:
	call proc_main
	mov eax, 60
	xor rdi, rdi
	syscall`)

	fmt.Fprintln(out, `exit:
	mov rdi, rax
	mov eax, 60
	syscall`)

	fmt.Fprintln(out, "puts:")
	fmt.Fprintln(out, "\tmov rdx, [rax]")
	fmt.Fprintln(out, "\tmov rsi, rax")
	fmt.Fprintln(out, "\tadd rsi, 8")
	fmt.Fprintln(out, "\tmov rax, 1")
	fmt.Fprintln(out, "\tmov rdi, 1")
	fmt.Fprintln(out, "\tsyscall")
	fmt.Fprintln(out, "\tret")

	ir := <-blocks[mainProc].Out
	frontend.Prune(&ir)
	frontend.DumpIr(ir)
	err = buildGlobalEnv(typer, env, structs)
	if err != nil {
		panic(err)
	}
	// parsing.Dump(env)
	typeTable, err := typer.InferAndCheck(env, &ir)
	if err != nil {
		panic(err)
	}
	for i, e := range typeTable {
		if e == nil {
			println(i)
			panic("Bug in typer -- not all vars have types!")
		}
	}
	backend(out, &labelGen, []frontend.OptBlock{ir}, typeTable, env)

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
