package frontend

import (
	"github.com/XrXr/alang/ir"
	"github.com/XrXr/alang/parsing"
	"sort"
	"strconv"
)

const undefinedMessage = "Undefined name"

func GenForProc(labelGen *LabelIdGen, order *ProcWorkOrder) {
	var gen procGen
	gen.rootScope = &scope{
		gen:         &gen,
		varTable:    make(map[string]int),
		parentScope: nil,
	}
	gen.labelGen = labelGen

	for i, arg := range order.ProcDecl.Args {
		_ = i
		gen.rootScope.newNamedVar(arg.Name.Name)
	}

	gen.opts = append(gen.opts, ir.Inst{Type: ir.StartProc, Extra: order.Name})
	idxAfter := genForProcSubSection(labelGen, order, gen.rootScope, 0)
	gen.opts = append(gen.opts, ir.Inst{Type: ir.EndProc})

	if idxAfter != len(order.In) {
		parsing.Dump(order.In)
		panic("ice: gen didn't process whole proc")
	}
	if len(gen.nodeStack) != 0 {
		parsing.Dump(gen.nodeStack)
		panic("ice: nodeStack is not empty after generation has finished")
	}
	order.Out <- OptBlock{NumberOfVars: gen.nextVarNum, NumberOfArgs: len(order.ProcDecl.Args), Opts: gen.opts}
	close(order.Out)
}

