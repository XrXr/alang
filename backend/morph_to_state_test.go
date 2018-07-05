package backend

import (
	"bytes"
	"fmt"
	"github.com/XrXr/alang/library"
	"github.com/XrXr/alang/typing"
	// "io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

func (p *procGen) saveAllRegisters() {
	for _, reg := range p.registers.all {
		p.issueCommand(fmt.Sprintf("push %s", reg.qwordName))
	}
}

func (p *procGen) restoreAllRegisters() {
	for i := len(p.registers.all) - 1; i >= 0; i-- {
		reg := p.registers.all[i]
		p.issueCommand(fmt.Sprintf("pop %s", reg.qwordName))
	}
}

func test(t *testing.T, currentState, targetState *fullVarState) {
	typer := typing.NewTyper()
	asm, err := os.Create("a.asm")
	if err != nil {
		t.Fatal("couldn't create temp file for asm")
	}
	typeTable := make([]typing.TypeRecord, numRegisters)
	for i := 0; i < int(numRegisters); i++ {
		typeTable[i] = typer.Builtins[typing.IntIdx]
	}
	firstOut := newOutputBlock()
	gen := procGen{
		fullVarState:     currentState,
		out:              firstOut,
		firstOutputBlock: firstOut,
		typeTable:        typeTable,
		typer:            typer,
	}
	fmt.Fprintln(gen.out.buffer, "proc_main:")
	stackSpace := int(numRegisters) * 8
	gen.issueCommand("mov rbp, rsp")
	gen.issueCommand(fmt.Sprintf("sub rsp, %d", stackSpace))
	for i := 0; i < int(numRegisters); i++ {
		gen.issueCommand(fmt.Sprintf("mov %s, %d", gen.varOperand(i), i))
	}
	gen.issueCommand("; morph")
	gen.morphToState(targetState)
	gen.fullVarState = targetState
	for i := 0; i < int(numRegisters); i++ {
		gen.issueCommand(fmt.Sprintf("; printing var %d", i))
		gen.saveAllRegisters()
		gen.issueCommand(fmt.Sprintf("mov rdi, %s", gen.varOperand(i)))
		gen.issueCommand("call proc_print_int")
		gen.restoreAllRegisters()
	}
	gen.issueCommand(fmt.Sprintf("add rsp, %d", stackSpace))
	gen.issueCommand("ret")

	library.WriteAssemblyPrologue(asm)
	collectOutput(gen.firstOutputBlock, asm)
	if err != nil {
		t.Fatal(err)
	}
	library.WriteBuiltins(asm)
	asm.Sync()

	t.Log(asm.Name())
	objectFileName := asm.Name() + ".o"
	cmd := exec.Command("nasm", "-felf64", "-o", objectFileName, asm.Name())
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal("nasm call failed", string(output))
	}
	defer os.Remove(objectFileName)
	exeName := "a.out"
	cmd = exec.Command("ld", "-o", exeName, objectFileName)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatal("ld call failed", string(output))
	}
	cmd = exec.Command("./" + exeName)
	output, err = cmd.Output()
	if err != nil {
		t.Fatal("call to executable failed to finish")
	}
	var fixture []byte
	for i := 0; i < int(numRegisters); i++ {
		fixture = append(fixture, strconv.Itoa(i)...)
		fixture = append(fixture, '\n')
	}
	if !bytes.Equal(output, fixture) {
		t.Fatal("output from the binary is incorrect")
	}
}

func logState(t *testing.T, state *fullVarState) {
	for i := range state.varStorage {
		t.Log(state.varInfoString(i))
	}
}

func TestFullRegToFullReg(t *testing.T) {
	currentState := newFullVarState(int(numRegisters))
	for i := 0; i < int(numRegisters); i++ {
		currentState.allocateRegToVar(registerId(i), i)
	}

	targetState := newFullVarState(int(numRegisters))
	targetPermutation := rand.Perm(int(numRegisters))
	for vn, regId := range targetPermutation {
		targetState.allocateRegToVar(registerId(regId), vn)
	}
	logState(t, targetState)
	test(t, currentState, targetState)
}

func TestRegToStack(t *testing.T) {
	currentState := newFullVarState(int(numRegisters))
	for i := 0; i < int(numRegisters); i++ {
		currentState.allocateRegToVar(registerId(i), i)
	}

	perm := rand.Perm(int(numRegisters))

	targetState := newFullVarState(int(numRegisters))
	offset := 8
	for i := 0; i < 3; i++ {
		vn := perm[i]
		targetState.varStorage[vn].rbpOffset = offset
		offset += 8
	}
	for i := 0; i < int(numRegisters); i++ {
		if !targetState.hasStackStorage(i) {
			targetState.allocateRegToVar(registerId(i), i)
		}
	}
	logState(t, currentState)
	logState(t, targetState)
	test(t, currentState, targetState)
}

func TestMain(m *testing.M) {
	rand.Seed(time.Now().UnixNano())
	os.Exit(m.Run())
}
