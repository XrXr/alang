package main

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/XrXr/alang/parser"
	"os"
	"os/exec"
)

var paramOrder = []string{"rax", "rbx"}

func genForBlock(content chan interface{}, codeOut chan string, staticDataOut chan string) {
	var codeBuf bytes.Buffer
	var staticDataBuf bytes.Buffer
	for line := range content {
		switch node := line.(type) {
		case parser.ProcCall:
			argLocations := make([]string, 0)
			for _, arg := range node.Args {
				switch a := arg.(type) {
				case parser.Literal:
					if a.Type == parser.String {
						var stringInsBuf bytes.Buffer
						stringInsBuf.WriteString("\tdb\t")
						// TODO label gen
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
						staticDataBuf.WriteString("message:\n" +
							fmt.Sprintf("\tdq\t%d\n", byteCount))
						staticDataBuf.ReadFrom(&stringInsBuf)
						argLocations = append(argLocations, "message")
					}
				}
			}
			for i, location := range argLocations {
				codeBuf.WriteString(fmt.Sprintf("\tmov %s, %s\n", paramOrder[i], location))
			}
			codeBuf.WriteString(fmt.Sprintf("\tcall %s", node.Callee))
		}
	}
	codeOut <- codeBuf.String()
	close(codeOut)
	staticDataOut <- staticDataBuf.String()
	close(staticDataOut)
}

func main() {
	file, err := os.Open("hello.al")
	if err != nil {
		return
	}
	defer file.Close()
	type blockInfo struct {
		code   chan string
		static chan string
		feed   chan interface{}
	}
	scanner := bufio.NewScanner(file)
	blocks := make(map[parser.IdName]blockInfo)
	var p parser.Parser
	for scanner.Scan() {
		isComplete, node, parent, _ := p.FeedLine(scanner.Text())
		exprNode, good := node.(parser.ExprNode)
		if !isComplete && good && exprNode.Op == parser.ConstDeclare {
			_, isProc := exprNode.Right.(parser.ProcNode)
			if isProc {
				info := blockInfo{
					make(chan string),
					make(chan string),
					make(chan interface{})}
				blocks[exprNode.Left.(parser.IdName)] = info
				go genForBlock(info.feed, info.code, info.static)
			}
			continue
		}
		bid := func(i interface{}) parser.IdName {
			return i.(parser.ExprNode).Left.(parser.IdName)
		}
		if isComplete && parent == nil {
			info, found := blocks[bid(node)]
			if found {
				close(info.feed)
			}
		} else if isComplete && parent != nil {
			info := blocks[bid(parent)]
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
	for blockName, info := range blocks {
		fmt.Fprintln(out, blockName+":")
		for s := range info.code {
			fmt.Fprintln(out, s)
		}
		fmt.Fprintln(out, "\tret")
		for s := range info.static {
			fmt.Fprintln(out, s)
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
	cmd = exec.Command("ld", "a.o")
	err = cmd.Start()
	if err != nil {
		fmt.Printf("Could not start ld\n")
	}
	err = cmd.Wait()
	if err != nil {
		fmt.Printf("ld call failed\n")
	}
}
