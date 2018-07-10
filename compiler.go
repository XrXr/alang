package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"github.com/XrXr/alang/backend"
	"github.com/XrXr/alang/errors"
	"github.com/XrXr/alang/frontend"
	"github.com/XrXr/alang/library"
	"github.com/XrXr/alang/parsing"
	"github.com/XrXr/alang/typing"
	"io"
	"log"
	"os"
	"os/exec"
	"sort"
)

type embedGraphNode struct {
	visited  bool
	embedees []*typing.StructRecord
}

// resolve all the type of members in structs and build the global environment
func buildGlobalEnv(typer *typing.Typer, env *typing.EnvRecord, nodeToStruct map[*parsing.ASTNode]*typing.StructRecord, workOrders []*frontend.ProcWorkOrder) error {
	notDone := make(map[string][]*typing.TypeRecord)
	addUnresolved := func(unresolvedRecord *typing.TypeRecord) {
		unresolved := (*unresolvedRecord).(typing.Unresolved)
		name := typing.GrabUnresolvedName(unresolved)
		notDone[name] = append(notDone[name], unresolvedRecord)
	}
	embedGraphString := make(map[*typing.StructRecord][]string)
	for _, structRecord := range nodeToStruct {
		for _, field := range structRecord.Members {
			unresolved, isUnresolved := field.Type.(typing.Unresolved)
			if isUnresolved {
				addUnresolved(&field.Type)

				if unresolved.Decl.LevelOfIndirection == 0 && (unresolved.Decl.ArrayBase == nil || unresolved.Decl.ArrayBase.LevelOfIndirection == 0) {
					embedGraphString[structRecord] = append(embedGraphString[structRecord], typing.GrabUnresolvedName(unresolved))
				}
			}
		}
	}
	for _, order := range workOrders {
		argRecords := make([]typing.TypeRecord, len(order.ProcDecl.Args))
		returnType := typer.TypeRecordFromDecl(order.ProcDecl.Return)
		if _, returnIsUnresolved := returnType.(typing.Unresolved); returnIsUnresolved {
			addUnresolved(&returnType)
		}

		for i, argDecl := range order.ProcDecl.Args {
			record := typer.TypeRecordFromDecl(argDecl.Type)
			argRecords[i] = record
			if _, recordIsUnresolved := record.(typing.Unresolved); recordIsUnresolved {
				addUnresolved(&argRecords[i])
			}
		}
		env.Procs[order.Name] = typing.ProcRecord{
			&returnType,
			argRecords,
			order.ProcDecl.IsForeign,
		}
	}

	for node, structRecord := range nodeToStruct {
		structNode := (*node).(parsing.StructDeclare)
		name := structNode.Name.Name
		for _, typeRecordPtr := range notDone[name] {
			unresolved := (*typeRecordPtr).(typing.Unresolved)
			*typeRecordPtr = typing.BuildRecordAccordingToUnresolved(structRecord, unresolved)
		}
		delete(notDone, name)
		env.Types[structNode.Name.Name] = structRecord
	}
	if len(notDone) > 0 {
		for typeName := range notDone {
			return fmt.Errorf("%s does not name a type", typeName)
		}
	}
	embedGraph := make(map[*typing.StructRecord]embedGraphNode)
	for record, stringEmbedees := range embedGraphString {
		sort.Slice(stringEmbedees, func(i, j int) bool {
			return stringEmbedees[i] < stringEmbedees[j]
		})
		stringEmbedees = dedupSorted(stringEmbedees)
		embedees := make([]*typing.StructRecord, len(stringEmbedees))
		for i, name := range stringEmbedees {
			embedees[i] = env.Types[name].(*typing.StructRecord)
		}
		embedGraph[record] = embedGraphNode{embedees: embedees}
	}
	for record, names := range embedGraphString {
		fmt.Printf("%s: %v\n", string(record.Name), names)
	}
	resolveStructSize(nodeToStruct, embedGraph)
	return nil
}