// return index to the first unprocessed node
func genForProcSubSection(labelGen *LabelIdGen, order *ProcWorkOrder, scope *scope, start int) int {
	gen := scope.gen
	i := start
	sawIf := false
	var optionSelectAllOutOfScopeMutations *[]int
	finishOptionSelect := func() {
		sort.Ints(*optionSelectAllOutOfScopeMutations)
		*optionSelectAllOutOfScopeMutations = DedupSorted(*optionSelectAllOutOfScopeMutations)
		optionSelectAllOutOfScopeMutations = nil
		scope.addOpt(ir.Inst{Type: ir.OptionSelectEnd})
	}

	for i < len(order.In) {
		nodePtr := order.In[i]
		i++
		sawIfLastIter := sawIf
		sawIf = false
		gen.pushCurrentlyGenerating(nodePtr)
		after := func() {
			gen.popCurrentlyGenerating(nodePtr)
		}
		switch node := (*nodePtr).(type) {
		case parsing.BlockEnd:
			if scope.outOfScopeMutations != nil {
				sort.Ints(*scope.outOfScopeMutations)
				*scope.outOfScopeMutations = DedupSorted(*scope.outOfScopeMutations)
				if scope.parentScope != nil && scope.parentScope.outOfScopeMutations != nil {
					for _, vn := range *scope.outOfScopeMutations {
						if vn < scope.parentScope.firstVarInScope {
							*scope.parentScope.outOfScopeMutations = append(*scope.parentScope.outOfScopeMutations, vn)
						}
					}
				}
			}
			after()
			return i
		case parsing.Declaration:
			// these are declaration without values i.e. not foo := 3
			newVar := scope.newNamedVar(node.Name.Name)
			scope.addOpt(ir.MakeMutateOnlyInst(ir.AssignImm, newVar, node.Type))
		case parsing.ExprNode:
			switch node.Op {
			case parsing.Declare:
				varName := node.Left.(parsing.IdName).Name
				if _, alreadyExist := scope.resolve(varName); alreadyExist {
					_, existsInCurrentScope := scope.varTable[varName]
					if existsInCurrentScope {
						panic(parsing.ErrorFromNode(node.Left, "Redeclaration of variable"))
					}
					// we need to generate the right side before we make the variable in the current scope since
					// in this statement the name on the rhs refers to the var in the outer scope.
					rhsResult := genExpressionValue(scope, node.Right)
					vnForNewVar := scope.newNamedVar(varName)
					scope.addOpt(ir.MakeBinaryInst(ir.Assign, vnForNewVar, rhsResult, nil))
				} else {
					varNum := scope.newNamedVar(varName)
					genExpressionValueToVar(scope, varNum, node.Right)
				}
			case parsing.Assign, parsing.PlusEqual, parsing.MinusEqual:
				leftAsIdent, leftIsIdent := node.Left.(parsing.IdName)
				if leftIdent := leftAsIdent.Name; leftIsIdent {
					leftVarNum, varFound := scope.resolve(leftIdent)
					if !varFound {
						panic(parsing.ErrorFromNode(node.Left, undefinedMessage))
					}
					rightResult := genExpressionValue(scope, node.Right)
					switch node.Op {
					case parsing.PlusEqual:
						scope.addOpt(ir.MakeBinaryInst(ir.Add, leftVarNum, rightResult, nil))
					case parsing.MinusEqual:
						scope.addOpt(ir.MakeBinaryInst(ir.Sub, leftVarNum, rightResult, nil))
					default:
						scope.addOpt(ir.MakeBinaryInst(ir.Assign, leftVarNum, rightResult, nil))
					}
				} else {
					assignmentPtr := genAssignmentTarget(scope, node.Left)
					rightResult := genExpressionValue(scope, node.Right)
					var leftTmp int
					if node.Op == parsing.PlusEqual || node.Op == parsing.MinusEqual {
						leftTmp = scope.newVar()
						scope.addOpt(ir.MakeBinaryInst(ir.IndirectLoad, leftTmp, assignmentPtr, nil))
					}
					switch node.Op {
					case parsing.PlusEqual:
						scope.addOpt(ir.MakeBinaryInst(ir.Add, leftTmp, rightResult, nil))
						scope.addOpt(ir.MakeBinaryInst(ir.IndirectWrite, assignmentPtr, leftTmp, nil))
					case parsing.MinusEqual:
						scope.addOpt(ir.MakeBinaryInst(ir.Sub, leftTmp, rightResult, nil))
						scope.addOpt(ir.MakeBinaryInst(ir.IndirectWrite, assignmentPtr, leftTmp, nil))
					default:
						scope.addOpt(ir.MakeBinaryInst(ir.IndirectWrite, assignmentPtr, rightResult, nil))
					}
				}
			default:
				//TODO issue warning here
			}
		case parsing.IfNode:
			sawIf = true
			optionSelectAllOutOfScopeMutations = &[]int{}
			mutations := &[]int{}

			pastBlock := labelGen.GenLabel("else_%d")
			scope := scope.inherit()
			scope.outOfScopeMutations = mutations
			scope.addOpt(ir.Inst{Type: ir.OptionSelectStart, Extra: optionSelectAllOutOfScopeMutations})
			condVar := genExpressionValue(scope, node.Condition)
			scope.addOpt(ir.MakeReadOnlyInst(ir.JumpIfFalse, condVar, pastBlock))
			i = genForProcSubSection(labelGen, order, scope, i)
			scope.addOpt(ir.Inst{Type: ir.OptionEnd, Extra: mutations})
			scope.addOpt(labelInst(pastBlock))
			*optionSelectAllOutOfScopeMutations = append(*optionSelectAllOutOfScopeMutations, *mutations...)
			if _, doingElseNext := (*order.In[i]).(parsing.ElseNode); !doingElseNext {
				finishOptionSelect()
			}
		case parsing.ElseNode:
			if !sawIfLastIter {
				panic("ice: bare else. Should've been caught by the parser")
			}
			endOfTree := labelGen.GenLabel("if_else_end_%d")
			elseLabel := gen.opts[len(gen.opts)-1]
			gen.opts[len(gen.opts)-1] = ir.MakePlainInst(ir.Jump, endOfTree)
			mutations := &[]int{}
			scope := scope.inherit()
			scope.outOfScopeMutations = mutations

			scope.addOpt(elseLabel)
			i = genForProcSubSection(labelGen, order, scope, i)
			scope.addOpt(ir.Inst{Type: ir.OptionEnd, Extra: mutations})
			scope.addOpt(labelInst(endOfTree))
			*optionSelectAllOutOfScopeMutations = append(*optionSelectAllOutOfScopeMutations, *mutations...)
			finishOptionSelect()
		case parsing.Loop:
			outsideLoopMutations := &[]int{}
			loopStart := labelGen.GenLabel("loop_%d")
			loopEnd := loopStart + "_loopEnd"

			var iterationVar, endVar int
			usingRangeExpr := false
			counterIsNamed := false
			var counterVarName string
			var rangeExpr parsing.ExprNode

			if loopExpr, loopExprIsExprNode := node.Expression.(parsing.ExprNode); loopExprIsExprNode {
				switch loopExpr.Op {
				case parsing.Declare:
					usingRangeExpr = true
					// parser gurantee
					rangeExpr = loopExpr.Right.(parsing.ExprNode)
					counterIsNamed = true
					counterVarName = loopExpr.Left.(parsing.IdName).Name
					fallthrough
				case parsing.Range:
					if !usingRangeExpr {
						rangeExpr = loopExpr
					}
					usingRangeExpr = true
					if rangeExpr.Op != parsing.Range {
						panic("parser bug")
					}
					iterationVar = scope.newVar()
					endVar = genExpressionValue(scope, rangeExpr.Right)
					genExpressionValueToVar(scope, iterationVar, rangeExpr.Left)

				}
			}
			// loop body
			scope := scope.inherit()
			scope.loopLabel = loopStart
			scope.outOfScopeMutations = outsideLoopMutations
			scope.addOpt(ir.Inst{Type: ir.OutsideLoopMutations, Extra: outsideLoopMutations})
			scope.addOpt(labelInst(loopStart))

			// exit condition check
			if usingRangeExpr {
				condVar := scope.newVar()
				scope.addOpt(ir.MakeReadOnlyInst(ir.Compare, iterationVar,
					ir.CompareExtra{
						How:   ir.LesserOrEqual,
						Right: endVar,
						Out:   condVar,
					}))
				scope.addOpt(ir.MakeReadOnlyInst(ir.JumpIfFalse, condVar, loopEnd))
				if counterIsNamed {
					namedCounterVn := scope.newNamedVar(counterVarName)
					scope.addOpt(ir.MakeBinaryInst(ir.Assign, namedCounterVn, iterationVar, nil))
				}
			} else {
				if node.Expression != nil {
					condVar := scope.newVar()
					genExpressionValueToVar(scope, condVar, node.Expression)
					scope.addOpt(ir.MakeReadOnlyInst(ir.JumpIfFalse, condVar, loopEnd))
				}
			}

			i = genForProcSubSection(labelGen, order, scope, i)
			// continue code
			scope.addOpt(labelInst(loopStart + "_loopContinue"))
			if usingRangeExpr {
				scope.addOpt(ir.MakeMutateOnlyInst(ir.Increment, iterationVar, nil))
				scope.addOpt(ir.MakePlainInst(ir.Jump, loopStart))
			} else {
				scope.addOpt(ir.MakePlainInst(ir.Jump, loopStart))
			}
			scope.addOpt(labelInst(loopEnd))
		case parsing.ContinueNode:
			if scope.loopLabel == "" {
				panic(parsing.ErrorFromNode(node, "Use of continue outside of a loop"))
			}
			scope.addOpt(ir.MakePlainInst(ir.Jump, scope.loopLabel+"_loopContinue"))
		case parsing.BreakNode:
			if scope.loopLabel == "" {
				panic(parsing.ErrorFromNode(node, "Use of break outside of a loop"))
			}
			scope.addOpt(ir.MakePlainInst(ir.Jump, scope.loopLabel+"_loopEnd"))
		case parsing.ReturnNode:
			var returnValues []int
			for _, valueExpr := range node.Values {
				retVar := genExpressionValue(scope, valueExpr)
				returnValues = append(returnValues, retVar)
			}
			scope.addOpt(ir.Inst{Type: ir.Return, Extra: ir.ReturnExtra{returnValues}})
		default:
			genExpressionValue(scope, node)
		}
		after()
	}
	panic("ice: genForProcSubSection unable to find a EndBlock node")
	return -1
}

