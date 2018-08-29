package backend

import (
	"bytes"
	"fmt"
	"github.com/XrXr/alang/frontend"
	"github.com/XrXr/alang/ir"
	"github.com/XrXr/alang/parsing"
	"github.com/XrXr/alang/typing"
	"io"
)

type outputBlock struct {
	buffer *bytes.Buffer
	next   *outputBlock
}

func newOutputBlock() *outputBlock {
	var block outputBlock
	block.buffer = new(bytes.Buffer)
	return &block
}

//go:generate $GOPATH/bin/stringer -type=precomputeType
type precomputeType int

const (
	notKnownAtCompileTime      precomputeType = iota
	integer                                   // .value is the value
	pointerRelativeToStackBase                // .value is an offset from rbp
	pointerRelativeToVar                      // .value is an index into a slice of relativePointer
)

type precomputeInfo struct {
	valueType       precomputeType
	value           int64
	precomputedOnce bool
}

type varPrecomputeInfo struct {
	vn int
	precomputeInfo
}

type relativePointer struct {
	baseVar int
	offset  int
}

type registerId int

const (
	rax registerId = iota
	rbx
	rcx
	rdx
	rsi
	rdi
	r8
	r9
	r10
	r11
	r12
	r13
	r14
	r15
	numRegisters
)

const invalidVn int = -1
const invalidRegister registerId = -1
const zombieMessage string = "ice: trying to revive a decommissioned variable"

var paramPassingRegOrder = [...]registerId{rdi, rsi, rdx, rcx, r8, r9}
var preservedRegisters = [...]registerId{rbx, r15, r14, r13, r12}

type registerInfo struct {
	qwordName  string // 64 bit
	dwordName  string // 32 bit
	wordName   string // 16 bit
	byteName   string // 8 bit
	occupiedBy int    // invalidVn if available
}

func (r *registerInfo) nameForSize(size int) string {
	switch size {
	case 8:
		return r.qwordName
	case 4:
		return r.dwordName
	case 2:
		return r.wordName
	case 1:
		return r.byteName
	default:
		panic("no register exactly fits that size")
	}
}

type registerBucket struct {
	all       [numRegisters]registerInfo
	available []registerId
}

func (r *registerBucket) nextAvailable() (registerId, bool) {
	if len(r.available) == 0 {
		return 0, false
	}
	return r.available[len(r.available)-1], true
}

func (r *registerBucket) allInUse() bool {
	return len(r.available) == 0
}

type varStorageInfo struct {
	rbpOffset       int        // 0 if not on stack / unknown at this time
	currentRegister registerId // invalidRegister if not in register
	decommissioned  bool
}

type fullVarState struct {
	varStorage         []varStorageInfo
	registers          registerBucket
	dontSwap           [numRegisters]bool
	nextRegToBeSwapped registerId
}

func newFullVarState(numVars int) *fullVarState {
	var state fullVarState
	state.varStorage = make([]varStorageInfo, numVars)
	for i := 0; i < numVars; i++ {
		state.varStorage[i].currentRegister = -1
	}

	bucket := &state.registers
	baseNames := [...]string{"ax", "bx", "cx", "dx", "si", "di"}
	for i, base := range baseNames {
		bucket.all[i].qwordName = "r" + base
		bucket.all[i].dwordName = "e" + base
		bucket.all[i].wordName = base
		bucket.all[i].byteName = base[0:1] + "l" // not correct for rsi and rdi. We adjust for those below
	}
	bucket.all[rsi].byteName = "sil"
	bucket.all[rdi].byteName = "dil"
	for i := len(baseNames); i < len(bucket.all); i++ {
		qwordName := fmt.Sprintf("r%d", i-len(baseNames)+8)
		bucket.all[i].qwordName = qwordName
		bucket.all[i].wordName = qwordName + "w"
		bucket.all[i].dwordName = qwordName + "d"
		bucket.all[i].byteName = qwordName + "b"
	}

	for i := range bucket.all {
		bucket.all[i].occupiedBy = invalidVn
	}

	bucket.available = []registerId{
		rax,
		rcx,
		rdx,
		rsi,
		rdi,
		r8,
		r9,
		r10,
		r11,
		// below are registers that are preserved across calls (in SystemV ABI)
		rbx,
		r15,
		r14,
		r13,
		r12,
	}
	return &state
}

func (f *fullVarState) copyVarState() *fullVarState {
	newState := *f
	newState.registers.available = make([]registerId, len(f.registers.available))
	copy(newState.registers.available, f.registers.available)
	newState.varStorage = make([]varStorageInfo, len(f.varStorage))
	copy(newState.varStorage, f.varStorage)
	return &newState
}

func (f *fullVarState) varInfoString(vn int) string {
	if f.varStorage[vn].decommissioned {
		return fmt.Sprintf("variable %d is decommissioned", vn)
	} else if f.inRegister(vn) {
		return fmt.Sprintf("variable %d is in %s", vn, f.registerOf(vn).qwordName)
	} else {
		return fmt.Sprintf("variable %d is at rbp-%d", vn, f.varStorage[vn].rbpOffset)
	}
}

type preJumpState struct {
	out    *outputBlock
	state  *fullVarState
	optIdx int
}

type procGen struct {
	*fullVarState
	block                     frontend.OptBlock
	out                       *outputBlock
	firstOutputBlock          *outputBlock
	staticDataBuf             *bytes.Buffer
	env                       *typing.EnvRecord
	procRecord                typing.ProcRecord
	typer                     *typing.Typer
	callerProvidesReturnSpace bool
	noNewStackStorage         bool // checked by loadRegisterWithVar
	currentFrameSize          int
	nextLabelId               int
	skipUntilLabel            string
	currentLineVarUsage       []int
	currentLineIdx            int
	preLoopVarState           []*fullVarState
	// info for compile time evaluation
	precompute             []precomputeInfo
	relativePointers       []relativePointer
	optionSelectPrecompute [][]varPrecomputeInfo
	constantVars           []bool
	stopPrecomputation     []int
	// info for backfilling instructions
	prologueBlock    *outputBlock
	conditionalJumps []preJumpState
	jumps            []preJumpState
	labelToState     map[string]*fullVarState
	// all three below are vn-indexed
	typeTable      []typing.TypeRecord
	lastUsage      []int
	stackBoundVars []int
}

func (p *procGen) switchToNewOutBlock() {
	current := p.out
	p.out = newOutputBlock()
	current.next = p.out
}

func (f *fullVarState) registerOf(vn int) *registerInfo {
	return &(f.registers.all[f.varStorage[vn].currentRegister])
}

func (f *fullVarState) inRegister(vn int) bool {
	return f.varStorage[vn].currentRegister > -1
}

func (f *fullVarState) hasStackStorage(vn int) bool {
	// note that rbpOffset might be negative in case of arguments
	return f.varStorage[vn].rbpOffset != 0
}

func (f *fullVarState) changeRegisterBookKeepking(vn int, register registerId) {
	f.registers.all[register].occupiedBy = vn
	f.varStorage[vn].currentRegister = register
}

func (f *fullVarState) allocateRegToVar(register registerId, vn int) {
	found := false
	var idxInAvailable int
	for i, reg := range f.registers.available {
		if reg == register {
			found = true
			idxInAvailable = i
			break
		}
	}
	if !found {
		panic("tried to take a register that's already taken")
	}
	for i := idxInAvailable + 1; i < len(f.registers.available); i++ {
		f.registers.available[i-1] = f.registers.available[i]
	}
	f.registers.available = f.registers.available[:len(f.registers.available)-1]
	f.changeRegisterBookKeepking(vn, register)
}

func (f *fullVarState) releaseRegister(register registerId) {
	for _, reg := range f.registers.available {
		if reg == register {
			panic("double release")
		}
	}
	currentOwner := f.registers.all[register].occupiedBy
	if currentOwner != invalidVn {
		f.varStorage[currentOwner].currentRegister = invalidRegister
	}
	f.registers.all[register].occupiedBy = invalidVn
	f.registers.available = append(f.registers.available, register)
}

func (p *procGen) issueCommand(command string) {
	fmt.Fprintf(p.out.buffer, "\t%s\n", command)
}

func (p *procGen) regImmCommand(command string, vn int, immediate int64) {
	p.issueCommand(fmt.Sprintf("%s %s, %d", command, p.registerOf(vn).qwordName, immediate))
}

func (p *procGen) sizeof(vn int) int {
	return p.typeTable[vn].Size()
}

func (p *procGen) fittingRegisterName(vn int) string {
	reg := p.registerOf(vn)
	size := p.sizeof(vn)
	switch {
	case size <= 1:
		return reg.byteName
	case size <= 2:
		return reg.wordName
	case size <= 4:
		return reg.dwordName
	case size <= 8:
		return reg.qwordName
	default:
		panic("does not fit in a register")
	}
}

func prefixForSize(size int) string {
	var prefix string
	switch size {
	case 8:
		prefix = "qword"
	case 4:
		prefix = "dword"
	case 2:
		prefix = "word"
	case 1:
		prefix = "byte"
	default:
		panic("no usable prefix")
	}
	return prefix
}

func makeStackOperand(prefix string, offset int) string {
	return fmt.Sprintf("%s[rbp-%d]", prefix, offset)
}

func (p *procGen) stackOperand(vn int) string {
	prefix := prefixForSize(p.typeTable[vn].Size())
	offset := p.varStorage[vn].rbpOffset
	if offset == 0 {
		panic("bad var offset")
	}
	return makeStackOperand(prefix, offset)
}

func (p *procGen) varOperand(vn int) string {
	if p.inRegister(vn) {
		return p.fittingRegisterName(vn)
	} else {
		return p.stackOperand(vn)
	}
}

func (p *procGen) rwInfoSizedToMem(memVar int, regVar int) (string, string, int) {
	memSize := p.sizeof(memVar)
	prefix := prefixForSize(memSize)
	register := p.registerOf(regVar).nameForSize(memSize)
	offset := p.varStorage[memVar].rbpOffset
	if offset == 0 {
		panic("tried to use the stack address of a var when it doesn't have one")
	}
	return prefix, register, offset
}

func (p *procGen) memRegCommand(command string, memVar int, regVar int) {
	prefix, register, offset := p.rwInfoSizedToMem(memVar, regVar)
	p.issueCommand(fmt.Sprintf("%s %s[rbp-%d], %s", command, prefix, offset, register))
}

