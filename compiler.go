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
	code      chan codeGenCommand
	static    chan string
	feed      chan *interface{}
	ownerType int
	label     string
}

type frontendGen struct {
	opts       []interface{}
	nextVarNum int
	varTable   map[parsing.IdName]int
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

func genForBlock(gen *frontendGen, labelGen *labelIdGen, info *blockInfo) {
	codeBuf := make([]codeGenCommand, 0)
	var staticDataBuf bytes.Buffer
	// var lastNodePtr *interface{}
	for nodePtr := range info.feed {
		switch node := (*nodePtr).(type) {
		case parsing.ExprNode:
			switch node.Op {
			case parsing.Declare:
				before := len(gen.opts)
				varNum := gen.newVar()
				gen.varTable[node.Left.(parsing.IdName)] = varNum
				err := genSimpleValuedExpression(gen, varNum, node.Right)
				if err != nil {
					panic(err)
				}
				backend(&codeBuf, gen.opts[before:])
			case parsing.Assign:
				before := len(gen.opts)
				leftVarNum, varFound := gen.varTable[node.Left.(parsing.IdName)]
				if !varFound {
					panic("bug in user program! assign to undefined var")
				}
				err := genSimpleValuedExpression(gen, leftVarNum, node.Right)
				if err != nil {
					panic(err)
				}
				backend(&codeBuf, gen.opts[before:])
			}
		case parsing.IfNode:
			// TODO: factor out the code for generating a single expression
			var cmd codeGenCommand
			switch cond := node.Condition.(type) {
			case parsing.Literal:
				if cond.Type == parsing.Boolean {
					cmd.line = fmt.Sprintf("\tmov rax, %d\n", boolStrToInt(cond.Value))
				}
			case parsing.IdName:
				varNum, found := gen.varTable[cond]
				if found {
					cmd.line = fmt.Sprintf("\tmov rax, %s\n", varTemp(varNum))
				} else {
					//TODO: compile error
				}
			}
			ifLabel := labelGen.genLabel("if_%d")
			codeBuf = append(codeBuf,
				cmd,
				codeGenCommand{line: "\tcmp rax, 0\n"},
				codeGenCommand{line: fmt.Sprintf("\tjz %s\n", ifLabel)},
				codeGenCommand{isTransclude: true, transclude: nodePtr},
				codeGenCommand{line: fmt.Sprintf("%s:\n", ifLabel)},
			)
		case parsing.ElseNode:
			elseLabel := labelGen.genLabel("else_%d")
			ifLabelCmd := codeBuf[len(codeBuf)-1]
			codeBuf[len(codeBuf)-1] =
				codeGenCommand{line: fmt.Sprintf("\tjmp %s\n", elseLabel)}
			codeBuf = append(codeBuf,
				ifLabelCmd,
				codeGenCommand{isTransclude: true, transclude: nodePtr},
				codeGenCommand{line: fmt.Sprintf("%s:\n", elseLabel)},
			)
		case parsing.ProcCall:
			argLocations := make([]string, 0)
			for _, arg := range node.Args {
				switch a := arg.(type) {
				case parsing.Literal:
					if a.Type == parsing.String {
						var stringInsBuf bytes.Buffer
						stringInsBuf.WriteString("\tdb\t")
						byteCount := 0
						i := 0
						needToStartQuote := true
						for ; i < len(a.Value); i++ {
							if needToStartQuote {
								stringInsBuf.WriteRune('"')
								needToStartQuote = false
							}
							if a.Value[i] == '\\' && a.Value[i+1] == 'n' {
								stringInsBuf.WriteString(`",10,`)
								needToStartQuote = true
								i++
							} else {
								stringInsBuf.WriteString(string(a.Value[i]))
							}
							byteCount++
						}
						// end the string
						if !needToStartQuote {
							stringInsBuf.WriteRune('"')
						}

						labelName := labelGen.genLabel("label%d")
						staticDataBuf.WriteString(fmt.Sprintf("%s:\n", labelName))
						staticDataBuf.WriteString(fmt.Sprintf("\tdq\t%d\n", byteCount))
						staticDataBuf.ReadFrom(&stringInsBuf)
						staticDataBuf.WriteRune('\n')
						argLocations = append(argLocations, labelName)
					}
				case parsing.IdName:
					argLocations = append(argLocations, varTemp(gen.varTable[a]))
				}
			}
			for i, location := range argLocations {
				codeBuf = append(codeBuf,
					codeGenCommand{line: fmt.Sprintf("\tmov %s, %s\n", paramOrder[i], location)})
			}
			codeBuf = append(codeBuf,
				codeGenCommand{line: fmt.Sprintf("\tcall %s\n", node.Callee)})
		}
		// lastNodePtr = nodePtr
	}

	for _, cmd := range codeBuf {
		info.code <- cmd
	}
	close(info.code)
	sendLinesToChan(&staticDataBuf, &info.static)
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
		}
		gen.addOpt(ir.AssignImm{dest, value})
	case parsing.IdName:
		gen.addOpt(ir.Assign{dest, gen.varTable[n]})
	case parsing.ExprNode:
		switch n.Op {
		case parsing.Star, parsing.Minus, parsing.Plus, parsing.Divide:
			err := genSimpleValuedExpression(gen, dest, n.Left)
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
				gen.addOpt(ir.Mult{dest, rightDest})
			case parsing.Divide:
				gen.addOpt(ir.Div{dest, rightDest})
			case parsing.Plus:
				gen.addOpt(ir.Add{dest, rightDest})
			case parsing.Minus:
				gen.addOpt(ir.Sub{dest, rightDest})
			}
		default:
			return errors.New(fmt.Sprintf("Unsupported value expression type %v", n.Op))
		}
	}
	return nil
}

