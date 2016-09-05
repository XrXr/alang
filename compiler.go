package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"github.com/XrXr/alang/parser"
	"log"
	"os"
	"os/exec"
	"regexp"
	"sync"
)

type labelIdGen struct {
	sync.Mutex
	availableId int
}

var paramOrder = []string{"rax", "rbx"}

func genForBlock(labelGen *labelIdGen, lineIn chan interface{}, codeOut chan string, staticDataOut chan string) {
	var codeBuf bytes.Buffer
	var staticDataBuf bytes.Buffer
	varNum := 0
	for line := range lineIn {
		switch node := line.(type) {
		case parser.ExprNode:
			if node.Op == parser.Declare {
				literal, rightIsLiteral := node.Right.(parser.Literal)
				if rightIsLiteral {
					switch literal.Type {
					case parser.Number:
						codeBuf.WriteString(fmt.Sprintf("\tmov $var%d, %s\n",
							varNum, literal.Value))
					case parser.Boolean:
						value := 1
						if literal.Value == "false" {
							value = 0
						}
						codeBuf.WriteString(fmt.Sprintf("\tmov $var%d, %d\n",
							varNum, value))
					}
				}
				varNum++
			}
		case parser.ProcCall:
			argLocations := make([]string, 0)
			for _, arg := range node.Args {
				switch a := arg.(type) {
				case parser.Literal:
					if a.Type == parser.String {
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

						labelGen.Lock()
						labelName := fmt.Sprintf("label%d", labelGen.availableId)
						labelGen.availableId++
						labelGen.Unlock()

						staticDataBuf.WriteString(fmt.Sprintf("%s:\n", labelName))
						staticDataBuf.WriteString(fmt.Sprintf("\tdq\t%d\n", byteCount))
						staticDataBuf.ReadFrom(&stringInsBuf)
						staticDataBuf.WriteRune('\n')
						argLocations = append(argLocations, labelName)
					}
				}
			}
			for i, location := range argLocations {
				codeBuf.WriteString(fmt.Sprintf("\tmov %s, %s\n", paramOrder[i], location))
			}
			codeBuf.WriteString(fmt.Sprintf("\tcall %s\n", node.Callee))
		}
	}
	sendLinesToChan(&codeBuf, &codeOut)
	sendLinesToChan(&staticDataBuf, &staticDataOut)
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
		return
	}
	defer source.Close()
	const (
		None = iota
		Proc
		If
		Else
	)
	type blockInfo struct {
		code      chan string
		static    chan string
		feed      chan interface{}
		ownerType int
		owner     string
	}
	scanner := bufio.NewScanner(source)
	blocks := make(map[parser.IdName]blockInfo)
	var labelGen labelIdGen
	var p parser.Parser
	for scanner.Scan() {
		isComplete, node, parent, _ := p.FeedLine(scanner.Text())
		exprNode, good := node.(parser.ExprNode)
		if !isComplete && good && exprNode.Op == parser.ConstDeclare {
			_, isProc := exprNode.Right.(parser.ProcNode)
			if isProc {
				info := blockInfo{
					code:      make(chan string),
					static:    make(chan string),
					feed:      make(chan interface{}),
					ownerType: Proc,
				}
				blocks[exprNode.Left.(parser.IdName)] = info
				go genForBlock(&labelGen, info.feed, info.code, info.static)
			}
			continue
		}
		idName := func(i interface{}) parser.IdName {
			return i.(parser.ExprNode).Left.(parser.IdName)
		}
		// end of proc
		if isComplete && parent == nil {
			info, found := blocks[idName(node)]
			if found {
				close(info.feed)
			}
		} else if isComplete && parent != nil { // line in a proc
			info := blocks[idName(parent)]
			info.feed <- node
		}
	}
	out, err := os.Create("a.asm")
	if err != nil {
		return
	}
	defer out.Close()
	fmt.Fprintln(out, "\tglobal _start")
	fmt.Fprintln(out, "\tsection .text")
	stackOffset := 0
	varNameRegex := regexp.MustCompile(`\$var[0-9]+`)
	for blockName, info := range blocks {
		// TODO: need more information about the block's owner. Is it a block
		// for a proc? if? else?
		fmt.Fprintln(out, blockName+":")
		fmt.Fprintln(out, "\tpush rbp")
		fmt.Fprintln(out, "\tmov rbp, rsp")
		// map "$var{number}" in this block to locations like [rsp-8]
		varTable := make(map[string]string)
		for s := range info.code {
			match := varNameRegex.FindStringIndex(s)
			if match != nil {
				fmt.Fprint(out, s[:match[0]])

				varTpl := s[match[0]:match[1]]
				loc, found := varTable[varTpl]
				if found {
					fmt.Fprint(out, loc)
				} else {
					loc := fmt.Sprintf("qword [rbp-%d]", stackOffset)
					varTable[varTpl] = loc
					fmt.Fprint(out, loc)
					stackOffset += 8
				}

				fmt.Fprint(out, s[match[1]:])
			} else {
				fmt.Fprint(out, s)
			}
		}
		fmt.Fprintln(out, "\tpop rbp")
		fmt.Fprintln(out, "\tret")
		for s := range info.static {
			fmt.Fprint(out, s)
		}
	}

	fmt.Fprintln(out, "puts:")
	fmt.Fprintln(out, "\tmov rdx, [rax]")
	fmt.Fprintln(out, "\tmov rsi, rax")
	fmt.Fprintln(out, "\tadd rsi, 8")
	fmt.Fprintln(out, "\tmov rax, 1")
	fmt.Fprintln(out, "\tmov rdi, 1")
	fmt.Fprintln(out, "\tsyscall")
	fmt.Fprintln(out, "\tret")
	fmt.Fprintln(out, "_start:")
	fmt.Fprintln(out, "\tcall main")
	fmt.Fprintln(out, "\tmov eax, 60")
	fmt.Fprintln(out, "\txor rdi, rdi")
	fmt.Fprintln(out, "\tsyscall")

	cmd := exec.Command("nasm", "-felf64", "a.asm")
	err = cmd.Start()
	if err != nil {
		fmt.Printf("Could not start nasm\n")
		return
	}
	err = cmd.Wait()
	if err != nil {
		fmt.Printf("nasm call failed %v\n", err)
		return
	}
	cmd = exec.Command("ld", "-o", *outputPath, "a.o")
	err = cmd.Start()
	if err != nil {
		fmt.Printf("Could not start ld\n")
	}
	err = cmd.Wait()
	if err != nil {
		fmt.Printf("ld call failed\n")
	}
}