func (p *procGen) regMemCommand(command string, regVar int, memVar int) {
	prefix, register, offset := p.rwInfoSizedToMem(memVar, regVar)
	p.issueCommand(fmt.Sprintf("%s %s, %s[rbp-%d]", command, register, prefix, offset))
}

func (p *procGen) regRegCommand(command string, a int, b int) {
	p.issueCommand(fmt.Sprintf("%s %s, %s", command, p.registerOf(a).qwordName, p.registerOf(b).qwordName))
}

func (p *procGen) regRegCommandSizedToFirst(command string, varA int, varB int) {
	varARegName := p.registerOf(varA).nameForSize(p.sizeof(varA))
	varBSizedToA := p.registerOf(varB).nameForSize(p.sizeof(varA))
	p.issueCommand(fmt.Sprintf("%s %s, %s", command, varARegName, varBSizedToA))
}

func (p *procGen) movRegReg(regA registerId, regB registerId) {
	p.issueCommand(fmt.Sprintf("mov %s, %s", p.registers.all[regA].qwordName, p.registers.all[regB].qwordName))
}

func (p *procGen) loadVarOffsetIntoReg(vn int, reg registerId) {
	p.issueCommand(fmt.Sprintf("lea %s, [rbp-%d]", p.registers.all[reg].qwordName, p.varStorage[vn].rbpOffset))
}

func (p *procGen) swapStackBoundVars() {
	for _, vn := range p.stackBoundVars {
		if p.inRegister(vn) {
			p.memRegCommand("mov", vn, vn)
			p.releaseRegister(p.varStorage[vn].currentRegister)
		}
	}
}

func (p *procGen) loadRegisterWithVar(register registerId, vn int) {
	if p.varStorage[vn].decommissioned {
		println(vn)
		panic(zombieMessage)
	}
	switch vnSize := p.sizeof(vn); {
	case vnSize == 0:
		panic("tried to put a zero-size var into a register")
	case vnSize > 8:
		panic("tried to put a var into a register when it doesn't fit")
	}

	vnAlreadyInRegister := p.inRegister(vn)
	vnRegister := p.varStorage[vn].currentRegister
	defer func() {
		// move the var from stack to reg, if it's on stack
		if !vnAlreadyInRegister && p.hasStackStorage(vn) {
			p.regMemCommand("mov", vn, vn)
		}
	}()
	currentTenant := p.registers.all[register].occupiedBy
	// take care of the var that's currently there and all the book keeping
	if currentTenant == invalidVn {
		if vnAlreadyInRegister {
			p.movRegReg(register, vnRegister)
			p.releaseRegister(vnRegister)
		}
		p.allocateRegToVar(register, vn)
	} else {
		if currentTenant == vn {
			return
		}
		if vnAlreadyInRegister {
			// both are in regiser. do a swap
			p.regRegCommand("xchg", vn, currentTenant)
			p.changeRegisterBookKeepking(currentTenant, vnRegister)
			p.changeRegisterBookKeepking(vn, register)
			return
		}

		newReg, freeRegExists := p.registers.nextAvailable()
		if freeRegExists {
			// swap currentTenant to a new register
			p.issueCommand(fmt.Sprintf("mov %s, %s", p.registers.all[newReg].qwordName, p.registers.all[register].qwordName))
			p.allocateRegToVar(newReg, currentTenant)
			p.changeRegisterBookKeepking(vn, register)
		} else {
			// swap currentTenant to stack
			if !p.noNewStackStorage {
				p.ensureStackOffsetValid(currentTenant)
			}
			if p.hasStackStorage(currentTenant) {
				p.memRegCommand("mov", currentTenant, currentTenant)
			}
			p.changeRegisterBookKeepking(vn, register)
			p.varStorage[currentTenant].currentRegister = invalidRegister
		}
	}
}

func (p *procGen) ensureInRegister(vn int) registerId {
	reg := invalidRegister
	defer func() {
		p.dontSwap[reg] = true
	}()
	if reg = p.varStorage[vn].currentRegister; reg != invalidRegister {
		return reg
	}
	reg, freeRegExists := p.registers.nextAvailable()
	if freeRegExists {
		p.loadRegisterWithVar(reg, vn)
		return reg
	} else {
		reg = p.nextRegToBeSwapped
		for p.dontSwap[reg] {
			reg = (reg + 1) % numRegisters
		}
		p.loadRegisterWithVar(reg, vn)
		p.nextRegToBeSwapped = (reg + 1) % numRegisters
		return reg
	}
}

func (p *procGen) ensureStackOffsetValid(vn int) {
	if p.varStorage[vn].decommissioned {
		panic(zombieMessage)
	}
	if p.varStorage[vn].rbpOffset != 0 {
		return
	}
	p.currentFrameSize += p.typeTable[vn].Size()
	p.varStorage[vn].rbpOffset = p.currentFrameSize
}

func (p *procGen) allocateRuntimeStorage(vn int) {
	if p.inRegister(vn) || p.hasStackStorage(vn) {
		return
	}
	reg, available := p.registers.nextAvailable()
	if available {
		p.loadRegisterWithVar(reg, vn)
	} else {
		p.ensureStackOffsetValid(vn)
	}
}

// rearrage varaible storage according to a fullVarState, return whether morphing generated any instructions.
func (p *procGen) morphToState(targetState *fullVarState) bool {
	backup := p.fullVarState.copyVarState()
	p.noNewStackStorage = true
	generationHappened := false
morph:
	for regId, reg := range targetState.registers.all {
		theirOccupiedBy := reg.occupiedBy
		for {
			ourOccupiedBy := p.registers.all[regId].occupiedBy
			if theirOccupiedBy == ourOccupiedBy {
				continue morph
			}
			if ourOccupiedBy == invalidVn {
				break
			}

			if targetState.varStorage[ourOccupiedBy].decommissioned || (!targetState.inRegister(ourOccupiedBy) && !targetState.hasStackStorage(ourOccupiedBy)) {
				// code in the target state doesn't care about the value of var ourOccupiedBy
				p.releaseRegister(registerId(regId))
			} else if targetState.inRegister(ourOccupiedBy) {
				p.loadRegisterWithVar(targetState.varStorage[ourOccupiedBy].currentRegister, ourOccupiedBy)
				generationHappened = true
			} else if targetState.hasStackStorage(ourOccupiedBy) {
				p.varStorage[ourOccupiedBy].rbpOffset = targetState.varStorage[ourOccupiedBy].rbpOffset
				p.memRegCommand("mov", ourOccupiedBy, ourOccupiedBy)
				p.releaseRegister(registerId(regId))
				generationHappened = true
			} else {
				panic("ice: this should be exhaustive")
			}
		}
		if theirOccupiedBy != invalidVn && !p.varStorage[theirOccupiedBy].decommissioned {
			p.loadRegisterWithVar(registerId(regId), theirOccupiedBy)
			generationHappened = true
		}
	}
	p.noNewStackStorage = false
	p.fullVarState = backup
	return generationHappened
}

// return the register of extendee sized to sizingVar
func (p *procGen) signOrZeroExtendIfNeeded(extendee int, sizingVar int) string {
	extendeeReg := p.registerOf(extendee)
	if p.sizeof(sizingVar) > p.sizeof(extendee) {
		p.signOrZeroExtendMov(extendee, extendee)
	}
	return extendeeReg.nameForSize(p.sizeof(sizingVar))
}

// make sure that registers passed in all have no tenant
func (p *procGen) freeUpRegisters(allocateNewStackStorage bool, targetList ...registerId) {
	for _, target := range targetList {
		currentTenant := p.registers.all[target].occupiedBy
		if currentTenant == invalidVn {
			continue
		}
		foundDifferentRegister := false
	searchForRegister:
		for _, reg := range p.registers.available {
			for _, otherTarget := range targetList {
				if otherTarget == reg {
					continue searchForRegister
				}
			}
			foundDifferentRegister = true
			p.loadRegisterWithVar(reg, currentTenant)
			break
		}
		if !foundDifferentRegister {
			if allocateNewStackStorage {
				p.ensureStackOffsetValid(currentTenant)
			}
			if p.hasStackStorage(currentTenant) {
				p.memRegCommand("mov", currentTenant, currentTenant)
			}
			p.releaseRegister(target)
		}
	}
}

func (p *procGen) zeroOutVarOnStack(vn int) {
	p.ensureStackOffsetValid(vn)
	p.freeUpRegisters(true, rdi, rcx)
	p.loadVarOffsetIntoReg(vn, rdi)
	p.issueCommand(fmt.Sprintf("mov rcx, %d", p.sizeof(vn)))
	p.issueCommand("call _intrinsic_zero_mem")
}

func isPerfectSize(size int) bool {
	return size == 8 || size == 4 || size == 2 || size == 1
}

func (p *procGen) varPerfectRegSize(vn int) bool {
	size := p.sizeof(vn)
	return isPerfectSize(size)
}

// return the mnemonic to use and the register sizing for dest
func (p *procGen) decideMovType(source typing.TypeRecord) (string, int) {
	sourceSize := source.Size()
	destSize := 8
	if sourceSize == destSize || sourceSize == 8 {
		return "mov", destSize
	}
	if p.typer.IsUnsigned(source) {
		if sourceSize == 4 {
			// upper 4 bytes automatically zeroed
			return "mov", 4
		} else {
			// movzx is available for every thing except 32 -> 64
			return "movzx", destSize
		}
	}
	return "movsx", destSize
}

func (p *procGen) signOrZeroExtendMovToReg(dest registerId, sourceVn int) {
	mnemonic, destSize := p.decideMovType(p.typeTable[sourceVn])
	destRegName := p.registers.all[dest].nameForSize(destSize)
	p.issueCommand(fmt.Sprintf("%s %s, %s", mnemonic, destRegName, p.varOperand(sourceVn)))
}

func (p *procGen) signOrZeroExtendMov(dest int, source int) {
	p.signOrZeroExtendMovToReg(p.varStorage[dest].currentRegister, source)
}

