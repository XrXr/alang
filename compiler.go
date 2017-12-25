package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"github.com/XrXr/alang/ir"
	"github.com/XrXr/alang/parsing"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"sync"
)

type labelIdGen struct {
	sync.Mutex
	availableId int
}

func (g *labelIdGen) genLabel(template string) (ret string) {
	g.Lock()
	ret = fmt.Sprintf(template, g.availableId)
	g.availableId++
	g.Unlock()
	return
}

// An output from genForBlock can either be a solid line or a transclusion of
// another block. There are no unions in golang, so we use this
type codeGenCommand struct {
	isTransclude bool
	line         string
	transclude   *interface{}
}

var paramOrder = []string{"rax", "rbx"}

const (
	Proc = iota + 1
	If
	Else
)

type blockInfo struct {
	code       chan []interface{}
	feed       chan *interface{}
	ownerType  int
	label      string
	upToVarNum int
}

type frontendGen struct {
	opts       []interface{}
	nextVarNum int
	varTable   map[parsing.IdName]int
	sawIf      bool
}

type optBlock struct {
	varOffset int
	opts      []interface{}
}

func (f *frontendGen) addOpt(opt interface{}) {
	f.opts = append(f.opts, opt)
}

func (f *frontendGen) newVar() int {
	current := f.nextVarNum
	f.nextVarNum++
	return current
}

func varTemp(varNum int) string {
	return fmt.Sprintf("$var%d", varNum)
}

func genForBlock(labelGen *labelIdGen, info *blockInfo) {
	var segments [][]interface{}
	lastSegmentEnd := 0
	var gen frontendGen
	gen.varTable = make(map[parsing.IdName]int)

	if info.ownerType == Proc {
		gen.addOpt(ir.StartProc{info.label})
	}

	for nodePtr := range info.feed {
		sawIfLastTime := gen.sawIf
		gen.sawIf = false
		switch node := (*nodePtr).(type) {
		case parsing.ExprNode:
			switch node.Op {
			case parsing.Declare:
				varNum := gen.newVar()
				gen.varTable[node.Left.(parsing.IdName)] = varNum
				err := genSimpleValuedExpression(&gen, varNum, node.Right)
				if err != nil {
					panic(err)
				}
			case parsing.Assign:
				leftVarNum, varFound := gen.varTable[node.Left.(parsing.IdName)]
				if !varFound {
					panic("bug in user program! assign to undefined var")
				}
				err := genSimpleValuedExpression(&gen, leftVarNum, node.Right)
				if err != nil {
					panic(err)
				}
			default:
				//TODO issue warning here
			}
		case parsing.IfNode:
			condVar := gen.newVar()
			genSimpleValuedExpression(&gen, condVar, node.Condition)
			labelForIf := labelGen.genLabel("if_%d")
			gen.addOpt(ir.JumpIfFalse{condVar, labelForIf})
			gen.addOpt(ir.Transclude{nodePtr})
			segments = append(segments, gen.opts[lastSegmentEnd:])
			lastSegmentEnd = len(gen.opts)
			gen.addOpt(ir.Label{labelForIf})
			gen.sawIf = true
		case parsing.ElseNode:
			if !sawIfLastTime {
				panic("Bare else. Should've been caught by the parser")
			}
			elseLabel := labelGen.genLabel("else_%d")
			ifLabel := gen.opts[len(gen.opts)-1]
			gen.opts[len(gen.opts)-1] = ir.Jump{elseLabel}
			gen.addOpt(ifLabel)
			gen.addOpt(ir.Transclude{nodePtr})
			segments = append(segments, gen.opts[lastSegmentEnd:])
			lastSegmentEnd = len(gen.opts)
			gen.addOpt(ir.Label{elseLabel})
		case parsing.ProcCall:
			var argVars []int
			for _, argNode := range node.Args {
				argEval := gen.newVar()
				err := genSimpleValuedExpression(&gen, argEval, argNode)
				if err != nil {
					panic(err)
				}
				argVars = append(argVars, argEval)
			}
			gen.addOpt(ir.Call{string(node.Callee), argVars})
		}
	}

	if info.ownerType == Proc {
		gen.addOpt(ir.EndProc{})
	}

	if len(gen.opts[lastSegmentEnd:]) != 0 {
		segments = append(segments, gen.opts[lastSegmentEnd:])
		lastSegmentEnd = len(gen.opts)
	}

	info.upToVarNum = gen.nextVarNum
	for _, segment := range segments {
		info.code <- segment
	}
	close(info.code)
}