// given an ast node, generate ir that computes its value. Returns the variable number which holds said value.
func genExpressionValue(scope *scope, node parsing.ASTNode) int {
	switch n := node.(type) {
	case parsing.IdName:
		vn, found := scope.resolve(n.Name)
		if !found {
			panic(parsing.ErrorFromNode(n, undefinedMessage))
		}
		return vn
	default:
		vn := scope.newVar()
		genExpressionValueToVar(scope, vn, node)
		return vn
	}
}

// same as genExpressionValue except the caller decides where the value goes
func genExpressionValueToVar(scope *scope, dest int, node parsing.ASTNode) {
	gen := scope.gen
	labelGen := gen.labelGen
	gen.pushCurrentlyGenerating(&node)
	defer gen.popCurrentlyGenerating(&node)
	switch n := node.(type) {
	case parsing.IdName:
		vn, found := scope.resolve(n.Name)
		if !found {
			panic(parsing.ErrorFromNode(n, undefinedMessage))
		}
		scope.addOpt(ir.MakeBinaryInst(ir.Assign, dest, vn, nil))
	case parsing.Literal:
		var value interface{}
		switch n.Type {
		case parsing.Number:
			v, err := strconv.ParseInt(n.Value, 10, 64)
			if err == nil {
				value = v
			} else {
				value, err = strconv.ParseUint(n.Value, 10, 64)
			}
		case parsing.Boolean:
			value = boolStrToBool(n.Value)
		case parsing.String:
			value = n.Value
		case parsing.NilPtr:
			value = parsing.NilPtr
		}
		scope.addOpt(ir.MakeMutateOnlyInst(ir.AssignImm, dest, value))
	case parsing.ProcCall:
		var argVars []int
		for _, argNode := range n.Args {
			argEval := genExpressionValue(scope, argNode)
			argVars = append(argVars, argEval)
		}
		scope.addOpt(ir.MakeMutateOnlyInst(ir.Call, dest, ir.CallExtra{
			Name:    n.Callee.Name,
			ArgVars: argVars,
		}))
	case parsing.ExprNode:
		switch n.Op {
		case parsing.Dereference:
			if n.Left != nil {
				panic("parser bug")
			}
			rightDest := genExpressionValue(scope, n.Right)
			scope.addOpt(ir.MakeBinaryInst(ir.IndirectLoad, dest, rightDest, nil))
		case parsing.LogicalNot:
			if n.Left != nil {
				panic("parser bug")
			}
			rightDest := genExpressionValue(scope, n.Right)
			scope.addOpt(ir.MakeBinaryInst(ir.Not, dest, rightDest, nil))
		case parsing.LogicalAnd:
			end := labelGen.GenLabel("andEnd_%d")
			genExpressionValueToVar(scope, dest, n.Left)
			scope.addOpt(ir.MakeReadOnlyInst(ir.ShortJumpIfFalse, dest, end))
			rightDest := genExpressionValue(scope, n.Right)
			scope.addOpt(ir.MakeBinaryInst(ir.And, dest, rightDest, nil))
			scope.addOpt(labelInst(end))
		case parsing.LogicalOr:
			end := labelGen.GenLabel("orEnd_%d")
			genExpressionValueToVar(scope, dest, n.Left)
			scope.addOpt(ir.MakeReadOnlyInst(ir.ShortJumpIfTrue, dest, end))
			rightDest := genExpressionValue(scope, n.Right)
			scope.addOpt(ir.MakeBinaryInst(ir.Or, dest, rightDest, nil))
			scope.addOpt(labelInst(end))
		case parsing.Star, parsing.Minus, parsing.Plus, parsing.Divide,
			parsing.Greater, parsing.GreaterEqual, parsing.Lesser,
			parsing.LesserEqual, parsing.DoubleEqual, parsing.BangEqual:
			leftDest := scope.newVar()
			genExpressionValueToVar(scope, leftDest, n.Left)
			rightDest := genExpressionValue(scope, n.Right)
			switch n.Op {
			case parsing.Star:
				scope.addOpt(ir.MakeBinaryInst(ir.Mult, leftDest, rightDest, nil))
				scope.addOpt(ir.MakeBinaryInst(ir.Assign, dest, leftDest, nil))
			case parsing.Divide:
				scope.addOpt(ir.MakeBinaryInst(ir.Div, leftDest, rightDest, nil))
				scope.addOpt(ir.MakeBinaryInst(ir.Assign, dest, leftDest, nil))
			case parsing.Plus:
				scope.addOpt(ir.MakeBinaryInst(ir.Add, leftDest, rightDest, nil))
				scope.addOpt(ir.MakeBinaryInst(ir.Assign, dest, leftDest, nil))
			case parsing.Minus:
				scope.addOpt(ir.MakeBinaryInst(ir.Sub, leftDest, rightDest, nil))
				scope.addOpt(ir.MakeBinaryInst(ir.Assign, dest, leftDest, nil))
			case parsing.Greater:
				scope.addOpt(ir.MakeReadOnlyInst(ir.Compare, leftDest, ir.CompareExtra{How: ir.Greater, Right: rightDest, Out: dest}))
			case parsing.GreaterEqual:
				scope.addOpt(ir.MakeReadOnlyInst(ir.Compare, leftDest, ir.CompareExtra{How: ir.GreaterOrEqual, Right: rightDest, Out: dest}))
			case parsing.Lesser:
				scope.addOpt(ir.MakeReadOnlyInst(ir.Compare, leftDest, ir.CompareExtra{How: ir.Lesser, Right: rightDest, Out: dest}))
			case parsing.LesserEqual:
				scope.addOpt(ir.MakeReadOnlyInst(ir.Compare, leftDest, ir.CompareExtra{How: ir.LesserOrEqual, Right: rightDest, Out: dest}))
			case parsing.DoubleEqual:
				scope.addOpt(ir.MakeReadOnlyInst(ir.Compare, leftDest, ir.CompareExtra{How: ir.AreEqual, Right: rightDest, Out: dest}))
			case parsing.BangEqual:
				scope.addOpt(ir.MakeReadOnlyInst(ir.Compare, leftDest, ir.CompareExtra{How: ir.NotEqual, Right: rightDest, Out: dest}))
			}
		case parsing.AddressOf:
			switch right := n.Right.(type) {
			case parsing.IdName:
				vn, found := scope.resolve(right.Name)
				if !found {
					panic(parsing.ErrorFromNode(n.Right, undefinedMessage))
				}
				scope.addOpt(ir.MakeBinaryInst(ir.TakeAddress, dest, vn, nil))
			default:
				//:structinreg we rely on ir.TakeAddress to know that some vars can't be in registers when
				// doing indirections. We don't issue ir.TakeAddress in this case currently because array/structs
				// are always on the stack.
				address := computePointer(scope, n.Right)
				scope.addOpt(ir.MakeBinaryInst(ir.Assign, dest, address, nil))
			}
		case parsing.ArrayAccess, parsing.Dot:
			location := computePointer(scope, n)
			scope.addOpt(ir.MakeBinaryInst(ir.IndirectLoad, dest, location, nil))
		default:
			panic(parsing.ErrorFromNode(node, "This expession must generate a value"))
		}
	default:
		panic(parsing.ErrorFromNode(node, "This expession must generate a value"))
	}
}