func (p *procGen) varVarCopy(dest int, source int) {
	if p.varPerfectRegSize(source) {
		p.ensureInRegister(source)
		if p.inRegister(dest) {
			if p.typeTable[dest].IsNumber() && p.typeTable[source].IsNumber() && p.sizeof(dest) > p.sizeof(source) {
				p.signOrZeroExtendMov(dest, source)
			} else {
				p.regRegCommand("mov", dest, source)
			}
		} else {
			p.ensureStackOffsetValid(dest)
			p.memRegCommand("mov", dest, source)
		}
	} else {
		if p.sizeof(dest) != p.sizeof(source) {
			panic("Assignment of two non-register-size vars. This shouldn't have made it past type checking")
		}
		if p.varStorage[source].rbpOffset == 0 {
			panic("Trying to copy from a non-register-size variable that doesn't have stack storage")
		}
		p.ensureStackOffsetValid(dest)

		p.freeUpRegisters(true, rsi, rdi, rcx)
		p.loadVarOffsetIntoReg(source, rsi)
		p.loadVarOffsetIntoReg(dest, rdi)
		p.issueCommand(fmt.Sprintf("mov rcx, %d", p.sizeof(source)))
		p.issueCommand("call _intrinsic_memcpy")
	}
}

func (p *procGen) conditionalJump(jumpInst ir.Inst) {
	label := jumpInst.Extra.(string)
	targetState := p.labelToState[label]
	// insert a new block
	originalOut := p.out
	originalNext := p.out.next
	p.switchToNewOutBlock()
	p.out.next = originalNext
	generatedCode := p.morphToState(targetState)
	if generatedCode {
		morphCodeOut := p.out
		p.out = originalOut
		nojump := p.genLabel(".nojump")
		var format string
		// we have to do state morphing before we jump so we test for the reverse condition
		if jumpInst.Type == ir.JumpIfFalse || jumpInst.Type == ir.ShortJumpIfFalse {
			format = "jnz %s"
		} else if jumpInst.Type == ir.JumpIfTrue || jumpInst.Type == ir.ShortJumpIfTrue {
			format = "jz %s"
		}
		p.issueCommand(fmt.Sprintf(format, nojump))
		p.out = morphCodeOut
		p.issueCommand(fmt.Sprintf("jmp .%s", label))
		fmt.Fprintf(p.out.buffer, "%s:\n", nojump)
	} else {
		p.out = originalOut
		originalOut.next = originalNext
		var format string
		if jumpInst.Type == ir.JumpIfFalse || jumpInst.Type == ir.ShortJumpIfFalse {
			format = "jz .%s"
		} else if jumpInst.Type == ir.JumpIfTrue || jumpInst.Type == ir.ShortJumpIfTrue {
			format = "jnz .%s"
		}
		p.issueCommand(fmt.Sprintf(format, label))
	}
}

func (p *procGen) jump(jumpInst *ir.Inst) {
	label := jumpInst.Extra.(string)
	targetState := p.labelToState[label]
	p.morphToState(targetState)
	p.issueCommand(fmt.Sprintf("jmp .%s", label))
}

func (p *procGen) jumpOrDelayedJump(optIdx int, opt *ir.Inst) {
	label := opt.Extra.(string)
	_, labelSeen := p.labelToState[label]
	if labelSeen {
		p.jump(opt)
	} else {
		p.jumps = append(p.jumps, preJumpState{
			out:    p.out,
			state:  p.copyVarState(),
			optIdx: optIdx,
		})
		p.switchToNewOutBlock()
	}
}

func (p *procGen) findOrMakeFreeReg() registerId {
	reg, freeRegExists := p.registers.nextAvailable()
	if freeRegExists {
		return reg
	}
	reg = paramPassingRegOrder[len(paramPassingRegOrder)-1]
	currentTenant := p.registers.all[reg].occupiedBy
	if currentTenant == invalidVn {
		panic("ice: inconsistent available list and register.occupiedBy")
	}
	p.ensureStackOffsetValid(currentTenant)
	p.memRegCommand("mov", currentTenant, currentTenant)
	p.releaseRegister(reg)
	return reg
}

func (p *procGen) startOptionSelect(optIdx int, opt ir.Inst) {
	outOfScopeMutations := *opt.Extra.(*[]int)
	precompStates := make([]varPrecomputeInfo, 0, len(outOfScopeMutations))
	for _, mut := range outOfScopeMutations {
		if p.valueKnown(mut) {
			precomp := p.precompute[mut]
			precompStates = append(precompStates, varPrecomputeInfo{mut, precomp})
			p.allocateRuntimeStorage(mut)
			p.issueCommand(fmt.Sprintf("mov %s, %d", p.varOperand(mut), precomp.value))
		}
	}
	p.optionSelectPrecompute = append(p.optionSelectPrecompute, precompStates)
}

func (p *procGen) endOptionSelect() {
	length := len(p.optionSelectPrecompute)
	for _, varPrecomp := range p.optionSelectPrecompute[length-1] {
		p.allocateRuntimeStorage(varPrecomp.vn)
		p.precompute[varPrecomp.vn].valueType = notKnownAtCompileTime
	}
	p.optionSelectPrecompute = p.optionSelectPrecompute[:length-1]
}

func (p *procGen) genOptionEnd(optIdx int, opt ir.Inst) {
	length := len(p.optionSelectPrecompute)
	currentPrecomp := p.optionSelectPrecompute[length-1]
	for _, varPrecomp := range currentPrecomp {
		if vn := varPrecomp.vn; p.valueKnown(varPrecomp.vn) {
			p.allocateRuntimeStorage(vn)
			p.issueCommand(fmt.Sprintf("mov %s, %d", p.varOperand(vn), p.getPrecomputedValue(vn)))
		}
	}
	for _, varPrecomp := range currentPrecomp {
		p.precompute[varPrecomp.vn] = varPrecomp.precomputeInfo
	}
}

func (p *procGen) genLabel(prefix string) string {
	label := fmt.Sprintf("%s_%d", prefix, p.nextLabelId)
	p.nextLabelId++
	return label
}

func (p *procGen) genAssignImm(optIdx int, opt ir.Inst) {
	out := opt.Out()

	if !p.precompute[out].precomputedOnce {
		switch value := opt.Extra.(type) {
		case int64:
			p.precompute[out].valueType = integer
			p.precompute[out].value = value
			p.precompute[out].precomputedOnce = true
			return
		case bool:
			var val int64 = 0
			if value == true {
				val = 1
			}
			p.precompute[out].valueType = integer
			p.precompute[out].value = val
			p.precompute[out].precomputedOnce = true
			return
		}
	}

	p.precompute[out].valueType = notKnownAtCompileTime

	switch value := opt.Extra.(type) {
	case bool:
		val := 0
		if value {
			val = 1
		}
		p.allocateRuntimeStorage(out)
		p.issueCommand(fmt.Sprintf("mov %s, %d", p.varOperand(out), val))
	case int64, uint64:
		p.ensureInRegister(out)
		p.issueCommand(fmt.Sprintf("mov %s, %d", p.registerOf(out).qwordName, value))
	case string:
		destReg := p.ensureInRegister(out)
		labelName := p.genLabel(fmt.Sprintf("static_string_%p", p.block.Opts))
		p.issueCommand(fmt.Sprintf("mov %s, %s", p.registers.all[destReg].qwordName, labelName))

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
			buf.WriteString(`",0`)
		} else {
			// it's a string that ends with \n
			buf.WriteRune('0')
		}

		p.staticDataBuf.WriteString(fmt.Sprintf("%s:\n", labelName))
		p.staticDataBuf.WriteString(fmt.Sprintf("\tdq\t%d\n", byteCount))
		p.staticDataBuf.ReadFrom(&buf)
		p.staticDataBuf.WriteRune('\n')
	case parsing.TypeDecl, parsing.LiteralType:
		// :structinreg
		out := opt.Out()
		_, isStruct := p.typeTable[out].(typing.StructRecord)
		_, isArray := p.typeTable[out].(typing.Array)
		freeReg, freeRegExists := p.registers.nextAvailable()
		if !isStruct && !isArray && p.varPerfectRegSize(out) && freeRegExists {
			p.loadRegisterWithVar(freeReg, out)
		}
		if p.inRegister(out) {
			p.issueCommand(fmt.Sprintf("mov %s, 0", p.registerOf(out).qwordName))
		} else {
			p.zeroOutVarOnStack(out)
		}
	default:
		parsing.Dump(value)
		panic("unknown immediate value type")
	}
}

func (p *procGen) genCall(optIdx int, opt ir.Inst) {
	p.swapStackBoundVars()
	extra := opt.Extra.(ir.CallExtra)
	if typeRecord, callToType := p.env.Types[extra.Name]; callToType {
		switch typeRecord.(type) {
		case *typing.StructRecord:
			// making a struct. We never put structs in registers even if they fit
			// :structinreg
			p.zeroOutVarOnStack(opt.Out())
		default:
			// cast
			firstArg := extra.ArgVars[0]
			if p.valueKnown(firstArg) {
				p.precompute[opt.Out()] = p.precompute[firstArg]
			} else {
				p.varVarCopy(opt.Out(), extra.ArgVars[0])
			}
		}
	} else {
		retVar := opt.Out()
		procRecord := p.env.Procs[extra.Name]
		var numStackVars int
		numArgs := len(extra.ArgVars)
		provideReturnStorage := p.sizeof(retVar) > 16
		if provideReturnStorage {
			numArgs += 1
		}

		visibleArgsInReg := len(extra.ArgVars)
		if len(paramPassingRegOrder) <= visibleArgsInReg {
			visibleArgsInReg = len(paramPassingRegOrder)
			if provideReturnStorage {
				visibleArgsInReg--
			}
		}

		if visibleArgsInReg < len(extra.ArgVars) {
			tmpReg := p.findOrMakeFreeReg()
			tmpRegInfo := &p.registers.all[tmpReg]
			numStackVars = len(extra.ArgVars) - visibleArgsInReg
			if numStackVars%2 == 1 {
				// Make sure we are aligned to 16
				p.issueCommand("sub rsp, 8")
			}
			for i := len(extra.ArgVars) - 1; i >= len(extra.ArgVars)-numStackVars; i-- {
				arg := extra.ArgVars[i]
				argSize := p.typeTable[arg].Size()
				switch argSize {
				case 8, 4, 2, 1:
					if p.valueKnown(arg) {
						p.loadKnownValueIntoRegSized(arg, procRecord.Args[i], tmpReg)
					} else {
						p.signOrZeroExtendMovToReg(tmpReg, arg)
					}
					p.issueCommand(fmt.Sprintf("push %s", tmpRegInfo.qwordName))
				default:
					panic("Unsupported parameter size")
				}
			}
		}

		for i, arg := range extra.ArgVars {
			if provideReturnStorage {
				i += 1
			}
			if i >= len(paramPassingRegOrder) {
				break
			}
			switch valueSize := p.typeTable[arg].Size(); valueSize {
			case 8, 4, 2, 1:
				reg := paramPassingRegOrder[i]
				p.loadRegisterWithVar(reg, arg)
				if p.valueKnown(arg) {
					p.loadKnownValueIntoRegSized(arg, procRecord.Args[i], reg)
				} else {
					if valueSize < procRecord.Args[i].Size() {
						p.signOrZeroExtendMovToReg(reg, arg)
					}
				}
			default:
				panic("Unsupported parameter size")
			}
		}

		// the first part of this array is the same as paramPassingRegOrder
		regsThatGetDestroyed := [...]registerId{rdi, rsi, rdx, rcx, r8, r9, rax, r10, r11}
		for _, reg := range regsThatGetDestroyed {
			owner := p.registers.all[reg].occupiedBy
			if owner != invalidVn {
				if !(p.lastUsage[owner] == optIdx || p.valueKnown(owner)) {
					p.ensureStackOffsetValid(owner)
					p.memRegCommand("mov", owner, owner)
				}
				p.releaseRegister(reg)
			}
		}

		if provideReturnStorage {
			p.ensureStackOffsetValid(retVar)
			p.loadVarOffsetIntoReg(retVar, rdi)
		}

		if procRecord.IsForeign {
			p.issueCommand(fmt.Sprintf("call %s  wrt ..plt", extra.Name))
		} else {
			p.issueCommand(fmt.Sprintf("call proc_%s", extra.Name))
		}

		// TODO this needs to change when we support things bigger than 8 bytes
		if numArgs > len(paramPassingRegOrder) {
			p.issueCommand(fmt.Sprintf("add rsp, %d", numStackVars*8+numStackVars%2*8))
		}
		if p.registers.all[rax].occupiedBy != invalidVn {
			panic("rax should've been freed up before the call")
		}
		if p.sizeof(retVar) > 0 && p.sizeof(retVar) <= 8 {
			if p.inRegister(retVar) {
				p.releaseRegister(p.varStorage[retVar].currentRegister)
			}
			p.allocateRegToVar(rax, retVar)
		}
	}

}