func genSimpleValuedExpression(gen *frontendGen, dest int, node interface{}) error {
	switch n := node.(type) {
	case parsing.Literal:
		var value interface{}
		switch n.Type {
		case parsing.Number:
			v, err := strconv.Atoi(n.Value)
			if err != nil {
				panic(err)
			}
			value = v
		case parsing.Boolean:
			value = boolStrToInt(n.Value)
		case parsing.String:
			value = n.Value
		}
		gen.addOpt(ir.AssignImm{dest, value})
	case parsing.IdName:
		gen.addOpt(ir.Assign{dest, gen.varTable[n]})
	case parsing.ExprNode:
		switch n.Op {
		case parsing.Star, parsing.Minus, parsing.Plus, parsing.Divide:
			leftDest := gen.newVar()
			err := genSimpleValuedExpression(gen, leftDest, n.Left)
			if err != nil {
				return err
			}
			rightDest := gen.newVar()
			err = genSimpleValuedExpression(gen, rightDest, n.Right)
			if err != nil {
				return err
			}
			switch n.Op {
			case parsing.Star:
				gen.addOpt(ir.Mult{leftDest, rightDest})
			case parsing.Divide:
				gen.addOpt(ir.Div{leftDest, rightDest})
			case parsing.Plus:
				gen.addOpt(ir.Add{leftDest, rightDest})
			case parsing.Minus:
				gen.addOpt(ir.Sub{leftDest, rightDest})
			}
			gen.addOpt(ir.Assign{dest, leftDest})
		default:
			return errors.New(fmt.Sprintf("Unsupported value expression type %v", n.Op))
		}
	}
	return nil
}

func backendForOptBlock(out io.Writer, staticDataBuf *bytes.Buffer, labelGen *labelIdGen, block optBlock) {
	addLine := func(line string) {
		io.WriteString(out, line)
	}
	varToStack := func(varNum int) string {
		acutalVar := varNum + block.varOffset
		//TODO: !! of course not every var is size 8...
		return fmt.Sprintf("qword [rbp-%d]", acutalVar*8)
	}
	for _, opt := range block.opts {
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

				labelName := labelGen.genLabel("label%d")
				staticDataBuf.WriteString(fmt.Sprintf("%s:\n", labelName))
				staticDataBuf.WriteString(fmt.Sprintf("\tdq\t%d\n", byteCount))
				staticDataBuf.ReadFrom(&buf)
				staticDataBuf.WriteRune('\n')
				addLine(fmt.Sprintf("\tmov %s, %s\n", varToStack(opt.Var), labelName))
			}
		case ir.Add:
			addLine(fmt.Sprintf("\tmov rax, %s\n", varToStack(opt.Right)))
			addLine(fmt.Sprintf("\tadd %s, rax\n", varToStack(opt.Left)))
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
			addLine(fmt.Sprintf("\tmov rax, %s\n", varToStack(opt.VarToCheck)))
			addLine("\tcmp rax, 0\n")
			addLine(fmt.Sprintf("\tjz %s\n", opt.Label))
		case ir.Call:
			// TODO: temporary
			addLine(fmt.Sprintf("\tmov rax, %s\n", varToStack(opt.ArgVars[0])))
			addLine(fmt.Sprintf("\tcall %s\n", opt.Label))
		case ir.Jump:
			addLine(fmt.Sprintf("\tjmp %s\n", opt.Label))
		case ir.Label:
			addLine(fmt.Sprintf("%s:\n", opt.Name))
		case ir.StartProc:
			addLine(fmt.Sprintf("%s:\n", opt.Name))
			addLine("\tpush rbp\n")
			addLine("\tmov rbp, rsp\n")
		case ir.EndProc:
			addLine("\tpop rbp\n")
			addLine("\tret\n")
		case ir.Transclude:
			panic("Should be gone by now")
		default:
			panic(opt)
		}
	}
}

