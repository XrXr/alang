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
		return fmt.Sprintf("variable %d is in %s", vn, f.registers.all[vn].qwordName)
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
	stackBoundVars            []int
	// info for backfilling instructions
	prologueBlock    *outputBlock
	conditionalJumps []preJumpState
	jumps            []preJumpState
	labelToState     map[string]*fullVarState
	// all three below are vn-indexed
	typeTable []typing.TypeRecord
	lastUsage []int
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

func (p *procGen) morphToState(targetState *fullVarState) {
	backup := p.fullVarState.copyVarState()
	p.noNewStackStorage = true
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
			} else if targetState.hasStackStorage(ourOccupiedBy) {
				p.varStorage[ourOccupiedBy].rbpOffset = targetState.varStorage[ourOccupiedBy].rbpOffset
				p.memRegCommand("mov", ourOccupiedBy, ourOccupiedBy)
				p.releaseRegister(registerId(regId))
			} else {
				panic("this should be exhaustive")
			}
		}
		if theirOccupiedBy != invalidVn {
			p.loadRegisterWithVar(registerId(regId), theirOccupiedBy)
		}
	}
	p.noNewStackStorage = false
	p.fullVarState = backup
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

func (p *procGen) perfectRegSize(vn int) bool {
	size := p.sizeof(vn)
	return size == 8 || size == 4 || size == 2 || size == 1
}

func (p *procGen) signOrZeroExtendMov(dest int, source int) {
	// caller is responsible for making sure that both vars are in register
	destReg := p.registerOf(dest)
	sourceTightFit := p.fittingRegisterName(source)
	destRegName := destReg.qwordName
	var mnemonic string
	if p.typer.IsUnsigned(p.typeTable[source]) {
		if p.sizeof(source) == 4 {
			mnemonic = "mov" // upper 4 bytes are automatically zeroed
			destRegName = destReg.nameForSize(4)
		} else {
			mnemonic = "movzx"
		}
	} else {
		mnemonic = "movsx"
	}
	p.issueCommand(fmt.Sprintf("%s %s, %s", mnemonic, destRegName, sourceTightFit))
}