func (p *procGen) genReturn(optIdx int, opt ir.Inst) {
	returnType := *p.procRecord.Return

	returnExtra := opt.Extra.(ir.ReturnExtra)
	if len(returnExtra.Values) > 0 {
		retVar := returnExtra.Values[0]
		if p.valueKnown(retVar) {
			p.loadKnownValueIntoReg(retVar, rax)
		} else if p.varPerfectRegSize(retVar) {
			p.loadRegisterWithVar(rax, retVar)
			if returnType.Size() > p.sizeof(retVar) {
				p.signOrZeroExtendMov(retVar, retVar)
			}
		} else if p.callerProvidesReturnSpace {
			if returnType.Size() != p.sizeof(retVar) {
				panic("ice: returning a big var that doesn't match the size of the declared return type. Typechecker should've caught it")
			}
			if p.inRegister(retVar) {
				panic("ice: a var this big shouldn't be in register")
			}
			p.ensureStackOffsetValid(retVar)
			p.freeUpRegisters(false, rsi, rdi, rcx)
			p.issueCommand("mov rdi, qword [rbp-8]")
			p.loadVarOffsetIntoReg(retVar, rsi)
			p.issueCommand(fmt.Sprintf("mov rcx, %d", p.sizeof(retVar)))
			p.issueCommand("call _intrinsic_memcpy")
		} else {
			panic("Can't handle return where the data doesn't exactly fit a register or 8 < size <= 16 yet")
		}
	}
	p.issueCommand("jmp .end_of_proc")
}

func (p *procGen) setccToVar(how ir.ComparisonMethod, vn int) {
	var mnemonic string
	switch how {
	case ir.Greater:
		mnemonic = "setg"
	case ir.Lesser:
		mnemonic = "setl"
	case ir.GreaterOrEqual:
		mnemonic = "setge"
	case ir.LesserOrEqual:
		mnemonic = "setle"
	case ir.AreEqual:
		mnemonic = "sete"
	case ir.NotEqual:
		mnemonic = "setne"
	default:
		panic("ice: passed a unknown method of comparison")
	}
	p.issueCommand(fmt.Sprintf("%s %s", mnemonic, p.varOperand(vn)))
}

func (p *procGen) andOrImm(opt *ir.Inst, imm int64) {
	var mnemonic string
	switch opt.Type {
	case ir.And:
		mnemonic = "and"
	case ir.Or:
		mnemonic = "or"
	}
	if p.sizeof(opt.Left()) == 1 {
		p.issueCommand(fmt.Sprintf("%s %s, %d", mnemonic, p.varOperand(opt.Left()), imm))
	} else {
		panic("ice: and and or for more than 1 byte is not supported yet")
	}
}

func (p *procGen) genTakeAddress(optIdx int, opt ir.Inst) {
	in := opt.In()
	out := opt.Out()
	if p.precompute[out].precomputedOnce && !p.valueKnown(out) {
		panic("ice: re-precompute by outputting from ir.TakeAddress")
	}
	p.stopPrecomputingAndMaterialize(in)
	p.ensureStackOffsetValid(in)
	p.stackBoundVars = append(p.stackBoundVars, in)
	p.precompute[out].valueType = pointerRelativeToStackBase
	p.precompute[out].precomputedOnce = true
	p.precompute[out].value = -int64(p.varStorage[in].rbpOffset)
}

func (p *procGen) addRelativePointer(baseVar int, offset int) int64 {
	rel := relativePointer{baseVar: baseVar, offset: offset}
	p.relativePointers = append(p.relativePointers, rel)
	length := len(p.relativePointers)
	return int64(length - 1)
}

func (p *procGen) arrayToPointer(optIdx int, opt ir.Inst) {
	in := opt.In()
	out := opt.Out()
	if p.precompute[out].precomputedOnce && !p.valueKnown(out) {
		panic("ice: re-precompute by outputting from ir.ArrayToPointer")
	}
	if p.valueKnown(in) {
		precomp := p.precompute[in]
		switch precomp.valueType {
		case pointerRelativeToVar:
			rel := p.relativePointers[precomp.value]
			p.precompute[out].valueType = pointerRelativeToVar
			p.precompute[out].value = p.addRelativePointer(rel.baseVar, rel.offset)
			p.precompute[out].precomputedOnce = true
		case pointerRelativeToStackBase:
			p.precompute[out] = precomp
		default:
			panic("ice: can convert precomputed array to data pointer. Value type: " + precomp.valueType.String())
		}
	} else {
		switch p.typeTable[in].(type) {
		case typing.Pointer:
			p.precompute[out].valueType = pointerRelativeToVar
			p.precompute[out].value = p.addRelativePointer(in, 0)
			p.precompute[out].precomputedOnce = true
		default:
			p.ensureStackOffsetValid(in)
			p.precompute[out].valueType = pointerRelativeToStackBase
			p.precompute[out].value = -int64(p.varStorage[in].rbpOffset)
			p.precompute[out].precomputedOnce = true
		}
	}
}

func (p *procGen) evalCompare(opt *ir.Inst) int64 {
	extra := opt.Extra.(ir.CompareExtra)
	leftValue := p.getPrecomputedValue(opt.ReadOperand)
	rightValue := p.getPrecomputedValue(extra.Right)
	var result bool
	switch extra.How {
	case ir.Lesser:
		result = leftValue < rightValue
	case ir.LesserOrEqual:
		result = leftValue <= rightValue
	case ir.Greater:
		result = leftValue > rightValue
	case ir.GreaterOrEqual:
		result = leftValue >= rightValue
	case ir.AreEqual:
		result = leftValue == rightValue
	case ir.NotEqual:
		result = leftValue != rightValue
	}

	if result {
		return 1
	} else {
		return 0
	}
}

func (p *procGen) startLoop() {
	p.preLoopVarState = append(p.preLoopVarState, p.copyVarState())
}

func (p *procGen) valueKnown(vn int) bool {
	return p.precompute[vn].valueType != notKnownAtCompileTime
}

func (p *procGen) knownStatForOpt(opt ir.Inst) (bool, bool, bool) {
	mut := ir.FindMutationVar(&opt)
	allVarsKnown := true
	anyVarKnown := false
	allInputsKnown := true
	ir.IterOverAllVars(opt, func(vn int) {
		if p.precompute[vn].valueType == notKnownAtCompileTime {
			allVarsKnown = false
			if vn != mut {
				allInputsKnown = false
			}
		} else {
			anyVarKnown = true
		}
	})
	return allVarsKnown, anyVarKnown, allInputsKnown
}

func (p *procGen) scribeVarUsage(vn int) {
	p.currentLineVarUsage = append(p.currentLineVarUsage, vn)
}

func (p *procGen) decommissionAllTempVars() {
	for _, vn := range p.currentLineVarUsage {
		dontDecommision := false
		for _, nonTmp := range p.block.NonTemporaryVars {
			if nonTmp == vn {
				dontDecommision = true
				break
			}
		}
		if !dontDecommision {
			p.decommission(vn)
		}
	}
	p.currentLineVarUsage = p.currentLineVarUsage[:0]
}

// Mark a variable as no longer used. It will stop having
// any storage and and value it had might be clobbered
func (p *procGen) decommission(vn int) {
	p.varStorage[vn].decommissioned = true
	reg := p.varStorage[vn].currentRegister
	if reg != invalidRegister {
		p.releaseRegister(reg)
	}
	p.varStorage[vn].rbpOffset = 0
}