func backend(out io.Writer, labelGen *labelIdGen, opts []optBlock) {
	var staticDataBuf bytes.Buffer
	for _, block := range opts {
		backendForOptBlock(out, &staticDataBuf, labelGen, block)
	}
	io.WriteString(out, "; ---static data segment start---\n")
	staticDataBuf.WriteTo(out)
}

func boolStrToInt(s string) (ret int) {
	if s == "true" {
		ret = 1
	}
	return
}

func sendLinesToChan(buf *bytes.Buffer, channel *chan string) {
	for {
		line, err := buf.ReadString('\n')
		*channel <- line
		if err != nil {
			close(*channel)
			break
		}
	}
}

var varNameRegex = regexp.MustCompile(`\$var[0-9]+`)

func collectIr(mainProc *interface{}, blocks map[*interface{}]*blockInfo) []optBlock {
	var out []optBlock
	type blockConsumptionInfo struct {
		block     *interface{}
		varOffset int
		firstTime bool
	}
	nextOffset := 0

	blockConsumptionStack := []blockConsumptionInfo{{mainProc, 0, true}}
consumeLoop:
	for len(blockConsumptionStack) != 0 {
		l := len(blockConsumptionStack)
		cur := blockConsumptionStack[l-1]
		blockConsumptionStack = blockConsumptionStack[:l-1]
		info := blocks[cur.block]
		for segment := range info.code {
			if cur.firstTime {
				nextOffset += info.upToVarNum
			}
			cur.firstTime = false

			transOpt, doTransclude := segment[len(segment)-1].(ir.Transclude)
			if doTransclude {
				if cur.block == transOpt.Node {
					panic("a block tried to transclude itself")
				}

				segment = segment[:len(segment)-1]
				var irBlock optBlock
				irBlock.varOffset = cur.varOffset
				irBlock.opts = segment
				out = append(out, irBlock)
				cur.firstTime = false
				// when we transclude, put the vars from that after the vars of the current block
				blockConsumptionStack = append(blockConsumptionStack,
					cur,
					blockConsumptionInfo{transOpt.Node, nextOffset, true},
				)
				continue consumeLoop
			} else {
				var irBlock optBlock
				irBlock.varOffset = cur.varOffset

				irBlock.opts = segment
				out = append(out, irBlock)
			}
		}
	}
	return out
}

// // collect from blocks and fill in variable offset
// func collectAsm(mainProc *interface{}, blocks map[*interface{}]blockInfo, out io.Writer) {
// 	stackOffset := 0
// 	type ifBlockInfo struct {
// 		offsetBeforeEntry int
// 		offsetAfterExit   int
// 	}
// 	ifBlockInfoStack := make([]ifBlockInfo, 0)
// 	var ifEntryOffset int
// 	type blockConsumptionInfo struct {
// 		block         *interface{}
// 		prologPrinted bool
// 	}
// 	blockConsumptionStack := []blockConsumptionInfo{{mainProc, false}}

// consumeLoop:
// 	for len(blockConsumptionStack) != 0 {
// 		l := len(blockConsumptionStack)
// 		cur := blockConsumptionStack[l-1]
// 		blockConsumptionStack = blockConsumptionStack[:l-1]