func dedupSorted(slice []string) []string {
	pushDist := 0
	for i := 1; i < len(slice); i++ {
		if slice[i] == slice[i-1] {
			pushDist++
			continue
		}
		if pushDist > 0 {
			slice[i-pushDist] = slice[i]
		}
	}
	return slice[0 : len(slice)-pushDist]
}

func resolveStructSize(nodeToStruct map[*parsing.ASTNode]*typing.StructRecord, embedGraph map[*typing.StructRecord]embedGraphNode) {
	for _, structRecord := range nodeToStruct {
		_, embeds := embedGraph[structRecord]
		if !embeds {
			structRecord.ResolveSizeAndOffset()
		}
	}
	if len(embedGraph) == 0 {
		return
	}
	for {
		done := true
		for structRecord := range embedGraph {
			if !structRecord.SizeAndOffsetsResolved {
				done = false
				for structRecord, node := range embedGraph {
					node.visited = false
					embedGraph[structRecord] = node
				}
				resolveStructSizeVisit(structRecord, embedGraph)
				break
			}
		}
		if done {
			break
		}
	}
}

func resolveStructSizeVisit(structRecord *typing.StructRecord, embedGraph map[*typing.StructRecord]embedGraphNode) {
	node := embedGraph[structRecord]
	if node.visited {
		panic("Embed cycle is not allowed")
	}
	node.visited = true
	embedGraph[structRecord] = node
	for _, embedee := range node.embedees {
		resolveStructSizeVisit(embedee, embedGraph)
	}

	structRecord.ResolveSizeAndOffset()
}

func doCompile(sourceLines []string, libc bool, asmOut io.Writer) {
	var workOrders []*frontend.ProcWorkOrder
	var labelGen frontend.LabelIdGen
	parser := parsing.NewParser()
	typer := typing.NewTyper()
	var currentProc *parsing.ASTNode
	var nodesForProc []*parsing.ASTNode
	env := typing.NewEnvRecord(typer)
	structs := make(map[*parsing.ASTNode]*typing.StructRecord)
	if libc {
		library.AddLibcExtrasToEnv(env, typer)
	}

	parseFailed := false
	for lineNumber, line := range sourceLines {
		if len(line) == 0 {
			continue
		}
		numNewEntries, err := parser.FeedLine(line, lineNumber)
		if err != nil {
			parseFailed = true
			if userError, isUserError := err.(*errors.UserError); isUserError {
				displayError(sourceLines, userError)
			}
		}
		if numNewEntries == 0 || parseFailed {
			continue
		}

		last := len(parser.OutBuffer) - 1
		isComplete := parser.OutBuffer[last].IsComplete
		node := parser.OutBuffer[last].Node
		parent := parser.OutBuffer[last].Parent
		var isForeignProc bool

		exprNode, isExpr := (*node).(parsing.ExprNode)
		if !isComplete && isExpr && exprNode.Op == parsing.ConstDeclare {
			procDecl, isProc := exprNode.Right.(parsing.ProcDecl)
			if isProc {
				currentProc = node
				if procDecl.IsForeign {
					isForeignProc = true
				} else {
					continue
				}
			} else {
				continue
			}
		}

		if _, isEnd := (*node).(parsing.BlockEnd); isForeignProc || (isEnd && parent == currentProc) {
			procDeclare := (*currentProc).(parsing.ExprNode)
			procDecl := procDeclare.Right.(parsing.ProcDecl)
			procName := procDeclare.Left.(parsing.IdName).Name
			order := frontend.ProcWorkOrder{
				Out:      make(chan frontend.OptBlock),
				In:       nodesForProc,
				Name:     procName,
				ProcDecl: procDecl,
			}
			workOrders = append(workOrders, &order)
			go frontend.GenForProc(&labelGen, &order)
			nodesForProc = nil
			currentProc = nil
			continue
		}

		if structDeclare, isStructDeclare := (*node).(parsing.StructDeclare); isStructDeclare {
			newStruct := typing.StructRecord{
				Name:    string(structDeclare.Name.Name),
				Members: make(map[string]*typing.StructField),
			}
			structs[node] = &newStruct
		}

		if typeDeclare, isDecl := (*node).(parsing.Declaration); isDecl {
			parentStruct, found := structs[parent]
			if found {
				newField := &typing.StructField{
					Type: typer.TypeRecordFromDecl(typeDeclare.Type),
				}
				parentStruct.MemberOrder = append(parentStruct.MemberOrder, newField)
				parentStruct.Members[typeDeclare.Name.Name] = newField
			}
		}
		if currentProc != nil {
			for i := numNewEntries; i > 0; i-- {
				nodesForProc = append(nodesForProc, parser.OutBuffer[len(parser.OutBuffer)-i].Node)
			}
		}
	}
	if parseFailed {
		os.Exit(1)
	}

	if libc {
		library.WriteLibcPrologue(asmOut)
	} else {
		library.WriteAssemblyPrologue(asmOut)
	}

	err := buildGlobalEnv(typer, env, structs, workOrders)
	if err != nil {
		panic(err)
	}
	// fmt.Printf("%#v\n", env.Types)

	var staticData []*bytes.Buffer
	for _, workOrder := range workOrders {
		ir := <-workOrder.Out
		// frontend.DumpIr(ir)
		frontend.Prune(&ir)
		frontend.DumpIr(ir)
		// parsing.Dump(env)
		procRecord := env.Procs[workOrder.Name]
		typeTable, err := typer.InferAndCheck(env, &ir, procRecord)
		if err != nil {
			panic(err)
		}
		for i, e := range typeTable {
			if e == nil {
				println(i)
				panic("Bug in typer -- not all vars have types!")
			}
		}
		if workOrder.ProcDecl.IsForeign {
			fmt.Fprintf(asmOut, "extern %s\n", workOrder.Name)
			continue
		}
		static := backend.X86ForBlock(asmOut, ir, typeTable, env, typer, procRecord)
		staticData = append(staticData, static)
	}

	io.WriteString(asmOut, "; ---user code end---\n")
	if libc {
		library.WriteLibcExtras(asmOut)
	}
	library.WriteBuiltins(asmOut)
	library.WriteDecimalTable(asmOut)

	io.WriteString(asmOut, "; ---static data segment begin---\n")
	io.WriteString(asmOut, "section .data\n")
	for _, static := range staticData {
		static.WriteTo(asmOut)
	}
}