func (p *procGen) generateSingleInst(optIdx int, opt ir.Inst) {
	if p.skipUntilLabel != "" {
		if opt.Type == ir.Label && opt.Extra.(string) == p.skipUntilLabel {
			p.skipUntilLabel = ""
		}
		return
	}
	if opt.GeneratedFrom != nil {
		optLineIdx := opt.GeneratedFrom.GetLineNumber()
		if optLineIdx > p.currentLineIdx {
			// println("source line", optLineIdx+1)
			p.decommissionAllTempVars()
			p.currentLineIdx = optLineIdx
		}
	}

	stopPrecomputingIfNeeded := func(vn int) {
		if stopAt := p.stopPrecomputation[vn]; stopAt == optIdx {
			p.stopPrecomputingAndMaterialize(vn)
		}
	}
	defer ir.IterOverAllVars(opt, stopPrecomputingIfNeeded)
	defer ir.IterOverAllVars(opt, p.scribeVarUsage)
	for i := range p.dontSwap {
		p.dontSwap[i] = false
	}

	switch opt.Type {
	case ir.TakeAddress:
		p.genTakeAddress(optIdx, opt)
		return
	case ir.ArrayToPointer:
		p.arrayToPointer(optIdx, opt)
		return
	case ir.OutOfScopeMutations, ir.OutsideLoopMutations:
		for _, vn := range *opt.Extra.(*[]int) {
			stopPrecomputingIfNeeded(vn)
		}
		if opt.Type == ir.OutsideLoopMutations {
			// We need to do this after we materialize any precomputed var
			// because we restore var storage state after the loop ends.
			// If we don't include those vars in the backup we would clobber them
			p.startLoop()
		}
		return
	case ir.LoopEnd:
		return
	case ir.OptionSelectStart:
		p.startOptionSelect(optIdx, opt)
		return
	case ir.OptionEnd:
		p.genOptionEnd(optIdx, opt)
		return
	case ir.OptionSelectEnd:
		p.endOptionSelect()
		return
	}

	if opt.Type == ir.IndirectLoad && p.valueKnown(opt.Out()) {
		// A special case. Even if we know both operands at copmile time,
		// we need to stop precomputing the output var.
		p.stopPrecomputingAndMaterialize(opt.Out())
	}
	mut := ir.FindMutationVar(&opt)
	var allInputsKnown, anyVarKnown bool
	varStat := func() {
		anyVarKnown = false
		allInputsKnown = true
		ir.IterOverAllVars(opt, func(vn int) {
			if p.precompute[vn].valueType == notKnownAtCompileTime {
				if vn != mut {
					allInputsKnown = false
				}
			} else {
				anyVarKnown = true
			}
		})
	}
	varStat()
	if mut >= 0 && p.valueKnown(mut) && !allInputsKnown {
		p.stopPrecomputingAndMaterialize(mut)
		varStat()
	}
	switch opt.Type {
	case ir.AssignImm:
		p.genAssignImm(optIdx, opt)
		return
	case ir.Call:
		p.genCall(optIdx, opt)
		return
	case ir.Return:
		p.genReturn(optIdx, opt)
		return
	}

	// PeelStruct and StructMemberPointer  can work even when the input is a runtime value
	if allInputsKnown || opt.Type == ir.PeelStruct || opt.Type == ir.StructMemberPtr {
		if p.doPrecomputaion(optIdx, opt) {
			return
		}
	}

	if anyVarKnown {
		p.genInstPartialKnown(optIdx, opt)
	} else {
		p.genInstAllRuntimeVars(optIdx, opt)

		ir.IterOverAllVars(opt, func(vn int) {
			if p.lastUsage[vn] == optIdx {
				p.decommission(vn)
			}
		})
	}
}

func (p *procGen) generate() {
	{
		paramOffset := -16 - 8*len(preservedRegisters)
		for i := 0; i < p.block.NumberOfArgs; i++ {
			regOrder := i
			if p.callerProvidesReturnSpace {
				regOrder += 1
			}
			if regOrder < len(paramPassingRegOrder) {
				p.loadRegisterWithVar(paramPassingRegOrder[regOrder], i)
			} else {
				p.varStorage[i].rbpOffset = paramOffset
				paramOffset -= 8 // params are rounded to eightbytes
			}
		}
	}

	// backendDebug(framesize, p.typeTable)
	for optIdx, opt := range p.block.Opts {
		// if opt.GeneratedFrom != nil && opt.GeneratedFrom.GetLineNumber() == 26 {
		// 	p.issueCommand("; line 26")

		// }
		// fmt.Println("doing ir line", optIdx)
		// fmt.Fprintf(p.out.buffer, ".ir_line_%d:\n", optIdx)
		p.generateSingleInst(optIdx, opt)
		// p.trace(23)
		// fmt.Printf("stroage for %d %#v\n", 1, p.varStorage[1])
	}

	for _, jump := range p.conditionalJumps {
		p.out = jump.out
		p.fullVarState = jump.state
		p.conditionalJump(p.block.Opts[jump.optIdx])
	}
	for _, jump := range p.jumps {
		p.out = jump.out
		p.fullVarState = jump.state
		p.jump(&p.block.Opts[jump.optIdx])
	}
}

func (p *procGen) getPrecomputedValue(vn int) int64 {
	if !p.valueKnown(vn) {
		println(vn)
		panic("ice: a value expected to be pre-computable is not")
	}
	return p.precompute[vn].value
}

func (p *procGen) stopPrecomputingAndMaterialize(vn int) {
	if !p.valueKnown(vn) {
		return
	}
	vnReg := p.ensureInRegister(vn)
	p.dontSwap[vnReg] = false
	p.loadKnownValueIntoReg(vn, vnReg)
	p.precompute[vn].valueType = notKnownAtCompileTime
}

// Caller makes sure that the value for every input variable is known at compile time
// returns whether whether the func was able to do precomputation for the instruction
func (p *procGen) doPrecomputaion(optIdx int, opt ir.Inst) bool {
	switch opt.Type {
	case ir.Add, ir.Sub, ir.Div, ir.Mult, ir.And, ir.Or, ir.Increment, ir.Decrement:
		if !p.valueKnown(opt.MutateOperand) {
			return false
		}
	}
	// check if we would start doing precomputation again
	if opt.Type.InputOutput() && p.precompute[opt.Out()].precomputedOnce && !p.valueKnown(opt.Out()) {
		return false
	}
	switch opt.Type {
	case ir.Assign:
		if !p.valueKnown(opt.Right()) {
			panic("ice: input to assign not known at compile time")
		}
		left := opt.Left()
		right := opt.Right()
		p.precompute[left] = p.precompute[right]
	case ir.Add:
		left := opt.Left()
		right := opt.Right()
		precomp := &p.precompute[left]
		rightValue := p.getPrecomputedValue(right)
		switch precomp.valueType {
		case integer:
			precomp.value += rightValue
		case pointerRelativeToVar, pointerRelativeToStackBase:
			pointedToSize := p.typeTable[left].(typing.Pointer).ToWhat.Size()
			delta := int64(pointedToSize) * rightValue
			if precomp.valueType == pointerRelativeToStackBase {
				precomp.value += delta
			} else {
				p.relativePointers[precomp.value].offset += int(delta)
			}
		default:
			panic("adding to an unsupported precomp value type " + precomp.valueType.String())
		}
	case ir.Sub:
		p.precompute[opt.Left()].value -= p.getPrecomputedValue(opt.Right())
	case ir.Mult:
		p.precompute[opt.Left()].value *= p.getPrecomputedValue(opt.Right())
	case ir.Div:
		rightValue := p.getPrecomputedValue(opt.Right())
		if rightValue == 0 {
			panic(parsing.ErrorFromNode(opt.GeneratedFrom, "Divide by zero"))
		}
		p.precompute[opt.Left()].value /= rightValue
	case ir.Compare:
		extra := opt.Extra.(ir.CompareExtra)
		if !(p.valueKnown(opt.ReadOperand) && p.valueKnown(extra.Right)) {
			return false
		}
		p.precompute[extra.Out].valueType = integer
		p.precompute[extra.Out].value = p.evalCompare(&opt)
	case ir.Increment:
		p.precompute[opt.Out()].value++
	case ir.Decrement:
		p.precompute[opt.Out()].value--
	case ir.Not:
		p.precompute[opt.Out()].valueType = integer
		if p.precompute[opt.In()].value == 0 {
			p.precompute[opt.Out()].value = 1
		} else {
			p.precompute[opt.Out()].value = 0
		}
	case ir.And:
		p.precompute[opt.Left()].value &= p.getPrecomputedValue(opt.Right())
	case ir.Or:
		p.precompute[opt.Left()].value |= p.getPrecomputedValue(opt.Right())
	case ir.PeelStruct:
		in := opt.In()
		fieldName := opt.Extra.(string)
		switch inType := p.typeTable[in].(type) {
		case *typing.StructRecord:
			_, memberIsPointer := inType.Members[fieldName].Type.(typing.Pointer)
			// If it's a pointer we would need to do a load
			if memberIsPointer {
				return false
			}
		case typing.Pointer:
			record := inType.ToWhat.(*typing.StructRecord)
			_, memberIsPointer := record.Members[fieldName].Type.(typing.Pointer)
			// If it's a pointer we would need to deference in. Can't do that at compile time.
			if memberIsPointer {
				return false
			}
		}
		fallthrough
	case ir.StructMemberPtr:
		// as a special case, we can get here even if we don't know the value of in
		in := opt.In()
		out := opt.Out()
		switch inType := p.typeTable[in].(type) {
		case *typing.StructRecord:
			p.ensureStackOffsetValid(in)
			fieldName := opt.Extra.(string)
			memberOffset := inType.Members[fieldName].Offset
			p.precompute[out].valueType = pointerRelativeToStackBase
			p.precompute[out].value = int64(-p.varStorage[in].rbpOffset + memberOffset)
			p.precompute[out].precomputedOnce = true
			return true
		case typing.String:
			fieldName := opt.Extra.(string)
			offset := 0
			if fieldName == "data" {
				offset = 8
			}
			p.precompute[out].valueType = pointerRelativeToVar
			p.precompute[out].value = p.addRelativePointer(in, offset)
			p.precompute[out].precomputedOnce = true
			return true
		case typing.Pointer:
			record, pointerToStruct := inType.ToWhat.(*typing.StructRecord)
			if !pointerToStruct {
				return false
			}
			fieldName := opt.Extra.(string)
			memberOffset := record.Members[fieldName].Offset
			precomp := p.precompute[in]
			switch precomp.valueType {
			case pointerRelativeToVar:
				rel := p.relativePointers[precomp.value]
				p.precompute[out].valueType = pointerRelativeToVar
				p.precompute[out].value = p.addRelativePointer(rel.baseVar, rel.offset+memberOffset)
				p.precompute[out].precomputedOnce = true
				return true
			case pointerRelativeToStackBase:
				p.precompute[out].valueType = pointerRelativeToStackBase
				p.precompute[out].value = precomp.value + int64(memberOffset)
				p.precompute[out].precomputedOnce = true
				return true
			case notKnownAtCompileTime:
				p.precompute[out].valueType = pointerRelativeToVar
				p.precompute[out].value = p.addRelativePointer(in, memberOffset)
				p.precompute[out].precomputedOnce = true
				return true
			}
		}
		return false
	default:
		return false
	}
	return true
}

