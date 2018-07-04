package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"github.com/XrXr/alang/backend"
	"github.com/XrXr/alang/frontend"
	"github.com/XrXr/alang/parsing"
	"github.com/XrXr/alang/typing"
	"io"
	"log"
	"os"
	"os/exec"
)

type embedGraphNode struct {
	visited  bool
	embedees []*typing.StructRecord
}

// resolve all the type of members in structs and build the global environment
func buildGlobalEnv(typer *typing.Typer, env *typing.EnvRecord, nodeToStruct map[*interface{}]*typing.StructRecord, workOrders []*frontend.ProcWorkOrder) error {
	notDone := make(map[parsing.IdName][]*typing.TypeRecord)
	addUnresolved := func(unresolvedRecord *typing.TypeRecord) {
		unresolved := (*unresolvedRecord).(typing.Unresolved)
		name := typing.GrabUnresolvedName(unresolved)
		notDone[name] = append(notDone[name], unresolvedRecord)
	}
	embedGraphString := make(map[*typing.StructRecord][]parsing.IdName)
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
			typing.SystemV,
			order.ProcDecl.IsForeign,
		}
	}

	for node, structRecord := range nodeToStruct {
		structNode := (*node).(parsing.StructDeclare)
		name := structNode.Name
		for _, typeRecordPtr := range notDone[name] {
			unresolved := (*typeRecordPtr).(typing.Unresolved)
			*typeRecordPtr = typing.BuildRecordAccordingToUnresolved(structRecord, unresolved)
		}
		delete(notDone, name)
		env.Types[structNode.Name] = structRecord
	}
	if len(notDone) > 0 {
		for typeName := range notDone {
			return fmt.Errorf("%s does not name a type", typeName)
		}
	}
	embedGraph := make(map[*typing.StructRecord]embedGraphNode)
	for record, stringEmbedees := range embedGraphString {
		embedees := make([]*typing.StructRecord, len(stringEmbedees))
		for i, name := range stringEmbedees {
			embedees[i] = env.Types[name].(*typing.StructRecord)
		}
		embedGraph[record] = embedGraphNode{embedees: embedees}
	}
	resolveStructSize(nodeToStruct, embedGraph)
	return nil
}

func resolveStructSize(nodeToStruct map[*interface{}]*typing.StructRecord, embedGraph map[*typing.StructRecord]embedGraphNode) {
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

	scanner := bufio.NewScanner(source)
	var workOrders []*frontend.ProcWorkOrder
	var labelGen frontend.LabelIdGen
	parser := parsing.NewParser()
	typer := typing.NewTyper()
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
		if numNewEntries == 0 {
			continue
		}

		last := len(parser.OutBuffer) - 1
		isComplete := parser.OutBuffer[last].IsComplete
		node := parser.OutBuffer[last].Node
		parent := parser.OutBuffer[last].Parent
		var isForeignProc bool

		exprNode, isExpr := (*node).(parsing.ExprNode)
		if !isComplete && isExpr && exprNode.Op == parsing.ConstDeclare {
			procNode, isProc := exprNode.Right.(parsing.ProcNode)
			if isProc {
				currentProc = node
				if procNode.ProcDecl.IsForeign {
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
			procNode := procDeclare.Right.(parsing.ProcNode)
			procName := procDeclare.Left.(parsing.IdName)
			order := frontend.ProcWorkOrder{
				Out:      make(chan frontend.OptBlock),
				In:       nodesForProc,
				Name:     procName,
				ProcDecl: procNode.ProcDecl,
			}
			workOrders = append(workOrders, &order)
			go frontend.GenForProc(&labelGen, &order)
			nodesForProc = nil
			currentProc = nil
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
					Type: typer.TypeRecordFromDecl(typeDeclare.Type),
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
		// fmt.Println("Line ", line)
		// fmt.Println("Gave: ")
		// parsing.Dump(parser.OutBuffer[len(parser.OutBuffer)-numNewEntries:])
	}
	out, err := os.Create("a.asm")
	if err != nil {
		fmt.Printf("Could not create temporary asm file\n")
		os.Exit(1)
	}
	defer out.Close()

	if *libc {
		writeLibcPrologue(out)
	} else {
		writeAssemblyPrologue(out)
	}

	err = buildGlobalEnv(typer, env, structs, workOrders)
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
			fmt.Fprintf(out, "extern %s\n", workOrder.Name)
			continue
		}
		static := backend.X86ForBlock(out, ir, typeTable, env, typer, procRecord)
		staticData = append(staticData, static)
	}

	io.WriteString(out, "; ---user code end---\n")
	writeBuiltins(out)
	writeDecimalTable(out)

	io.WriteString(out, "; ---static data segment begin---\n")
	io.WriteString(out, "section .data\n")
	for _, static := range staticData {
		static.WriteTo(out)
	}

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