// 		info := blocks[cur.block]
// 		if !cur.prologPrinted {
// 			switch info.ownerType {
// 			case Proc:
// 				fmt.Fprintf(out, "%s:\n", info.label)
// 				fmt.Fprintln(out, "\tpush rbp")
// 				fmt.Fprintln(out, "\tmov rbp, rsp")
// 			case If:
// 				ifEntryOffset = stackOffset
// 			case Else:
// 				info := ifBlockInfoStack[len(ifBlockInfoStack)-1]
// 				stackOffset = info.offsetBeforeEntry
// 			}
// 		}
// 		varTable := make(map[string]string)
// 		for cmd := range info.code {
// 			if cmd.isTransclude {
// 				if cur.block == cmd.transclude {
// 					panic("a block tried to transclude itself")
// 				}
// 				blockConsumptionStack = append(blockConsumptionStack,
// 					blockConsumptionInfo{cur.block, true},
// 					blockConsumptionInfo{cmd.transclude, false},
// 				)
// 				continue consumeLoop
// 			} else {
// 				// map "$var{number}" in this block to locations like [rsp-8]
// 				s := cmd.line
// 				match := varNameRegex.FindStringIndex(s)
// 				if match != nil {
// 					varTpl := s[match[0]:match[1]]
// 					_, found := varTable[varTpl]
// 					if !found {
// 						loc := fmt.Sprintf("qword [rbp-%d]", stackOffset)
// 						varTable[varTpl] = loc
// 						stackOffset += 8
// 					}

// 					filled := varNameRegex.ReplaceAllFunc([]byte(s), func(match []byte) []byte {
// 						loc, ok := varTable[string(match[:])]
// 						if !ok {
// 							panic("referenced an undefined variable internally")
// 						}
// 						return []byte(loc)
// 					})
// 					s = string(filled[:])
// 				}
// 				fmt.Fprint(out, s)
// 			}
// 		}
// 		// epilogue
// 		switch info.ownerType {
// 		case Proc:
// 			fmt.Fprintln(out, "\tpop rbp")
// 			fmt.Fprintln(out, "\tret")
// 		case If:
// 			ifBlockInfoStack = append(ifBlockInfoStack, ifBlockInfo{
// 				offsetBeforeEntry: ifEntryOffset,
// 				offsetAfterExit:   stackOffset,
// 			})
// 		case Else:
// 			l := len(ifBlockInfoStack)
// 			info := ifBlockInfoStack[l-1]
// 			ifBlockInfoStack = ifBlockInfoStack[:l-1]
// 			if info.offsetAfterExit > stackOffset {
// 				stackOffset = info.offsetAfterExit
// 			}
// 		}
// 	}

// 	for _, info := range blocks {
// 		for s := range info.static {
// 			fmt.Fprint(out, s)
// 		}
// 	}
// }

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
	blocks := make(map[*interface{}]*blockInfo)
	var labelGen labelIdGen
	var parser parsing.Parser
	var mainProc *interface{}
	ifCount := 0
	startGenForBlock := func(node *interface{}, label string, ownerType int) {
		info := blockInfo{
			code:      make(chan []interface{}),
			feed:      make(chan *interface{}),
			ownerType: ownerType,
			label:     label,
		}
		blocks[node] = &info
		go genForBlock(&labelGen, &info)
	}
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			continue
		}
		isComplete, node, parent, err := parser.FeedLine(line)
		if err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
		}

		exprNode, isExpr := (*node).(parsing.ExprNode)
		if !isComplete && isExpr && exprNode.Op == parsing.ConstDeclare {
			_, isProc := exprNode.Right.(parsing.ProcNode)
			if isProc {
				procName := string(exprNode.Left.(parsing.IdName))
				label := "proc_" + procName
				startGenForBlock(node, label, Proc)
				if procName == "main" {
					mainProc = node
				}
			}
			continue
		}

		_, isIf := (*node).(parsing.IfNode)
		_, isElse := (*node).(parsing.ElseNode)
		if !isComplete && (isIf || isElse) && parent != nil {
			label := fmt.Sprintf("if_%d", ifCount)
			startGenForBlock(node, label, If)
			ifCount++
			parentInfo := blocks[parent]
			parentInfo.feed <- node
			continue
		}

		if isComplete && parent != nil {
			info := blocks[parent]
			info.feed <- node
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

	for _, info := range blocks {
		close(info.feed)
	}

	ir := collectIr(mainProc, blocks)
	// fmt.Printf("%#v\n", ir)
	backend(out, &labelGen, ir)

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
