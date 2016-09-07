package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"github.com/XrXr/alang/parser"
	"io"
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

func (g *labelIdGen) genLabel(template string) (ret string) {
	g.Lock()
	ret = fmt.Sprintf(template, g.availableId)
	g.availableId++
	g.Unlock()
	return
}

// An output from genForBlock can either be a solid line or a transclusion of
// another block. There no unions in golang, so this structure is used
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

func genForBlock(labelGen *labelIdGen, info *blockInfo) {
	codeBuf := make([]codeGenCommand, 0)
	var staticDataBuf bytes.Buffer
	idToTemplateName := make(map[parser.IdName]string)
	varNum := 0
	var lastNodePtr *interface{}
	for nodePtr := range info.feed {
		switch node := (*nodePtr).(type) {
		case parser.ExprNode:
			if node.Op == parser.Declare {
				literal, rightIsLiteral := node.Right.(parser.Literal)
				varTemplateName := fmt.Sprintf("$var%d", varNum)
				varNum++
				idToTemplateName[node.Left.(parser.IdName)] = varTemplateName
				if rightIsLiteral {
					var cmd codeGenCommand
					switch literal.Type {
					case parser.Number:
						cmd.line = fmt.Sprintf("\tmov %s, %s\n", varTemplateName, literal.Value)
					case parser.Boolean:
						cmd.line = fmt.Sprintf("\tmov %s, %d\n", varTemplateName, boolStrToInt(literal.Value))
					}
					codeBuf = append(codeBuf, cmd)
				}
			}
		case parser.IfNode:
			// TODO: factor out the code for generating a single expression
			var cmd codeGenCommand
			switch cond := node.Condition.(type) {
			case parser.Literal:
				if cond.Type == parser.Boolean {
					cmd.line = fmt.Sprintf("\tmov rax, %d\n", boolStrToInt(cond.Value))
				}
			case parser.IdName:
				templateName, found := idToTemplateName[cond]
				if found {
					cmd.line = fmt.Sprintf("\tmov rax, %s\n", templateName)
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
		case parser.ElseNode:
			if lastNodePtr != nil {
				_, lastIsIf := (*lastNodePtr).(parser.IfNode)
				if lastIsIf {
					elseLabel := labelGen.genLabel("else_%d")
					ifLabelCmd := codeBuf[len(codeBuf)-1]
					codeBuf[len(codeBuf)-1] =
						codeGenCommand{line: fmt.Sprintf("\tjmp %s\n", elseLabel)}
					codeBuf = append(codeBuf,
						ifLabelCmd,
						codeGenCommand{isTransclude: true, transclude: nodePtr},
						codeGenCommand{line: fmt.Sprintf("%s:\n", elseLabel)},
					)
				}
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

						labelName := labelGen.genLabel("label%d")
						staticDataBuf.WriteString(fmt.Sprintf("%s:\n", labelName))
						staticDataBuf.WriteString(fmt.Sprintf("\tdq\t%d\n", byteCount))
						staticDataBuf.ReadFrom(&stringInsBuf)
						staticDataBuf.WriteRune('\n')
						argLocations = append(argLocations, labelName)
					}
				}
			}
			for i, location := range argLocations {
				codeBuf = append(codeBuf,
					codeGenCommand{line: fmt.Sprintf("\tmov %s, %s\n", paramOrder[i], location)})
			}
			codeBuf = append(codeBuf,
				codeGenCommand{line: fmt.Sprintf("\tcall %s\n", node.Callee)})
		}
		lastNodePtr = nodePtr
	}

	for _, cmd := range codeBuf {
		info.code <- cmd
	}
	close(info.code)
	sendLinesToChan(&staticDataBuf, &info.static)
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
		// map "$var{number}" in this block to locations like [rsp-8]
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
				s := cmd.line
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
		return
	}
	defer source.Close()

	scanner := bufio.NewScanner(source)
	blocks := make(map[*interface{}]blockInfo)
	var labelGen labelIdGen
	var p parser.Parser
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
		go genForBlock(&labelGen, &info)
	}
	for scanner.Scan() {
		isComplete, node, parent, err := p.FeedLine(scanner.Text())
		if err != nil {
			fmt.Println(err.Error())
			return
		}
		exprNode, isExpr := (*node).(parser.ExprNode)
		if !isComplete && isExpr && exprNode.Op == parser.ConstDeclare {
			_, isProc := exprNode.Right.(parser.ProcNode)
			if isProc {
				procName := string(exprNode.Left.(parser.IdName))
				label := "proc_" + procName
				startGenForBlock(node, label, Proc)
				if procName == "main" {
					mainProc = node
				}
			}
			continue
		}

		_, isIf := (*node).(parser.IfNode)
		_, isElse := (*node).(parser.ElseNode)
		if !isComplete && (isIf || isElse) && parent != nil {
			label := fmt.Sprintf("if_%d", ifCount)
			startGenForBlock(node, label, If)
			ifCount++
			parentInfo := blocks[parent]
			parentInfo.feed <- node
			continue
		}

		closeLineFeed := func(key *interface{}) {
			info, found := blocks[key]
			if found {
				close(info.feed)
			} else {
				panic("failed to find block to close")
			}
		}

		_, isBlockEnd := (*node).(parser.BlockEnd)
		if isBlockEnd && parent != nil {
			closeLineFeed(parent)
		} else if isComplete && parent == nil {
			closeLineFeed(node)
		} else if isComplete && parent != nil {
			info := blocks[parent]
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

	fmt.Fprintln(out, "_start:")
	fmt.Fprintln(out, "\tcall proc_main")
	fmt.Fprintln(out, "\tmov eax, 60")
	fmt.Fprintln(out, "\txor rdi, rdi")
	fmt.Fprintln(out, "\tsyscall")

	fmt.Fprintln(out, "puts:")
	fmt.Fprintln(out, "\tmov rdx, [rax]")
	fmt.Fprintln(out, "\tmov rsi, rax")
	fmt.Fprintln(out, "\tadd rsi, 8")
	fmt.Fprintln(out, "\tmov rax, 1")
	fmt.Fprintln(out, "\tmov rdi, 1")
	fmt.Fprintln(out, "\tsyscall")
	fmt.Fprintln(out, "\tret")

	collectAsm(mainProc, blocks, out)

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