func displayError(sourceLines []string, err *errors.UserError) {
	fmt.Fprintln(os.Stderr, err.Error())
	line := sourceLines[err.Line]
	lineLength := len(line)
	if line[lineLength-1] == '\n' {
		line = line[:lineLength]
	}
	fmt.Fprintln(os.Stderr, line)

	for i := 0; i < lineLength; i++ {
		if i < err.StartColumn {
			charToPrint := " "
			if line[i] == '\t' {
				charToPrint = "\t"
			}
			fmt.Fprint(os.Stderr, charToPrint)
		} else if i <= err.EndColumn {
			fmt.Fprint(os.Stderr, "^")
		}
	}
	fmt.Fprintln(os.Stderr)
}

func compile(sourceLines []string, libc bool, asmOut io.Writer) {
	defer func() {
		err := recover()
		if err != nil {
			switch err := err.(type) {
			case *errors.UserError:
				displayError(sourceLines, err)
			default:
				panic(err)
			}
		}
	}()
	doCompile(sourceLines, libc, asmOut)
}

func main() {
	outputPath := flag.String("o", "a.out", "path to the binary")
	stopAfterAssembly := flag.Bool("c", false, "generate object file only")
	libc := flag.Bool("libc", false, "generate main instead of _start for ues with libc")
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

	asmOut, err := os.Create("a.asm")
	if err != nil {
		fmt.Printf("Could not create temporary asm file\n")
		os.Exit(1)
	}
	defer asmOut.Close()

	scanner := bufio.NewScanner(source)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	compile(lines, *libc, asmOut)
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
	if *stopAfterAssembly {
		return
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