func (p *procGen) varVarCopy(dest int, source int) {
	if p.perfectRegSize(source) {
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
	nojump := p.genLabel(".nojump")
	if jumpInst.Type == ir.JumpIfFalse {
		p.issueCommand(fmt.Sprintf("jnz %s", nojump))
	} else if jumpInst.Type == ir.JumpIfTrue {
		p.issueCommand(fmt.Sprintf("jz %s", nojump))
	}
	p.morphToState(targetState)
	p.issueCommand(fmt.Sprintf("jmp .%s", label))
	fmt.Fprintf(p.out.buffer, "%s:\n", nojump)
}

func (p *procGen) jump(jumpInst ir.Inst) {
	label := jumpInst.Extra.(string)
	targetState := p.labelToState[label]
	p.morphToState(targetState)
	p.issueCommand(fmt.Sprintf("jmp .%s", label))
}

func (p *procGen) genLabel(prefix string) string {
	label := fmt.Sprintf("%s_%d", prefix, p.nextLabelId)
	p.nextLabelId++
	return label
}

func (p *procGen) generate() {
	addLine := func(line string) {
		io.WriteString(p.out.buffer, line)
	}

	paramPassingRegOrder := [...]registerId{rdi, rsi, rdx, rcx, r8, r9}
	preservedRegisters := [...]registerId{rbx, r15, r14, r13, r12}

	{
		paramOffset := -16
		for i := 0; i < p.block.NumberOfArgs; i++ {
			regOrder := i
			if p.callerProvidesReturnSpace {
				regOrder += 1
			}
			if regOrder < len(paramPassingRegOrder) {
				p.loadRegisterWithVar(paramPassingRegOrder[regOrder], i)
			} else {
				p.varStorage[i].rbpOffset = paramOffset
				paramOffset -= p.sizeof(i)
			}
		}
	}

	// backendDebug(framesize, p.typeTable)
	for optIdx, opt := range p.block.Opts {
		addLine(fmt.Sprintf(".ir_line_%d:\n", optIdx))
		for i := 0; i < len(p.dontSwap); i++ {
			p.dontSwap[i] = false
		}
		switch opt.Type {
		case ir.Assign:
			p.varVarCopy(opt.Left(), opt.Right())
		case ir.AssignImm:
			dst := opt.Out()
			switch value := opt.Extra.(type) {
			case int64:
				p.ensureInRegister(dst)
				p.regImmCommand("mov", dst, value)
			case uint64:
				p.ensureInRegister(dst)
				p.issueCommand(fmt.Sprintf("mov %s, %d", p.registerOf(dst).qwordName, value))
			case bool:
				p.ensureInRegister(dst)
				var val int64 = 0
				if value == true {
					val = 1
				}
				p.regImmCommand("mov", dst, val)
			case string:
				destReg := p.ensureInRegister(dst)
				labelName := p.genLabel(fmt.Sprintf("static_string_%p", p.staticDataBuf))
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
				if !isStruct && !isArray && p.perfectRegSize(out) && freeRegExists {
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
					tempReg, freeRegExists := p.registers.nextAvailable()
					var tempRegTenant int
					if !freeRegExists {
						tempReg = rRegId
						for tempReg == lRegIdx || tempReg == rRegId {
							tempReg = (tempReg + 1) % numRegisters
						}
						tempRegTenant = p.registers.all[tempReg].occupiedBy
						p.ensureStackOffsetValid(tempRegTenant)
						p.memRegCommand("mov", tempRegTenant, tempRegTenant)
					}
					p.movRegReg(tempReg, rRegId)
					tempRegName := p.registers.all[tempReg].qwordName
					p.issueCommand(fmt.Sprintf("imul %s, %d", tempRegName, pointedToSize))
					p.issueCommand(fmt.Sprintf("%s %s, %s", mnemonic, p.registerOf(opt.Left()).qwordName, tempRegName))
					if !freeRegExists {
						p.regMemCommand("mov", tempRegTenant, tempRegTenant)
					}
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
			rdxTenant := p.registers.all[rdx].occupiedBy
			var borrowedAReg bool
			var regBorrowed registerId
			if rdxTenant != invalidVn {
				if regBorrowed, borrowedAReg = p.registers.nextAvailable(); borrowedAReg {
					p.movRegReg(regBorrowed, rdx)
				} else {
					p.ensureStackOffsetValid(rdxTenant)
					p.memRegCommand("mov", rdxTenant, rdxTenant)
				}
			}
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
			if rdxTenant != invalidVn {
				if borrowedAReg {
					p.movRegReg(rdx, regBorrowed)
				} else {
					p.regMemCommand("mov", rdxTenant, rdxTenant)
				}
			}
		case ir.JumpIfFalse, ir.JumpIfTrue:
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
		case ir.Call:
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
					p.varVarCopy(opt.Out(), extra.ArgVars[0])
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

				argsPassed := 0
				for i, arg := range extra.ArgVars {
					if provideReturnStorage {
						i += 1
					}
					if i >= len(paramPassingRegOrder) {
						break
					}
					argsPassed++
					switch p.typeTable[arg].Size() {
					case 8, 4, 2, 1:
						p.loadRegisterWithVar(paramPassingRegOrder[i], arg)
					default:
						panic("Unsupported parameter size")
					}
				}

				if argsPassed < len(extra.ArgVars) {
					numStackVars = len(extra.ArgVars) - argsPassed
					if numStackVars%2 == 1 {
						// Make sure we are aligned to 16
						p.issueCommand("sub rsp, 8")
					}
					for i := len(extra.ArgVars) - 1; i >= len(extra.ArgVars)-numStackVars; i-- {
						arg := extra.ArgVars[i]
						argSize := p.typeTable[arg].Size()
						switch argSize {
						case 8, 4, 2, 1:
							if p.inRegister(arg) {
								p.issueCommand(fmt.Sprintf("push %s", p.registerOf(arg).qwordName))
							} else {
								p.issueCommand(fmt.Sprintf("push %s", p.stackOperand(arg)))
								if argSize < 8 {
									// each param is rounded to 8 bytes
									p.issueCommand(fmt.Sprintf("sub esp, %d", 8-argSize))
								}
							}
						default:
							panic("Unsupported parameter size")
						}
					}
				}

				// the first part of this array is the same as paramPassingRegOrder
				regsThatGetDestroyed := [...]registerId{rdi, rsi, rdx, rcx, r8, r9, rax, r10, r11}
				for _, reg := range regsThatGetDestroyed {
					owner := p.registers.all[reg].occupiedBy
					if owner != invalidVn {
						if p.lastUsage[owner] != optIdx {
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
		case ir.Jump:
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
		case ir.Label:
			label := opt.Extra.(string)
			addLine(fmt.Sprintf(".%s:\n", label))
			if _, alreadyThere := p.labelToState[label]; alreadyThere {
				panic("same label issued twice")
			}
			p.labelToState[label] = p.copyVarState()
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

			outReg, outInReg := p.registers.nextAvailable()
			if outInReg {
				p.loadRegisterWithVar(outReg, out)
				p.issueCommand(fmt.Sprintf("mov %s, 1", p.fittingRegisterName(out)))
			} else {
				p.ensureStackOffsetValid(out)
				p.issueCommand(fmt.Sprintf("mov %s, 1", p.stackOperand(out)))
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
					panic("faulty ir: comparsion between non numbers with different sizes")
				}
			} else {
				// TODO: autoCommand() we can have a method that gives an operand preferring register.
				// Not sure we do it in other places yet though.
				if !p.inRegister(l) && !p.inRegister(r) {
					p.ensureInRegister(l)
				}
				firstOperand = p.varOperand(l)
				secondOperand = p.varOperand(r)
			}

			p.issueCommand(fmt.Sprintf("cmp %s, %s", firstOperand, secondOperand))
			labelName := p.genLabel(".cmp")
			switch extra.How {
			case ir.Greater:
				p.issueCommand(fmt.Sprintf("jg %s", labelName))
			case ir.Lesser:
				p.issueCommand(fmt.Sprintf("jl %s", labelName))
			case ir.GreaterOrEqual:
				p.issueCommand(fmt.Sprintf("jge %s", labelName))
			case ir.LesserOrEqual:
				p.issueCommand(fmt.Sprintf("jle %s", labelName))
			case ir.AreEqual:
				p.issueCommand(fmt.Sprintf("je %s", labelName))
			case ir.NotEqual:
				p.issueCommand(fmt.Sprintf("jne %s", labelName))
			}

			p.issueCommand(fmt.Sprintf("mov %s, 0", p.varOperand(out)))
			fmt.Fprintf(p.out.buffer, "%s:\n", labelName)
		case ir.Transclude:
			panic("Transcludes should be gone by now")
		case ir.TakeAddress:
			in := opt.In()
			out := opt.Out()
			p.ensureStackOffsetValid(in)
			outReg := p.ensureInRegister(out)
			p.loadVarOffsetIntoReg(in, outReg)
			p.stackBoundVars = append(p.stackBoundVars, in)
		case ir.ArrayToPointer:
			in := opt.In()
			out := opt.Out()
			p.ensureStackOffsetValid(in)
			p.ensureInRegister(out)
			switch p.typeTable[opt.In()].(type) {
			case typing.Array:
				p.issueCommand(fmt.Sprintf("lea %s, [rbp-%d]", p.registerOf(out).qwordName, p.varStorage[in].rbpOffset))
			case typing.Pointer:
				p.issueCommand(fmt.Sprintf("mov %s, %s", p.fittingRegisterName(out), p.varOperand(in)))
			default:
				panic("must be array or pointer to an array")
			}
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

			if p.perfectRegSize(data) {
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
		case ir.StructMemberPtr:
			out := opt.Out()
			in := opt.In()
			p.ensureInRegister(out)
			outReg := p.registerOf(out)
			baseType := p.typeTable[opt.In()]
			fieldName := opt.Extra.(string)
			switch baseType := baseType.(type) {
			case typing.Pointer:
				switch baseType.ToWhat.(type) {
				case typing.String:
					p.swapStackBoundVars()
					p.ensureInRegister(out)
					p.ensureInRegister(in)
					switch fieldName {
					case "data":
						p.issueCommand(fmt.Sprintf("mov %s, qword [%s]", outReg.qwordName, p.registerOf(in).qwordName))
						p.issueCommand(fmt.Sprintf("add %s, 8", outReg.qwordName))
					case "length":
						p.issueCommand(fmt.Sprintf("mov %s, qword [%s]", outReg.qwordName, p.registerOf(in).qwordName))
					}
				default:
					p.ensureInRegister(in)
					record := baseType.ToWhat.(*typing.StructRecord)
					p.issueCommand(fmt.Sprintf("lea %s, [%s+%d]",
						outReg.qwordName, p.registerOf(in).qwordName, record.Members[fieldName].Offset))
				}
			case *typing.StructRecord:
				p.ensureStackOffsetValid(in)
				p.issueCommand(fmt.Sprintf("lea %s, [rbp-%d+%d]",
					outReg.qwordName, p.varStorage[in].rbpOffset, baseType.Members[fieldName].Offset))
			case typing.String:
				p.ensureInRegister(in)
				switch fieldName {
				case "data":
					p.ensureInRegister(in)
					p.issueCommand(fmt.Sprintf("lea %s, [%s+8]", outReg.qwordName, p.registerOf(in).qwordName))
				case "length":
					p.movRegReg(p.varStorage[out].currentRegister, p.varStorage[in].currentRegister)
				}
			default:
				panic("Type checker didn't do its job")
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
			p.issueCommand(fmt.Sprintf("mov %s, 0", p.fittingRegisterName(out)))
			p.issueCommand(fmt.Sprintf("cmp %s, 0", p.varOperand(in)))
			p.issueCommand(fmt.Sprintf("jnz %s", setLabel))
			p.issueCommand(fmt.Sprintf("mov %s, 1", p.fittingRegisterName(out)))
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
		case ir.Return:
			returnType := *p.procRecord.Return

			returnExtra := opt.Extra.(ir.ReturnExtra)
			if len(returnExtra.Values) > 0 {
				retVar := returnExtra.Values[0]
				if p.perfectRegSize(retVar) {
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
		default:
			panic(opt)
		}

		decommissionIfLastUse := func(vn int) {
			reg := p.varStorage[vn].currentRegister
			if p.lastUsage[vn] == optIdx {
				if reg != invalidRegister {
					p.releaseRegister(reg)
				}
				p.varStorage[vn].decommissioned = true
			}
		}
		ir.IterOverAllVars(opt, decommissionIfLastUse)
		// p.trace(3)
	}

	for _, jump := range p.conditionalJumps {
		p.out = jump.out
		p.fullVarState = jump.state
		p.conditionalJump(p.block.Opts[jump.optIdx])
	}
	for _, jump := range p.jumps {
		p.out = jump.out
		p.fullVarState = jump.state
		p.jump(p.block.Opts[jump.optIdx])
	}
}

func (p *procGen) trace(vn int) {
	if vn >= len(p.varStorage) {
		return
	}
	fmt.Fprintf(p.out.buffer, "\t\t\t;%s\n", p.varInfoString(vn))
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
		callerProvidesReturnSpace: (*(procRecord.Return)).Size() > 16,
	}
	gen.generate()
	collectOutput(gen.firstOutputBlock, out)
	return &staticDataBuf
}