func computePointer(scope *scope, node parsing.ASTNode) int {
	gen := scope.gen
	gen.pushCurrentlyGenerating(&node)
	defer gen.popCurrentlyGenerating(&node)
	switch n := node.(type) {
	case parsing.ExprNode:
		switch n.Op {
		case parsing.ArrayAccess:
			return computePointerRecursive(scope, node)
		case parsing.Dot:
			left := computePointerRecursive(scope, n.Left)
			result := scope.newVar()
			fieldName := n.Right.(parsing.IdName).Name
			scope.addOpt(ir.MakeBinaryInst(ir.StructMemberPtr, result, left, fieldName))
			return result
		}
	}
	panic("ice: computePointer given a node it doesn't know how to handle. Only ArrayAccess and Dot is supported")
	return 0
}

func computePointerRecursive(scope *scope, node parsing.ASTNode) int {
	gen := scope.gen
	gen.pushCurrentlyGenerating(&node)
	defer gen.popCurrentlyGenerating(&node)
	switch n := node.(type) {
	case parsing.ExprNode:
		switch n.Op {
		case parsing.ArrayAccess:
			left := computePointerRecursive(scope, n.Left)
			position := genExpressionValue(scope, n.Right)
			arrayPointer := scope.newVar()
			scope.addOpt(ir.MakeBinaryInst(ir.ArrayToPointer, arrayPointer, left, nil))
			scope.addOpt(ir.MakeBinaryInst(ir.Add, arrayPointer, position, nil))
			return arrayPointer
		case parsing.Dot:
			left := computePointerRecursive(scope, n.Left)
			result := scope.newVar()
			fieldName := n.Right.(parsing.IdName).Name
			scope.addOpt(ir.MakeBinaryInst(ir.PeelStruct, result, left, fieldName))
			return result
		}
	}
	return genExpressionValue(scope, node)
}