func (p *procGen) prepareEffectiveAddressWithOffset(pointerVn int, additonalOffset int) string {
	if p.valueKnown(pointerVn) {
		precomp := &p.precompute[pointerVn]
		switch precomp.valueType {
		case pointerRelativeToStackBase:
			return fmt.Sprintf("[rbp+%d]", precomp.value+int64(additonalOffset))
		case pointerRelativeToVar:
			rel := &p.relativePointers[precomp.value]
			p.ensureInRegister(rel.baseVar)
			return fmt.Sprintf("[%s+%d]", p.registerOf(rel.baseVar).qwordName, rel.offset+additonalOffset)
		default:
			panic("ice: unknown precomp pointer type")
		}
	} else {
		p.ensureInRegister(pointerVn)
		return fmt.Sprintf("[%s+%d]", p.registerOf(pointerVn).qwordName, additonalOffset)
	}
}

func (p *procGen) prepareEffectiveAddress(pointerVn int) string {
	return p.prepareEffectiveAddressWithOffset(pointerVn, 0)
}

// @clobbber
func (p *procGen) loadPointerIntoReg(pointerVn int, reg registerId) {
	if p.valueKnown(pointerVn) {
		effectiveAddress := p.prepareEffectiveAddress(pointerVn)
		p.issueCommand(fmt.Sprintf("lea %s, %s", p.registers.all[reg].qwordName, effectiveAddress))
	} else {
		p.loadRegisterWithVar(reg, pointerVn)
	}
}

// @clobber
func (p *procGen) loadKnownValueIntoRegSized(vn int, sizingType typing.TypeRecord, reg registerId) {
	switch precomp := p.precompute[vn]; precomp.valueType {
	case integer:
		p.issueCommand(fmt.Sprintf("mov %s, %d", p.registers.all[reg].nameForSize(sizingType.Size()), precomp.value))
	case pointerRelativeToVar, pointerRelativeToStackBase:
		p.loadPointerIntoReg(vn, reg)
	default:
		panic("ice: don't know how to materialize precompute value type " + precomp.valueType.String())
	}
}

// @clobber
func (p *procGen) loadKnownValueIntoReg(vn int, reg registerId) {
	p.loadKnownValueIntoRegSized(vn, p.typeTable[vn], reg)
}

// caller makes sure that we don't try to put a runtime value into a compile time variable
func (p *procGen) genInstPartialKnown(optIdx int, opt ir.Inst) {
	switch opt.Type {
	case ir.Assign:
		l := opt.Left()
		r := opt.Right()
		p.ensureStackOffsetValid(l)
		lReg := p.ensureInRegister(l)
		p.loadKnownValueIntoReg(r, lReg)
	case ir.Add:
		howMuch := p.getPrecomputedValue(opt.Right())
		if pointer, isPointer := p.typeTable[opt.Left()].(typing.Pointer); isPointer {
			howMuch *= int64(pointer.ToWhat.Size())
		}
		p.issueCommand(fmt.Sprintf("add %s, %d", p.varOperand(opt.Left()), howMuch))
	case ir.Sub:
		p.issueCommand(fmt.Sprintf("sub %s, %d", p.varOperand(opt.Left()), p.getPrecomputedValue(opt.Right())))
	case ir.Mult:
		l := opt.Left()
		r := opt.Right()
		tmpStackStorage := fmt.Sprintf("%s[rsp-%d]", prefixForSize(p.sizeof(l)), p.sizeof(l))
		p.issueCommand(fmt.Sprintf("mov %s, %d", tmpStackStorage, p.getPrecomputedValue(r)))
		if p.sizeof(l) == 1 {
			p.loadRegisterWithVar(rax, l)
			p.issueCommand(fmt.Sprintf("imul %s", tmpStackStorage))
		} else {
			p.ensureInRegister(l)
			p.issueCommand(fmt.Sprintf("imul %s, %s", p.fittingRegisterName(l), tmpStackStorage))
		}
	case ir.Div:
		l := opt.Left()
		r := opt.Right()
		precompValue := p.getPrecomputedValue(r)
		if precompValue == 0 {
			panic(parsing.ErrorFromNode(opt.GeneratedFrom, "Divide by zero"))
		}
		tmpStackStorage := fmt.Sprintf("%s[rsp-%d]", prefixForSize(p.sizeof(l)), p.sizeof(l))
		p.issueCommand(fmt.Sprintf("mov %s, %d", tmpStackStorage, precompValue))
		p.loadRegisterWithVar(rax, l)
		p.freeUpRegisters(true, rdx)
		p.issueCommand("xor rdx, rdx")
		p.issueCommand(fmt.Sprintf("div %s", tmpStackStorage))
	case ir.Compare:
		extra := opt.Extra.(ir.CompareExtra)
		l := opt.ReadOperand
		r := extra.Right
		lSize := p.sizeof(l)
		rSize := p.sizeof(r)
		tmpReg := p.findOrMakeFreeReg()
		var leftOperand string
		var rightOperand string
		if p.valueKnown(l) {
			p.loadKnownValueIntoReg(l, tmpReg)
			leftOperand = p.registers.all[tmpReg].nameForSize(rSize)
			rightOperand = p.varOperand(r)
		} else if p.valueKnown(r) {
			p.loadKnownValueIntoReg(r, tmpReg)
			leftOperand = p.varOperand(l)
			rightOperand = p.registers.all[tmpReg].nameForSize(lSize)
		} else {
			panic("ice: genPartialKnown cmp: one of left or right must be known")
		}

		p.issueCommand(fmt.Sprintf("cmp %s, %s", leftOperand, rightOperand))
		p.allocateRuntimeStorage(extra.Out)
		p.setccToVar(extra.How, extra.Out)
	case ir.And:
		preCompValue := p.getPrecomputedValue(opt.Right())
		if preCompValue == 0 {
			p.issueCommand(fmt.Sprintf("mov %s, 0", p.varOperand(opt.Left())))
		} else {
			p.andOrImm(&opt, preCompValue)
		}
	case ir.Or:
		preCompValue := p.getPrecomputedValue(opt.Right())
		if preCompValue != 0 {
			p.issueCommand(fmt.Sprintf("mov %s, 1", p.varOperand(opt.Left())))
		} else {
			p.andOrImm(&opt, preCompValue)
		}
	case ir.JumpIfTrue:
		if p.getPrecomputedValue(opt.ReadOperand) != 0 {
			p.jumpOrDelayedJump(optIdx, &opt)
		} else {
			p.issueCommand("; never jumps")
		}
	case ir.JumpIfFalse:
		if p.getPrecomputedValue(opt.ReadOperand) == 0 {
			p.jumpOrDelayedJump(optIdx, &opt)
		} else {
			p.issueCommand("; never jumps")
		}
	case ir.ShortJumpIfTrue:
		// The difference between short jumps and normal jumps is that for short jumps,
		// the jump target is within the same source line and is somewhere ahead in the ir list.
		// This guarentees that between the short jump and the target, there is no mutation of
		// named variables. Since nothing user-visible is changed, we can safely skip over all
		// the insts in between if we know the condition at compile time.
		if p.getPrecomputedValue(opt.ReadOperand) != 0 {
			p.skipUntilLabel = opt.Extra.(string)
		} else {
			p.issueCommand("; never jumps")
		}
	case ir.ShortJumpIfFalse:
		if p.getPrecomputedValue(opt.ReadOperand) == 0 {
			p.skipUntilLabel = opt.Extra.(string)
		} else {
			p.issueCommand("; never jumps")
		}
	case ir.IndirectLoad:
		out := opt.Out()
		in := opt.In()
		if p.valueKnown(out) {
			panic("ice: can't indirect load into a copmile time value. Should've checked")
		}

		switch inType := p.typeTable[in].(type) {
		case typing.StringDataPointer:
			outReg := p.ensureInRegister(out)
			p.loadPointerIntoReg(in, outReg)
			return
		case typing.Pointer:
			pointedToSize := inType.ToWhat.Size()
			p.swapStackBoundVars()
			outSize := p.sizeof(out)
			if isPerfectSize(pointedToSize) && isPerfectSize(outSize) {
				p.ensureInRegister(out)
				sourceOperand := p.prepareEffectiveAddress(in)
				prefix := prefixForSize(pointedToSize)
				mnemonic, outRegSizing := p.decideMovType(inType.ToWhat)
				regName := p.registerOf(out).nameForSize(outRegSizing)
				p.issueCommand(fmt.Sprintf("%s %s, %s %s", mnemonic, regName, prefix, sourceOperand))
			} else {
				if pointedToSize != p.sizeof(out) {
					panic("ice: memcpy indirect load where the sizes are not equal")
				}
				p.freeUpRegisters(true, rsi, rdi, rcx)
				p.loadPointerIntoReg(in, rsi)
				p.ensureStackOffsetValid(out)
				p.loadVarOffsetIntoReg(out, rdi)
				p.issueCommand(fmt.Sprintf("mov rcx, %d", pointedToSize))
				p.issueCommand("call _intrinsic_memcpy")
				if p.inRegister(out) {
					p.releaseRegister(p.varStorage[out].currentRegister)
				}
			}
		default:
			panic("ice: don't know how to IndirectLoad from " + inType.Rep())
		}
	case ir.IndirectWrite:
		p.swapStackBoundVars()
		target := opt.Out()
		data := opt.In()
		pointedToSize := p.typeTable[target].(typing.Pointer).ToWhat.Size()

		if p.varPerfectRegSize(data) {
			destOperand := p.prepareEffectiveAddress(target)
			prefix := prefixForSize(pointedToSize)
			if p.valueKnown(data) {
				switch precomp := &p.precompute[data]; precomp.valueType {
				case integer:
					precompValue := p.getPrecomputedValue(data)
					p.issueCommand(fmt.Sprintf("mov %s %s, %d", prefix, destOperand, precompValue))
				case pointerRelativeToVar, pointerRelativeToStackBase:
					tmpReg := p.findOrMakeFreeReg()
					p.loadPointerIntoReg(data, tmpReg)
					p.issueCommand(fmt.Sprintf("mov %s %s, %s", prefix, destOperand, p.registers.all[tmpReg].qwordName))
				default:
					panic("don't know how to do an indirect write with precomputed type " + precomp.valueType.String())
				}
			} else {
				p.ensureInRegister(data)
				// TODO sign extend here
				regName := p.registerOf(data).nameForSize(pointedToSize)
				p.issueCommand(fmt.Sprintf("mov %s %s, %s", prefix, destOperand, regName))
			}
		} else {
			if pointedToSize != p.sizeof(data) {
				panic("ice: memcpy indirect write where the sizes are not equal")
			}
			p.freeUpRegisters(true, rsi, rdi, rcx)
			p.loadPointerIntoReg(target, rdi)
			p.ensureStackOffsetValid(data)
			if p.inRegister(data) {
				p.memRegCommand("mov", data, data)
			}
			p.loadVarOffsetIntoReg(data, rsi)
			p.issueCommand(fmt.Sprintf("mov rcx, %d", pointedToSize))
			p.issueCommand("call _intrinsic_memcpy")
		}
	case ir.StructMemberPtr:
		out := opt.Out()
		in := opt.In()
		pointer, isPointer := p.typeTable[in].(typing.Pointer)
		_, isPointerToString := pointer.ToWhat.(typing.String)
		if !isPointer || !isPointerToString {
			// The only case where we can't keep doing precomputation is when we have a *string.
			// We only get here if we can't keep doing precomputation
			panic("ice: asked to generate member pointer of precomputed " + p.typeTable[in].Rep())
		}

		p.swapStackBoundVars()
		p.ensureInRegister(out)
		effectiveAddress := p.prepareEffectiveAddress(in)
		outReg := p.registerOf(out)
		fieldName := opt.Extra.(string)
		p.issueCommand(fmt.Sprintf("mov %s, qword %s", outReg.qwordName, effectiveAddress))
		switch fieldName {
		case "data":
			p.issueCommand(fmt.Sprintf("add %s, 8", outReg.qwordName))
		case "length":
			// string is a pointer to the length, so we are done.
		}
	case ir.PeelStruct:
		in := opt.In()
		out := opt.Out()
		if !p.valueKnown(in) {
			panic("ice: ask to partial generate an ir.PeelStruct but input is not known")
		}
		pointer, isPointer := p.typeTable[in].(typing.Pointer)
		if !isPointer {
			// The only case where we can't keep doing precomputation is when we need to dereference
			panic("ice: precompute-generate peeling a non pointer. Can only handle precomputed pointers")
		}
		fieldName := opt.Extra.(string)
		record := pointer.ToWhat.(*typing.StructRecord)
		member := record.Members[fieldName]
		if _, memberIsPointer := member.Type.(typing.Pointer); !memberIsPointer {
			panic("ice: genInstPartialKnown asked to peel a struct by doing anything but dereferencing")
		}
		offset := member.Offset
		outReg := p.ensureInRegister(out)
		effectiveAddress := p.prepareEffectiveAddressWithOffset(in, offset)
		p.issueCommand(fmt.Sprintf("mov %s, qword %s", p.registers.all[outReg].qwordName, effectiveAddress))
	default:
		println(opt.GeneratedFrom.GetLineNumber())
		panic("ice: unknown inst trying to use a precomputed value " + opt.Type.String())
	}
}