func backend(cmdBuf *[]codeGenCommand, opts []interface{}) {
	var cmd codeGenCommand
	for _, opt := range opts {
		switch opt := opt.(type) {
		case ir.Assign:
			cmd.line = fmt.Sprintf("\tmov rax, %s\n", varTemp(opt.Right))
			*cmdBuf = append(*cmdBuf, cmd)
			cmd.line = fmt.Sprintf("\tmov %s, rax\n", varTemp(opt.Left))
			*cmdBuf = append(*cmdBuf, cmd)
		case ir.AssignImm:
			// TODO types, assuming int right now
			cmd.line = fmt.Sprintf("\tmov %s, %v\n", varTemp(opt.Var), opt.Val)
			*cmdBuf = append(*cmdBuf, cmd)
		case ir.Add:
			cmd.line = fmt.Sprintf("\tmov rax, %s\n", varTemp(opt.Right))
			*cmdBuf = append(*cmdBuf, cmd)
			cmd.line = fmt.Sprintf("\tadd %s, rax\n", varTemp(opt.Left))
			*cmdBuf = append(*cmdBuf, cmd)
		case ir.Sub:
			cmd.line = fmt.Sprintf("\tmov rax, %s\n", varTemp(opt.Right))
			*cmdBuf = append(*cmdBuf, cmd)
			cmd.line = fmt.Sprintf("\tsub %s, rax\n", varTemp(opt.Left))
			*cmdBuf = append(*cmdBuf, cmd)
		case ir.Mult:
			cmd.line = fmt.Sprintf("\tmov r8, %s\n", varTemp(opt.Left))
			*cmdBuf = append(*cmdBuf, cmd)
			cmd.line = fmt.Sprintf("\tmov r9, %s\n", varTemp(opt.Right))
			*cmdBuf = append(*cmdBuf, cmd)
			cmd.line = "\timul r8, r9\n"
			*cmdBuf = append(*cmdBuf, cmd)
			cmd.line = fmt.Sprintf("\tmov %s, r8\n", varTemp(opt.Left))
			*cmdBuf = append(*cmdBuf, cmd)
		case ir.Div:
			cmd.line = "\txor rdx, rdx\n"
			*cmdBuf = append(*cmdBuf, cmd)
			cmd.line = fmt.Sprintf("\tmov rax, %s\n", varTemp(opt.Left))
			*cmdBuf = append(*cmdBuf, cmd)
			cmd.line = fmt.Sprintf("\tmov r8, %s\n", varTemp(opt.Right))
			*cmdBuf = append(*cmdBuf, cmd)
			cmd.line = "\tidiv r8\n"
			*cmdBuf = append(*cmdBuf, cmd)
			cmd.line = fmt.Sprintf("\tmov %s, rax\n", varTemp(opt.Left))
			*cmdBuf = append(*cmdBuf, cmd)
		}
	}
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

// collect from blocks and fill in variable offset
func collectAsm(mainProc *interface{}, blocks map[*interface{}]blockInfo, out io.Writer) {
	stackOffset := 0
	type ifBlockInfo struct {
		offsetBeforeEntry int
		offsetAfterExit   int
	}
	ifBlockInfoStack := make([]ifBlockInfo, 0)
	var ifEntryOffset int
	type blockConsumptionInfo struct {
		block         *interface{}
		prologPrinted bool
	}
	blockConsumptionStack := []blockConsumptionInfo{{mainProc, false}}

consumeLoop:
	for len(blockConsumptionStack) != 0 {
		l := len(blockConsumptionStack)
		cur := blockConsumptionStack[l-1]
		blockConsumptionStack = blockConsumptionStack[:l-1]

		info := blocks[cur.block]
		if !cur.prologPrinted {
			switch info.ownerType {
			case Proc:
				fmt.Fprintf(out, "%s:\n", info.label)
				fmt.Fprintln(out, "\tpush rbp")
				fmt.Fprintln(out, "\tmov rbp, rsp")
			case If:
				ifEntryOffset = stackOffset
			case Else:
				info := ifBlockInfoStack[len(ifBlockInfoStack)-1]
				stackOffset = info.offsetBeforeEntry
			}
		}
		varTable := make(map[string]string)
		for cmd := range info.code {
			if cmd.isTransclude {
				if cur.block == cmd.transclude {
					panic("a block tried to transclude itself")
				}
				blockConsumptionStack = append(blockConsumptionStack,
					blockConsumptionInfo{cur.block, true},
					blockConsumptionInfo{cmd.transclude, false},
				)
				continue consumeLoop
			} else {
				// map "$var{number}" in this block to locations like [rsp-8]
				s := cmd.line
				match := varNameRegex.FindStringIndex(s)
				if match != nil {
					varTpl := s[match[0]:match[1]]
					_, found := varTable[varTpl]
					if !found {
						loc := fmt.Sprintf("qword [rbp-%d]", stackOffset)
						varTable[varTpl] = loc
						stackOffset += 8
					}

					filled := varNameRegex.ReplaceAllFunc([]byte(s), func(match []byte) []byte {
						loc, ok := varTable[string(match[:])]
						if !ok {
							panic("referenced an undefined variable internally")
						}
						return []byte(loc)
					})
					s = string(filled[:])
				}
				fmt.Fprint(out, s)
			}
		}
		// epilogue
		switch info.ownerType {
		case Proc:
			fmt.Fprintln(out, "\tpop rbp")
			fmt.Fprintln(out, "\tret")
		case If:
			ifBlockInfoStack = append(ifBlockInfoStack, ifBlockInfo{
				offsetBeforeEntry: ifEntryOffset,
				offsetAfterExit:   stackOffset,
			})
		case Else:
			l := len(ifBlockInfoStack)
			info := ifBlockInfoStack[l-1]
			ifBlockInfoStack = ifBlockInfoStack[:l-1]
			if info.offsetAfterExit > stackOffset {
				stackOffset = info.offsetAfterExit
			}
		}
	}

	for _, info := range blocks {
		for s := range info.static {
			fmt.Fprint(out, s)
		}
	}
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
		fmt.Printf("Could not start nasm\n")
		os.Exit(1)
	}
	defer source.Close()

	scanner := bufio.NewScanner(source)
	blocks := make(map[*interface{}]blockInfo)
	var labelGen labelIdGen
	var frontend frontendGen
	frontend.varTable = make(map[parsing.IdName]int)
	var parser parsing.Parser
	var mainProc *interface{}
	ifCount := 0
	startGenForBlock := func(node *interface{}, label string, ownerType int) {
		info := blockInfo{
			code:      make(chan codeGenCommand),
			static:    make(chan string),
			feed:      make(chan *interface{}),
			ownerType: ownerType,
			label:     label,
		}
		blocks[node] = info
		go genForBlock(&frontend, &labelGen, &info)
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
	collectAsm(mainProc, blocks, out)

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
