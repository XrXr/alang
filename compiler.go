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
	"regexp"
)

const (
	Proc = iota + 1
	If
	Else
)

func backendForOptBlock(out io.Writer, staticDataBuf *bytes.Buffer, labelGen *frontend.LabelIdGen, block frontend.OptBlock) {
	addLine := func(line string) {
		io.WriteString(out, line)
	}
	varToStack := func(varNum int) string {
		acutalVar := varNum
		//TODO: !! of course not every var is size 8...
		// +8 for the return addresss
		return fmt.Sprintf("qword [rbp-%d]", acutalVar*8+8)
	}
	for _, opt := range block.Opts {
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
			addLine(fmt.Sprintf("\tsub rsp, %d\n", block.FrameSize))
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

func backend(out io.Writer, labelGen *frontend.LabelIdGen, opts []frontend.OptBlock) {
	var staticDataBuf bytes.Buffer
	for _, block := range opts {
		backendForOptBlock(out, &staticDataBuf, labelGen, block)
	}
	io.WriteString(out, "; ---static data segment start---\n")
	staticDataBuf.WriteTo(out)
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
	blocks := make(map[*interface{}]*frontend.ProcWorkOrder)
	var labelGen frontend.LabelIdGen
	var parser parsing.Parser
	var mainProc *interface{}
	var currentProc *interface{}
	var nodesForProc []*interface{}
	var globals typing.EnvRecord

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

	ir := <-blocks[mainProc].Out
	typer := typing.NewTyper()
	err = typer.InferAndCheck(&globals, &ir)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%#v\n", ir)
	backend(out, &labelGen, []frontend.OptBlock{ir})

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
