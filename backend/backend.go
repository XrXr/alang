package backend

import (
	"bytes"
	"fmt"
	"github.com/XrXr/alang/frontend"
	"github.com/XrXr/alang/ir"
	"github.com/XrXr/alang/parsing"
	"github.com/XrXr/alang/typing"
	"io"
)

type outputBlock struct {
	buffer *bytes.Buffer
	next   *outputBlock
}

func newOutputBlock() *outputBlock {
	var block outputBlock
	block.buffer = new(bytes.Buffer)
	return &block
}

type procGen struct {
	out              *outputBlock
	firstOutputBlock *outputBlock
	staticDataBuf    *bytes.Buffer
	block            frontend.OptBlock
	typeTable        []typing.TypeRecord
	env              *typing.EnvRecord
	typer            *typing.Typer
}

func backendDebug(framesize int, typeTable []typing.TypeRecord, offsetTable []int) {
	fmt.Println("framesize", framesize)
	if len(typeTable) != len(offsetTable) {
		panic("what?")
	}
	for i, typeRecord := range typeTable {
		fmt.Printf("var %d type: %#v offset %d\n", i, typeRecord, offsetTable[i])
	}
}

func (p *procGen) switchToNewOutBlock() {
	current := p.out
	p.out = newOutputBlock()
	current.next = p.out
}