// return a var number which stores a pointer
func genAssignmentTarget(scope *scope, node parsing.ASTNode) int {
	switch n := node.(type) {
	case parsing.ExprNode:
		switch n.Op {
		case parsing.Dereference:
			if ident, bareDeref := n.Right.(parsing.IdName); bareDeref {
				vn, found := scope.resolve(ident.Name)
				if !found {
					panic(parsing.ErrorFromNode(ident, undefinedMessage))
				}
				return vn
			}
			return genExpressionValue(scope, n.Right)
		case parsing.ArrayAccess, parsing.Dot:
			return computePointer(scope, node)
		}
	}
	panic(parsing.ErrorFromNode(node, "Not a valid assignment target"))
}

func boolStrToBool(s string) bool {
	if s == "true" {
		return true
	}
	return false
}

// Get rid of aliasing ir.Assign. Works by catching vars that appear on the rhs of an ir.Assign but is never
// used again. Also get rid of unused variables.
func Prune(block *OptBlock) {
	if block.NumberOfVars == 0 {
		return
	}
	usageLog := make([]struct {
		count        int
		firstUseIdx  int
		secondUseIdx int
		vn           int
	}, block.NumberOfVars)
	for idx, opt := range block.Opts {
		recordUsage := func(varNum int) {
			if usageLog[varNum].count == 0 {
				usageLog[varNum].firstUseIdx = idx
			}
			if usageLog[varNum].count == 1 {
				usageLog[varNum].secondUseIdx = idx
			}
			usageLog[varNum].count++
			usageLog[varNum].vn = varNum
		}
		ir.IterOverAllVars(opt, recordUsage)
	}
	// keep track of all the uneeded assigns
	holes := make([]int, 0, len(block.Opts)/2)
	sort.Slice(usageLog, func(i int, j int) bool {
		return usageLog[i].secondUseIdx < usageLog[j].secondUseIdx
	})
	for _, log := range usageLog {
		if log.count > 2 {
			continue
		}
		genesis := block.Opts[log.firstUseIdx]
		if log.count == 1 && (genesis.Type == ir.Assign || genesis.Type == ir.AssignImm) && log.vn == genesis.Left() {
			// assigned but not used
			holes = append(holes, log.firstUseIdx)
			continue
		}
		changed := false
		if genesis.Type == ir.AssignImm && block.Opts[log.secondUseIdx].Type == ir.Assign {
			changed = true
			holes = append(holes, log.firstUseIdx)
			block.Opts[log.secondUseIdx].Type = ir.AssignImm
			block.Opts[log.secondUseIdx].Extra = block.Opts[log.firstUseIdx].Extra
		}
		if genesis.Type == ir.Assign {
			changed = true
			usingAlias := false
			ir.IterAndMutate(&block.Opts[log.secondUseIdx], func(vn *int) {
				if *vn == genesis.Left() {
					usingAlias = true
					*vn = genesis.Right()
				}
			})
			if usingAlias {
				holes = append(holes, log.firstUseIdx)
			}
		}
		if changed {
			// DumpIr(*block)
		}
	}
	sort.Ints(holes)
	holes = DedupSorted(holes)
	pushDist := 0
	for i, j := 0, 0; i < len(block.Opts); i++ {
		if j < len(holes) && holes[j] == i {
			pushDist++
			j++
			continue
		}
		if pushDist > 0 {
			block.Opts[i-pushDist] = block.Opts[i]
		}
	}
	block.Opts = block.Opts[0 : len(block.Opts)-len(holes)]

	// renumber all the vars
	allVarNums := make([]int, 0, block.NumberOfVars)
	for _, opt := range block.Opts {
		ir.IterOverAllVars(opt, func(vn int) {
			allVarNums = append(allVarNums, vn)
		})
	}
	sort.Ints(allVarNums)
	allVarNums = DedupSorted(allVarNums)
	start := len(allVarNums)
	for i, vn := range allVarNums {
		if vn >= block.NumberOfArgs {
			start = i
			break
		}
	}
	if start < len(allVarNums) {
		allVarNums = allVarNums[start:]
	} else {
		allVarNums = []int{}
	}

	vnMap := make([]int, block.NumberOfVars)
	for idx, vn := range allVarNums {
		vnMap[vn] = idx + block.NumberOfArgs
	}
	for i := 0; i < block.NumberOfArgs; i++ {
		vnMap[i] = i
	}
	for i, opt := range block.Opts {
		switch opt.Type {
		case ir.OptionSelectStart, ir.OutsideLoopMutations, ir.OutOfScopeMutations:
			vnArray := *opt.Extra.(*[]int)
			for i, vn := range vnArray {
				vnArray[i] = vnMap[vn]
			}
		default:
			ir.IterAndMutate(&block.Opts[i], func(vn *int) {
				*vn = vnMap[*vn]
			})
		}
	}

	block.NumberOfVars = len(allVarNums) + block.NumberOfArgs
}

func labelInst(name string) ir.Inst {
	label := ir.Inst{Type: ir.Label}
	label.Extra = name
	return label
}

func DedupSorted(slice []int) []int {
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
