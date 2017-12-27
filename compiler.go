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

type procWorkOrder struct {
	out   chan optBlock
	in    []*interface{}
	label string
}

type procGen struct {
	opts       []interface{}
	nextVarNum int
	rootScope  *scope
}

type scope struct {
	gen         *procGen
	parentScope *scope
	varTable    map[parsing.IdName]int
}

type optBlock struct {
	frameSize int
	opts      []interface{}
}

func (s *scope) inherit() *scope {
	sub := scope{
		gen:         s.gen,
		parentScope: s,
		varTable:    make(map[parsing.IdName]int)}
	// #speed
	return &sub
}

func (s *scope) resolve(name parsing.IdName) (int, bool) {
	cur := s
	for cur != nil {
		varNum, found := cur.varTable[name]
		if found {
			return varNum, found
		} else {
			cur = cur.parentScope
		}
	}
	return 0, false
}

func (p *procGen) addOpt(opts ...interface{}) {
	p.opts = append(p.opts, opts...)
}

func (s *scope) newVar() int {
	current := s.gen.nextVarNum
	s.gen.nextVarNum++
	return current
}

func (s *scope) newNamedVar(name parsing.IdName) int {
	varNum := s.newVar()
	s.varTable[name] = varNum
	return varNum
}

func varTemp(varNum int) string {
	return fmt.Sprintf("$var%d", varNum)
}

func genForProc(labelGen *labelIdGen, order *procWorkOrder) {
	var gen procGen
	gen.rootScope = &scope{
		gen:      &gen,
		varTable: make(map[parsing.IdName]int), parentScope: nil}

	gen.addOpt(ir.StartProc{order.label})
	ret := genForProcSubSection(labelGen, order, gen.rootScope, 0)
	gen.addOpt(ir.EndProc{})
	if ret != len(order.in) {
		panic("gen didn't process whole proc")
	}
	// TODO: framesize here
	order.out <- optBlock{gen.nextVarNum * 8, gen.opts}
	close(order.out)
}

// return index to the first unprocessed node
func genForProcSubSection(labelGen *labelIdGen, order *procWorkOrder, scope *scope, start int) int {
	gen := scope.gen
	i := start
	sawIf := false
	for i < len(order.in) {
		nodePtr := order.in[i]
		i++
		sawIfLastTime := sawIf
		sawIf = false
		switch node := (*nodePtr).(type) {
		case parsing.ExprNode:
			switch node.Op {
			case parsing.Declare:
				varNum := scope.newNamedVar(node.Left.(parsing.IdName))
				err := genSimpleValuedExpression(scope, varNum, node.Right)
				if err != nil {
					panic(err)
				}
			case parsing.Assign:
				leftVarNum, varFound := scope.resolve(node.Left.(parsing.IdName))
				if !varFound {
					panic("bug in user program! assign to undefined var")
				}
				err := genSimpleValuedExpression(scope, leftVarNum, node.Right)
				if err != nil {
					panic(err)
				}
			default:
				//TODO issue warning here
			}
		case parsing.IfNode:
			sawIf = true
			condVar := scope.newVar()
			genSimpleValuedExpression(scope, condVar, node.Condition)
			labelForIf := labelGen.genLabel("if_%d")
			gen.addOpt(ir.JumpIfFalse{condVar, labelForIf})
			i = genForProcSubSection(labelGen, order, scope.inherit(), i)
			gen.addOpt(ir.Label{labelForIf})
		case parsing.ElseNode:
			if !sawIfLastTime {
				panic("Bare else. Should've been caught by the parser")
			}
			elseLabel := labelGen.genLabel("else_%d")
			ifLabel := gen.opts[len(gen.opts)-1]
			gen.opts[len(gen.opts)-1] = ir.Jump{elseLabel}
			gen.addOpt(ifLabel)
			i = genForProcSubSection(labelGen, order, scope.inherit(), i)
			gen.addOpt(ir.Label{elseLabel})
		case parsing.ProcCall:
			var argVars []int
			for _, argNode := range node.Args {
				argEval := scope.newVar()
				err := genSimpleValuedExpression(scope, argEval, argNode)
				if err != nil {
					panic(err)
				}
				argVars = append(argVars, argEval)
			}
			gen.addOpt(ir.Call{string(node.Callee), argVars})
		case parsing.BlockEnd:
			return i
		}
	}
	return i
}

func genSimpleValuedExpression(scope *scope, dest int, node interface{}) error {
	gen := scope.gen
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
		vn, found := scope.resolve(n)
		if !found {
			return errors.New("undefined var")
		}
		gen.addOpt(ir.Assign{dest, vn})
	case parsing.ExprNode:
		switch n.Op {
		case parsing.Star, parsing.Minus, parsing.Plus, parsing.Divide:
			leftDest := scope.newVar()
			err := genSimpleValuedExpression(scope, leftDest, n.Left)
			if err != nil {
				return err
			}
			rightDest := scope.newVar()
			err = genSimpleValuedExpression(scope, rightDest, n.Right)
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
		acutalVar := varNum
		//TODO: !! of course not every var is size 8...
		// +8 for the return addresss
		return fmt.Sprintf("qword [rbp-%d]", acutalVar*8+8)
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
			addLine(fmt.Sprintf("\tsub rsp, %d\n", block.frameSize))
		case ir.EndProc:
			addLine("\tmov rsp, rbp\n")
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
	blocks := make(map[*interface{}]*procWorkOrder)
	var labelGen labelIdGen
	var parser parsing.Parser
	var mainProc *interface{}
	var currentProc *interface{}
	var nodesForProc []*interface{}

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

		_, isEnd := (*node).(parsing.BlockEnd)
		if isEnd && parent == currentProc {
			procName := string((*currentProc).(parsing.ExprNode).Left.(parsing.IdName))
			label := "proc_" + procName
			order := procWorkOrder{
				out:   make(chan optBlock),
				in:    nodesForProc,
				label: label,
			}
			blocks[currentProc] = &order
			go genForProc(&labelGen, &order)
			nodesForProc = nil
			continue
		}

		for i := numNewEntries; i > 0; i-- {
			nodesForProc = append(nodesForProc, parser.OutBuffer[len(parser.OutBuffer)-i].Node)
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

	ir := <-blocks[mainProc].out
	// fmt.Printf("%#v\n", ir)
	backend(out, &labelGen, []optBlock{ir})

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