func (p *procGen) genInstAllRuntimeVars(optIdx int, opt ir.Inst) {
	switch opt.Type {
	case ir.Assign:
		p.varVarCopy(opt.Left(), opt.Right())
	case ir.Add, ir.Sub:
		var leaFormatString string
		var mnemonic string
		if opt.Type == ir.Add {
			leaFormatString = "lea %[1]s, [%[1]s+%[2]s*%[3]d]"
			mnemonic = "add"
		} else {
			leaFormatString = "lea %[1]s, [%[1]s-%[2]s*%[3]d]"
			mnemonic = "sub"
		}
		pointer, leftIsPointer := p.typeTable[opt.Left()].(typing.Pointer)
		rRegId := p.ensureInRegister(opt.Right())
		rReg := p.registerOf(opt.Right())

		if leftIsPointer {
			lRegIdx := p.ensureInRegister(opt.Left())
			pointedToSize := pointer.ToWhat.Size()
			switch pointedToSize {
			case 1, 2, 4, 8:
				p.issueCommand(
					fmt.Sprintf(leaFormatString, p.registers.all[lRegIdx].qwordName, rReg.qwordName, pointedToSize))
			default:
				tempReg := p.findOrMakeFreeReg()
				p.movRegReg(tempReg, rRegId)
				tempRegName := p.registers.all[tempReg].qwordName
				p.issueCommand(fmt.Sprintf("imul %s, %d", tempRegName, pointedToSize))
				p.issueCommand(fmt.Sprintf("%s %s, %s", mnemonic, p.registerOf(opt.Left()).qwordName, tempRegName))
			}
		} else {
			rRegLeftSize := p.signOrZeroExtendIfNeeded(opt.Right(), opt.Left())
			if p.inRegister(opt.Left()) {
				// we sign extend right above in case sizeof(left) > sizeof(right)
				// in case that sizeof(left) <= sizeof(right) we don't need to do anything extra
				// since the truncation/modding happens naturally
				p.issueCommand(fmt.Sprintf("%s %s, %s", mnemonic, p.fittingRegisterName(opt.Left()), rRegLeftSize))
			} else {
				p.memRegCommand(mnemonic, opt.Left(), opt.Right())
			}
		}
	case ir.Increment:
		p.issueCommand(fmt.Sprintf("inc %s", p.varOperand(opt.MutateOperand)))
	case ir.Decrement:
		p.issueCommand(fmt.Sprintf("dec %s", p.varOperand(opt.MutateOperand)))
	case ir.Mult:
		l := opt.Left()
		r := opt.Right()
		p.loadRegisterWithVar(rax, l)
		p.dontSwap[rax] = true
		if p.sizeof(l) > p.sizeof(r) {
			p.ensureInRegister(r)
			p.signOrZeroExtendIfNeeded(r, l)
		}
		if p.sizeof(l) == 1 {
			// we have to bring r to a register to do a 8 bit multiply
			p.ensureInRegister(r)
			p.issueCommand(fmt.Sprintf("imul %s", p.registerOf(r).byteName))
		} else if p.inRegister(r) {
			p.regRegCommandSizedToFirst("imul", l, r)
		} else {
			p.regMemCommand("imul", l, r)
		}
	case ir.Div:
		l := opt.Left()
		r := opt.Right()

		p.loadRegisterWithVar(rax, l)
		p.freeUpRegisters(true, rdx)
		p.issueCommand("xor rdx, rdx")
		needSignExtension := p.sizeof(l) > p.sizeof(r)
		if !p.inRegister(r) && needSignExtension {
			p.loadRegisterWithVar(r8, r) // got to bring it into register to do sign extension
		}
		if p.inRegister(r) && p.varStorage[r].currentRegister != rdx {
			rRegLeftSize := p.signOrZeroExtendIfNeeded(r, l)
			p.issueCommand(fmt.Sprintf("idiv %s", rRegLeftSize))
		} else {
			if !p.hasStackStorage(r) {
				panic("operand to div doens't have stack offset nor is it in register. Where is the value?")
			}
			p.issueCommand(fmt.Sprintf("idiv %s", p.stackOperand(r)))
		}
	case ir.JumpIfFalse, ir.ShortJumpIfFalse, ir.ShortJumpIfTrue, ir.JumpIfTrue:
		label := opt.Extra.(string)
		in := opt.In()

		if p.inRegister(in) {
			regName := p.fittingRegisterName(in)
			p.issueCommand(fmt.Sprintf("cmp %s, 0", regName))
		} else {
			p.issueCommand(fmt.Sprintf("cmp %s, 0", p.stackOperand(in)))
		}
		_, labelSeen := p.labelToState[label]
		if labelSeen {
			p.conditionalJump(opt)
		} else {
			p.conditionalJumps = append(p.conditionalJumps, preJumpState{
				out:    p.out,
				state:  p.copyVarState(),
				optIdx: optIdx})
			p.switchToNewOutBlock()
		}
	case ir.Jump:
		p.jumpOrDelayedJump(optIdx, &opt)
	case ir.Label:
		label := opt.Extra.(string)
		fmt.Fprintf(p.out.buffer, ".%s:\n", label)
		if _, alreadyThere := p.labelToState[label]; alreadyThere {
			panic("ice: same label issued twice")
		}
		if optIdx > 0 && p.block.Opts[optIdx-1].Type == ir.LoopEnd {
			length := len(p.preLoopVarState)
			preLoopState := p.preLoopVarState[length-1]
			p.preLoopVarState = p.preLoopVarState[:length-1]
			p.labelToState[label] = preLoopState
			// Since we always jump to this label and never reach it in normal execution order,
			// this is safe to do. (There is a jmp loop_head at the end of every loop) Everyone
			// who jumps to here will morph into the var state we had before we entered the loop
			p.fullVarState = preLoopState.copyVarState()
		} else {
			p.labelToState[label] = p.copyVarState()
		}
	case ir.StartProc:
		fmt.Fprintf(p.out.buffer, "proc_%s:\n", opt.Extra.(string))
		p.issueCommand("push rbp")
		for _, reg := range preservedRegisters {
			p.issueCommand(fmt.Sprintf("push %s", p.registers.all[reg].qwordName))
		}
		p.issueCommand("mov rbp, rsp")
		if p.callerProvidesReturnSpace {
			p.issueCommand("push rdi")
			p.currentFrameSize += 8
		}
		p.prologueBlock = p.out
		p.switchToNewOutBlock()
	case ir.EndProc:
		fmt.Fprintln(p.out.buffer, ".end_of_proc:")
		p.issueCommand("mov rsp, rbp")
		for i := len(preservedRegisters) - 1; i >= 0; i-- {
			reg := preservedRegisters[i]
			p.issueCommand(fmt.Sprintf("pop %s", p.registers.all[reg].qwordName))
		}
		p.issueCommand("pop rbp")
		p.issueCommand("ret")
		framesize := p.currentFrameSize
		if effectiveFrameSize := framesize + 8*len(preservedRegisters); effectiveFrameSize%16 != 0 {
			// align the stack for SystemV abi. Upon being called, we are 8 bytes misaligned.
			// Since we push rbp in our prologue we align to 16 here
			framesize += 16 - effectiveFrameSize%16
		}
		fmt.Fprintf(p.prologueBlock.buffer, "\tsub rsp, %d\n", framesize)
	case ir.Compare:
		extra := opt.Extra.(ir.CompareExtra)
		out := extra.Out
		l := opt.In()
		r := extra.Right
		lt := p.typeTable[l]
		rt := p.typeTable[r]
		if ls := lt.Size(); !(ls == 8 || ls == 4 || ls == 1) {
			// array & struct compare
			panic("Not yet")
		}

		var firstOperand, secondOperand string
		if lt.Size() != rt.Size() {
			if lt.IsNumber() && rt.IsNumber() {
				p.ensureInRegister(l)
				p.ensureInRegister(r)
				if lt.Size() < rt.Size() {
					firstOperand = p.signOrZeroExtendIfNeeded(l, r)
					secondOperand = p.fittingRegisterName(r)
				} else {
					firstOperand = p.fittingRegisterName(l)
					secondOperand = p.signOrZeroExtendIfNeeded(r, l)
				}
			} else {
				panic("faulty ir: comparison between non numbers with different sizes")
			}
		} else {
			if !p.inRegister(l) && !p.inRegister(r) {
				p.ensureInRegister(l)
			}
			firstOperand = p.varOperand(l)
			secondOperand = p.varOperand(r)
		}

		p.issueCommand(fmt.Sprintf("cmp %s, %s", firstOperand, secondOperand))
		p.allocateRuntimeStorage(out)
		p.setccToVar(extra.How, out)
	case ir.Transclude:
		panic("ice: Transcludes should be gone by now")
	case ir.ArrayToPointer, ir.StructMemberPtr:
		panic("ice: " + opt.Type.String() + " should always be a compile time operation")
	case ir.IndirectLoad, ir.IndirectWrite:
		p.swapStackBoundVars()
		_, isDataMemberOfString := p.typeTable[opt.In()].(typing.StringDataPointer)
		if opt.Type == ir.IndirectLoad && isDataMemberOfString {
			p.varVarCopy(opt.Out(), opt.In())
			break
		}

		var ptr, data int
		var commandTemplate string
		if opt.Type == ir.IndirectLoad {
			ptr = opt.In()
			data = opt.Out()
			commandTemplate = "mov %s, %s [%s]"
		} else {
			ptr = opt.Left()
			data = opt.Right()
			commandTemplate = "mov %[2]s [%[3]s], %[1]s"
		}
		pointedToSize := p.typeTable[ptr].(typing.Pointer).ToWhat.Size()
		p.ensureInRegister(ptr)

		if p.varPerfectRegSize(data) {
			p.ensureInRegister(data)
			dataRegName := p.registerOf(data).nameForSize(pointedToSize)
			prefix := prefixForSize(pointedToSize)
			ptrRegName := p.registerOf(ptr).qwordName
			p.issueCommand(fmt.Sprintf(commandTemplate, dataRegName, prefix, ptrRegName))
		} else {
			if p.sizeof(data) != pointedToSize {
				panic("indirect read/write with inconsistent sizes")
			}
			memcpy := func(dataDest registerId, ptrDest registerId) {
				p.freeUpRegisters(true, rsi, rdi, rcx)
				p.ensureStackOffsetValid(data)
				p.loadVarOffsetIntoReg(data, dataDest)
				p.issueCommand(fmt.Sprintf("mov %s, %s", p.registers.all[ptrDest].qwordName, p.varOperand(ptr)))
				p.issueCommand(fmt.Sprintf("mov rcx, %d", pointedToSize))
				p.issueCommand("call _intrinsic_memcpy")
			}
			if opt.Type == ir.IndirectLoad {
				memcpy(rdi, rsi)
			} else {
				memcpy(rsi, rdi)
			}
		}
	case ir.PeelStruct:
		// this instruction is an artifact of the fact that the frontend doesn't have type information when generating ir.
		// every intermediate dot use this instruction to deal with both auto dereferencing and struct nesting.
		in := opt.In()
		out := opt.Out()
		p.ensureInRegister(out)
		outReg := p.registerOf(out)
		baseType := p.typeTable[in]
		fieldName := opt.Extra.(string)
		switch baseType := baseType.(type) {
		case typing.Pointer:
			p.ensureInRegister(in)
			inReg := p.registerOf(in)
			record := baseType.ToWhat.(*typing.StructRecord)
			memberOffset := record.Members[fieldName].Offset
			_, memberIsPointer := record.Members[fieldName].Type.(typing.Pointer)
			if memberIsPointer {
				p.issueCommand(fmt.Sprintf("mov %s, qword [%s+%d]", outReg.qwordName, inReg.qwordName, memberOffset))
			} else {
				p.issueCommand(fmt.Sprintf("lea %s, [%s+%d]", outReg.qwordName, inReg.qwordName, memberOffset))
			}
		case *typing.StructRecord:
			memberOffset := baseType.Members[fieldName].Offset
			_, memberIsPointer := baseType.Members[fieldName].Type.(typing.Pointer)
			p.ensureStackOffsetValid(in)
			if memberIsPointer {
				p.issueCommand(fmt.Sprintf("mov %s, qword [rbp-%d+%d]", outReg.qwordName, p.varStorage[in].rbpOffset, memberOffset))
			} else {
				p.issueCommand(fmt.Sprintf("lea %s, [rbp-%d+%d]", outReg.qwordName, p.varStorage[in].rbpOffset, memberOffset))
			}
		}
	case ir.Not:
		setLabel := p.genLabel(".keep_zero")
		out := opt.Out()
		in := opt.In()
		p.ensureInRegister(out)
		p.issueCommand(fmt.Sprintf("cmp %s, 0", p.varOperand(in)))
		p.issueCommand(fmt.Sprintf("setz %s", p.fittingRegisterName(out)))
		fmt.Fprintf(p.out.buffer, "%s:\n", setLabel)
	case ir.And:
		l := opt.Left()
		r := opt.Right()
		if !p.inRegister(l) && !p.inRegister(r) {
			p.ensureInRegister(l)
		}
		p.issueCommand(fmt.Sprintf("and %s, %s", p.varOperand(l), p.varOperand(r)))
	case ir.Or:
		l := opt.Left()
		r := opt.Right()
		if !p.inRegister(l) && !p.inRegister(r) {
			p.ensureInRegister(l)
		}
		p.issueCommand(fmt.Sprintf("or %s, %s", p.varOperand(l), p.varOperand(r)))
	default:
		panic(opt)
	}
}