func (p *procGen) backendForOptBlock() {
	nextId := 1
	addLine := func(line string) {
		io.WriteString(p.out.buffer, line)
	}
	varOffset := make([]int, p.block.NumberOfVars)
	genLabel := func(prefix string) string {
		label := fmt.Sprintf("%s_%d", prefix, nextId)
		nextId++
		return label
	}

	if p.block.NumberOfArgs > 0 {
		// we push rbp in the prologue and call pushes the return address
		varOffset[0] = -16
		for i := 1; i < p.block.NumberOfArgs; i++ {
			varOffset[i] = varOffset[i-1] - p.typeTable[i-1].Size()
		}
	}

	firstLocal := p.block.NumberOfArgs
	if firstLocal < p.block.NumberOfVars {
		varOffset[firstLocal] = p.typeTable[firstLocal].Size()
		for i := firstLocal + 1; i < p.block.NumberOfVars; i++ {
			varOffset[i] = varOffset[i-1] + p.typeTable[i].Size()
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
		soruceType := p.typeTable[sourceVarNum]
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
			// search for :morecopies
			// panic("need a complex copy")
		}
	}

	oprandString := func(vn int) string {
		dest := "bad addressing mode"
		switch p.typeTable[vn].Size() {
		case 1:
			dest = byteVarToStack(vn)
		case 4:
			dest = wordVarToStack(vn)
		case 8:
			dest = qwordVarToStack(vn)
		}
		return dest
	}

	framesize := 0
	for _, typeRecord := range p.typeTable {
		framesize += typeRecord.Size()
	}
	if framesize%16 != 0 {
		// align the stack for SystemV abi. Upon being called, we are 8 bytes misaligned.
		// Since we push rbp in our prologue we align to 16 here
		framesize += 16 - framesize%16
	}
	backendDebug(framesize, p.typeTable, varOffset)
	for i, opt := range p.block.Opts {
		addLine(fmt.Sprintf(";ir line %d\n", i))
		switch opt.Type {
		case ir.Assign:
			simpleCopy(opt.Right(), oprandString(opt.Left()))
		case ir.AssignImm:
			switch value := opt.Extra.(type) {
			case int64, uint64, int:
				addLine(fmt.Sprintf("\tmov %s, %d\n", oprandString(opt.Oprand1), opt.Extra))
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
					buf.WriteString(`",0`)
				} else {
					// it's a string that ends with \n
					buf.WriteRune('0')
				}

				labelName := genLabel(fmt.Sprintf("static_string_%p", p.staticDataBuf))
				p.staticDataBuf.WriteString(fmt.Sprintf("%s:\n", labelName))
				p.staticDataBuf.WriteString(fmt.Sprintf("\tdq\t%d\n", byteCount))
				p.staticDataBuf.ReadFrom(&buf)
				p.staticDataBuf.WriteRune('\n')
				addLine(fmt.Sprintf("\tmov %s, %s\n", qwordVarToStack(opt.Oprand1), labelName))
			case parsing.TypeDecl:
				// TODO zero out decl
			default:
				panic("unknown immediate value type")
			}
		case ir.Add:
			p.switchToNewOutBlock()
			pointer, leftIsPointer := p.typeTable[opt.Left()].(typing.Pointer)
			rightSize := p.typeTable[opt.Right()].Size()
			if leftIsPointer {
				switch rightSize {
				case 8:
					addLine(fmt.Sprintf("\tmov rax, %s\n", qwordVarToStack(opt.Right())))
				case 4:
					addLine(fmt.Sprintf("\tmovsx rax, %s\n", wordVarToStack(opt.Right())))
				case 1:
					addLine(fmt.Sprintf("\tmovsx eax, %s\n", byteVarToStack(opt.Right())))
					addLine("\tmovsx rax, eax\n")
				}
				addLine(fmt.Sprintf("\timul rax, %d\n", pointer.ToWhat.Size()))
				addLine(fmt.Sprintf("\tadd %s, rax\n", qwordVarToStack(opt.Left())))
			} else {
				switch rightSize {
				case 8:
					addLine(fmt.Sprintf("\tmov rax, %s\n", qwordVarToStack(opt.Right())))
				case 4:
					addLine(fmt.Sprintf("\tmov eax, %s\n", wordVarToStack(opt.Right())))
				case 1:
					addLine(fmt.Sprintf("\tmov al, %s\n", byteVarToStack(opt.Right())))
				}
				leftSize := p.typeTable[opt.Left()].Size()
				if rightSize == 1 && leftSize > 1 {
					addLine("\tmovsx eax, al\n")
				}
				switch leftSize {
				case 8:
					addLine(fmt.Sprintf("\tadd %s, rax\n", qwordVarToStack(opt.Left())))
				case 4:
					addLine(fmt.Sprintf("\tadd %s, eax\n", wordVarToStack(opt.Left())))
				case 1:
					addLine(fmt.Sprintf("\tadd %s, al\n", byteVarToStack(opt.Left())))
				}
			}
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
			addLine(fmt.Sprintf("\tjz .%s\n", opt.Extra.(string)))
		case ir.JumpIfTrue:
			addLine(fmt.Sprintf("\tmov al, %s\n", byteVarToStack(opt.In())))
			addLine("\tcmp al, 0\n")
			addLine(fmt.Sprintf("\tjnz .%s\n", opt.Extra.(string)))
		case ir.Call:
			extra := opt.Extra.(ir.CallExtra)
			if _, isStruct := p.env.Types[parsing.IdName(extra.Name)]; isStruct {
				// TODO: code to zero the members
			} else {
				// TODO: this can be done once
				totalArgSize := 0
				for _, arg := range extra.ArgVars {
					totalArgSize += p.typeTable[arg].Size()
				}
				var numExtraArgs int

				procRecord := p.env.Procs[parsing.IdName(extra.Name)]
				switch procRecord.CallingConvention {
				case typing.Cdecl:
					addLine(fmt.Sprintf("\tsub rsp, %d\n", totalArgSize))
					offset := 0

					for _, arg := range extra.ArgVars {
						thisArgSize := p.typeTable[arg].Size()
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
				case typing.SystemV:
					regOrder := [...]string{"rdi", "rsi", "rdx", "rcx", "r8", "r9"}
					regOrderSmaller := [...]string{"edi", "esi", "edx", "ecx", "r8d", "r9d"}

					for i, arg := range extra.ArgVars {
						if i >= len(regOrder) {
							break
						}
						switch p.typeTable[arg].Size() {
						case 8:
							addLine(fmt.Sprintf("\tmov %s, %s\n", regOrder[i], qwordVarToStack(arg)))
						case 4:
							addLine(fmt.Sprintf("\tmovsx %s, %s\n", regOrder[i], wordVarToStack(arg)))
						case 1:
							addLine(fmt.Sprintf("\tmovsx %s, %s\n", regOrderSmaller[i], byteVarToStack(arg)))
						default:
							panic("Unsupported parameter size")
						}
					}
					if len(extra.ArgVars) > len(regOrder) {
						numExtraArgs = len(extra.ArgVars) - len(regOrder)
						if numExtraArgs%2 == 1 {
							// Make sure we are aligned to 16
							addLine("\tsub rsp, 8\n")
						}
						for i := len(extra.ArgVars) - 1; i >= len(extra.ArgVars)-numExtraArgs; i-- {
							arg := extra.ArgVars[i]
							switch p.typeTable[arg].Size() {
							case 8:
								addLine(fmt.Sprintf("\tpush %s\n", qwordVarToStack(arg)))
							case 4:
								addLine(fmt.Sprintf("\tmov eax, %s\n", wordVarToStack(arg)))
								addLine("\tpush rax\n")
							case 1:
								addLine(fmt.Sprintf("\tmov al, %s\n", byteVarToStack(arg)))
								addLine("\tpush rax\n")
							default:
								panic("Unsupported parameter size")
							}
						}
					}
				}
				if procRecord.IsForeign {
					addLine(fmt.Sprintf("\tcall %s\n", extra.Name))
				} else {
					addLine(fmt.Sprintf("\tcall proc_%s\n", extra.Name))
				}
				switch procRecord.CallingConvention {
				case typing.SystemV:
					// TODO this needs to change when we support things bigger than 8 bytes
					if len(extra.ArgVars) > 6 {
						addLine(fmt.Sprintf("\tadd rsp, %d\n", numExtraArgs*8+numExtraArgs%2*8))
					}
					switch p.typeTable[opt.Oprand1].Size() {
					case 1:
						addLine(fmt.Sprintf("\tmov %s, al\n", byteVarToStack(opt.Oprand1)))
					case 4:
						addLine(fmt.Sprintf("\tmov %s, eax\n", wordVarToStack(opt.Oprand1)))
					case 8:
						addLine(fmt.Sprintf("\tmov %s, rax\n", qwordVarToStack(opt.Oprand1)))
					}
				case typing.Cdecl:
					addLine(fmt.Sprintf("\tadd rsp, %d\n", totalArgSize))
					if p.typeTable[opt.Oprand1].Size() > 0 {
						if procRecord.IsForeign {
							addLine(fmt.Sprintf("\tmov %s, rax\n", qwordVarToStack(opt.Oprand1)))
						} else {
							returnType := procRecord.Return
							addLine(fmt.Sprintf("\tmov rdx, %d\n", returnType.Size()))
							addLine(fmt.Sprintf("\tlea rdi, [rbp-%d]\n", varOffset[opt.Oprand1]))
							addLine("\tmov rsi, rax\n")
							addLine("\tcall _intrinsic_memcpy\n")
						}
					}
				}
			}
		case ir.Jump:
			addLine(fmt.Sprintf("\tjmp .%s\n", opt.Extra.(string)))
		case ir.Label:
			addLine(fmt.Sprintf(".%s:\n", opt.Extra.(string)))
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
			lt := p.typeTable[opt.Left()]
			rt := p.typeTable[opt.Right()]
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
			labelName := genLabel(".cmp")
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
			switch p.typeTable[opt.In()].(type) {
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
			switch p.typeTable[opt.Left()].(typing.Pointer).ToWhat.Size() {
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
			switch p.typeTable[opt.In()].(typing.Pointer).ToWhat.Size() {
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
			baseType := p.typeTable[opt.In()]
			fieldName := opt.Extra.(string)
			switch baseType := baseType.(type) {
			case typing.Pointer:
				record := baseType.ToWhat.(*typing.StructRecord)
				addLine(fmt.Sprintf("\tmov rax, %s\n", qwordVarToStack(opt.In())))
				addLine(fmt.Sprintf("\tadd rax, %d\n", record.Members[fieldName].Offset))
			case *typing.StructRecord:
				addLine(fmt.Sprintf("\tlea rax, [rbp-%d+%d]\n", varOffset[opt.In()], baseType.Members[fieldName].Offset))
			case typing.String:
				addLine(fmt.Sprintf("\tmov rax, %s\n", qwordVarToStack(opt.In())))
				switch fieldName {
				case "data":
					addLine("\tadd rax, 8\n")
				case "length":
					// it's pointing to it already
				}
			default:
				panic("Type checker didn't do its job")
			}
			addLine(fmt.Sprintf("\tmov %s, rax\n", qwordVarToStack(opt.Out())))
		case ir.LoadStructMember:
			baseType := p.typeTable[opt.In()]
			fieldName := opt.Extra.(string)

			switch baseType := baseType.(type) {
			case typing.Pointer:
				record := baseType.ToWhat.(*typing.StructRecord)
				addLine(fmt.Sprintf("\tmov rax, %s\n", qwordVarToStack(opt.In())))
				addLine(fmt.Sprintf("\tmov rax, [rax+%d]\n", record.Members[fieldName].Offset))
			case *typing.StructRecord:
				offset := varOffset[opt.In()] - baseType.Members[fieldName].Offset
				if offset < 0 {
					println(opt.In())
					println(baseType.Members[fieldName].Offset)
					panic("bad struct member offset")
				}
				addLine(fmt.Sprintf("\tmov rax, [rbp-%d]\n", offset))
			case typing.String:
				switch fieldName {
				case "data":
					addLine(fmt.Sprintf("\tmov rax, %s\n", qwordVarToStack(opt.In())))
					addLine("\tadd rax, 8\n")
				case "length":
					addLine(fmt.Sprintf("\tmov rax, %s\n", qwordVarToStack(opt.In())))
					addLine("\tmov rax, [rax]\n")
				}
			default:
				panic("Type checker didn't do its job")
			}
			// TODO does not account for size of that member atm :morecopies
			switch p.typeTable[opt.Out()].Size() {
			case 8:
				addLine(fmt.Sprintf("\tmov %s, rax\n", qwordVarToStack(opt.Out())))
			case 4:
				addLine(fmt.Sprintf("\tmov %s, eax\n", wordVarToStack(opt.Out())))
			case 1:
				addLine(fmt.Sprintf("\tmov %s, al\n", byteVarToStack(opt.Out())))
			}
		case ir.Not:
			setLabel := genLabel(".not")
			addLine(fmt.Sprintf("\tmov %s, 0\n", byteVarToStack(opt.Out())))
			switch p.typeTable[opt.In()].(type) {
			case typing.Pointer:
				addLine(fmt.Sprintf("\tmov rax, %s\n", qwordVarToStack(opt.In())))
				addLine(fmt.Sprintf("\tcmp rax, 0\n"))
			case typing.Boolean:
				addLine(fmt.Sprintf("\tmov al, %s\n", byteVarToStack(opt.In())))
				addLine(fmt.Sprintf("\tcmp al, 0\n"))
			}
			addLine(fmt.Sprintf("\tjnz %s\n", setLabel))
			addLine(fmt.Sprintf("\tmov %s, 1\n", byteVarToStack(opt.Out())))
			addLine(fmt.Sprintf("%s:\n", setLabel))
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

func X86ForBlock(out io.Writer, block frontend.OptBlock, typeTable []typing.TypeRecord, globalEnv *typing.EnvRecord, typer *typing.Typer) *bytes.Buffer {
	firstOut := newOutputBlock()
	var staticDataBuf bytes.Buffer
	gen := procGen{
		out:              firstOut,
		firstOutputBlock: firstOut,
		block:            block,
		typeTable:        typeTable,
		env:              globalEnv,
		typer:            typer,
		staticDataBuf:    &staticDataBuf}
	gen.backendForOptBlock()
	outBlock := gen.firstOutputBlock
	for outBlock != nil {
		_, err := outBlock.buffer.WriteTo(out)
		if err != nil {
			panic(err)
		}
		outBlock = outBlock.next
	}
	return &staticDataBuf
}