func (p *procGen) trace(vn int) {
	if vn >= len(p.varStorage) {
		return
	}
	if p.valueKnown(vn) {
		fmt.Printf("var %d has compile time value %d\n", vn, p.getPrecomputedValue(vn))
		if p.precompute[vn].valueType == pointerRelativeToVar {
			rel := p.relativePointers[p.precompute[vn].value]
			fmt.Printf("    relative to %d, offset %d\n", rel.baseVar, rel.offset)
		}
	} else {
		info := p.varInfoString(vn)
		fmt.Fprintf(p.out.buffer, "\t\t\t;%s\n", info)
		fmt.Println(info)
	}
}

func backendDebug(framesize int, typeTable []typing.TypeRecord) {
	for i, typeRecord := range typeTable {
		fmt.Printf("var %d type: %#v\n", i, typeRecord)
	}
}

// return an array where the ith element has the index of the inst in which vn=i is last used
func findLastusage(block frontend.OptBlock) []int {
	lastUse := make([]int, block.NumberOfVars)
	for i := len(block.Opts) - 1; i >= 0; i-- {
		recordUsage := func(vn int) {
			if lastUse[vn] == 0 { // the 0th instruction is always startproc, which doesn't use any var
				lastUse[vn] = i
			}
		}
		ir.IterOverAllVars(block.Opts[i], recordUsage)
	}

	// adjust for jumpbacks and indirect use of variable
	labelToIdx := make(map[string]int)
	for i, opt := range block.Opts {
		if opt.Type == ir.Label {
			labelToIdx[opt.Extra.(string)] = i
		}
	}
	for i := len(block.Opts) - 1; i >= 0; i-- {
		opt := block.Opts[i]
		switch opt.Type {
		case ir.JumpIfTrue, ir.JumpIfFalse, ir.Jump:
			label := opt.Extra.(string)
			for j := labelToIdx[label]; j < i; j++ {
				ir.IterOverAllVars(block.Opts[j], func(vn int) {
					if lastUse[vn] < i {
						lastUse[vn] = i
					}
				})
			}
		}
	}

	for _, opt := range block.Opts {
		if opt.Type == ir.TakeAddress {
			lastUse[opt.In()] = lastUse[opt.Out()]
		}
	}

	return lastUse
}

func findConstantVars(block frontend.OptBlock) []bool {
	mutatedOnce := make([]bool, block.NumberOfVars)
	constant := make([]bool, block.NumberOfVars)
	for i := range constant {
		constant[i] = true
	}
	for _, opt := range block.Opts {
		mut := ir.FindMutationVar(&opt)
		if mut < 0 {
			continue
		}
		if mutatedOnce[mut] {
			constant[mut] = false
		}
		mutatedOnce[mut] = true
	}
	return constant
}

func findWhenToStopPrecomputation(block frontend.OptBlock) []int {
	stop := make([]int, block.NumberOfVars)
	for i := range stop {
		stop[i] = len(block.Opts)
	}
	for i, opt := range block.Opts {
		if opt.Type == ir.OutsideLoopMutations {
			for _, mut := range *opt.Extra.(*[]int) {
				if mut >= 0 && i < stop[mut] {
					stop[mut] = i
				}
			}
		}
	}
	return stop
}

func collectOutput(firstBlock *outputBlock, out io.Writer) {
	outBlock := firstBlock
	for outBlock != nil {
		_, err := outBlock.buffer.WriteTo(out)
		if err != nil {
			panic(err)
		}
		outBlock = outBlock.next
	}
}

func X86ForBlock(out io.Writer, block frontend.OptBlock, typeTable []typing.TypeRecord, globalEnv *typing.EnvRecord, typer *typing.Typer, procRecord typing.ProcRecord) *bytes.Buffer {
	firstOut := newOutputBlock()
	var staticDataBuf bytes.Buffer
	gen := procGen{
		fullVarState:              newFullVarState(block.NumberOfVars),
		out:                       firstOut,
		firstOutputBlock:          firstOut,
		block:                     block,
		typeTable:                 typeTable,
		env:                       globalEnv,
		typer:                     typer,
		staticDataBuf:             &staticDataBuf,
		labelToState:              make(map[string]*fullVarState),
		lastUsage:                 findLastusage(block),
		procRecord:                procRecord,
		precompute:                make([]precomputeInfo, block.NumberOfVars),
		callerProvidesReturnSpace: (*(procRecord.Return)).Size() > 16,
	}
	constantVars := findConstantVars(block)
	stopPrecomputation := findWhenToStopPrecomputation(block)
	gen.constantVars = constantVars
	gen.stopPrecomputation = stopPrecomputation
	gen.generate()
	collectOutput(gen.firstOutputBlock, out)
	return &staticDataBuf
}
