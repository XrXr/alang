package frontend

import (
	"errors"
	"fmt"
	"github.com/XrXr/alang/ir"
	"github.com/XrXr/alang/parsing"
	"sort"
	"strconv"
)

func GenForProc(labelGen *LabelIdGen, order *ProcWorkOrder) {
	var gen procGen
	gen.rootScope = &scope{
		gen:         &gen,
		varTable:    make(map[parsing.IdName]int),
		parentScope: nil,
	}
	gen.labelGen = labelGen

	// fmt.Printf("frontend: generating for %s\n", order.Name)
	for i, arg := range order.ProcDecl.Args {
		_ = i
		gen.rootScope.newNamedVar(arg.Name)
		// fmt.Printf("arg %d is named %s\n", i, arg.Name)
	}

	gen.addOpt(ir.Inst{Type: ir.StartProc, Extra: string(order.Name)})
	ret := genForProcSubSection(labelGen, order, gen.rootScope, 0)
	gen.addOpt(ir.Inst{Type: ir.EndProc})
	if ret != len(order.In) {
		parsing.Dump(order.In)
		panic("gen didn't process whole proc")
	}
	order.Out <- OptBlock{NumberOfVars: gen.nextVarNum, NumberOfArgs: len(order.ProcDecl.Args), Opts: gen.opts}
	close(order.Out)
}

// return index to the first unprocessed node
func genForProcSubSection(labelGen *LabelIdGen, order *ProcWorkOrder, scope *scope, start int) int {
	gen := scope.gen
	i := start
	sawIf := false
	for i < len(order.In) {
		nodePtr := order.In[i]
		i++
		sawIfLastTime := sawIf
		sawIf = false
		switch node := (*nodePtr).(type) {
		case parsing.Declaration:
			// these are declaration without values i.e. not foo := 3
			newVar := scope.newNamedVar(node.Name)
			gen.addOpt(ir.MakeUnaryInstWithAux(ir.AssignImm, newVar, node.Type))
		case parsing.ExprNode:
			switch node.Op {
			case parsing.Declare:
				varNum := scope.newNamedVar(node.Left.(parsing.IdName))
				err := genExpressionRhs(scope, varNum, node.Right)
				if err != nil {
					panic(err)
				}
			case parsing.Assign, parsing.PlusEqual, parsing.MinusEqual:
				leftAsIdent, leftIsIdent := node.Left.(parsing.IdName)
				if leftIsIdent {
					leftVarNum, varFound := scope.resolve(leftAsIdent)
					if !varFound {
						panic(fmt.Sprintf("bug in user program! assign to undefined variable \"%s\"", leftAsIdent))
					}
					rightResult := leftVarNum
					if node.Op == parsing.PlusEqual || node.Op == parsing.MinusEqual {
						rightResult = scope.newVar()
					}
					err := genExpressionRhs(scope, rightResult, node.Right)
					if err != nil {
						panic(err)
					}
					switch node.Op {
					case parsing.PlusEqual:
						gen.addOpt(ir.MakeBinaryInstWithAux(ir.Add, leftVarNum, rightResult, nil))
					case parsing.MinusEqual:
						gen.addOpt(ir.MakeBinaryInstWithAux(ir.Sub, leftVarNum, rightResult, nil))
					default:
						// put there already in the genExpressionRhs above
					}
				} else {
					leftResult, err := genAssignmentTarget(scope, node.Left)
					if err != nil {
						panic(err)
					}
					rightResult := scope.newVar()
					err = genExpressionRhs(scope, rightResult, node.Right)
					if err != nil {
						panic(err)
					}
					var leftTmp int
					if node.Op == parsing.PlusEqual || node.Op == parsing.MinusEqual {
						leftTmp = scope.newVar()
						gen.addOpt(ir.MakeBinaryInstWithAux(ir.IndirectLoad, leftResult, leftTmp, nil))
					}
					switch node.Op {
					case parsing.PlusEqual:
						gen.addOpt(ir.MakeBinaryInstWithAux(ir.Add, leftTmp, rightResult, nil))
						gen.addOpt(ir.MakeBinaryInstWithAux(ir.IndirectWrite, leftResult, leftTmp, nil))
					case parsing.MinusEqual:
						gen.addOpt(ir.MakeBinaryInstWithAux(ir.Sub, leftTmp, rightResult, nil))
						gen.addOpt(ir.MakeBinaryInstWithAux(ir.IndirectWrite, leftResult, leftTmp, nil))
					default:
						gen.addOpt(ir.MakeBinaryInstWithAux(ir.IndirectWrite, leftResult, rightResult, nil))
					}
				}
			default:
				//TODO issue warning here
			}
		case parsing.IfNode:
			sawIf = true
			condVar := scope.newVar()
			err := genExpressionRhs(scope, condVar, node.Condition)
			if err != nil {
				panic(err)
			}
			labelForIf := labelGen.GenLabel("if_%d")
			gen.addOpt(ir.MakeUnaryInstWithAux(ir.JumpIfFalse, condVar, labelForIf))
			i = genForProcSubSection(labelGen, order, scope.inherit(), i)
			gen.addOpt(labelInst(labelForIf))
		case parsing.ElseNode:
			if !sawIfLastTime {
				panic("Bare else. Should've been caught by the parser")
			}
			elseLabel := labelGen.GenLabel("else_%d")
			ifLabel := gen.opts[len(gen.opts)-1]
			gen.opts[len(gen.opts)-1] = ir.MakeUnaryInstWithAux(ir.Jump, 0, elseLabel)
			gen.addOpt(ifLabel)
			i = genForProcSubSection(labelGen, order, scope.inherit(), i)
			gen.addOpt(labelInst(elseLabel))
		case parsing.Loop:
			loopStart := labelGen.GenLabel("loop_%d")
			loopEnd := loopStart + "_loopEnd"
			loopScope := scope.inherit()
			loopScope.loopLabel = loopStart

			var iterationVar int
			var varUsed bool
			usingRangeExpr := false
			var rangeExpr parsing.ExprNode

			if loopExpr, loopExprIsExprNode := node.Expression.(parsing.ExprNode); loopExprIsExprNode {
				switch loopExpr.Op {
				case parsing.Declare:
					// parser gurantee
					usingRangeExpr = true
					rangeExpr = loopExpr.Right.(parsing.ExprNode)
					iterationVar = loopScope.newNamedVar(loopExpr.Left.(parsing.IdName))
					varUsed = true
					fallthrough
				case parsing.Range:
					if !usingRangeExpr {
						rangeExpr = loopExpr
					}
					if !varUsed {
						varUsed = true
						iterationVar = scope.newVar()
					}
					usingRangeExpr = true
					if rangeExpr.Op != parsing.Range {
						panic("parser bug")
					}
					endVar := scope.newVar()
					err := genExpressionRhs(loopScope, endVar, rangeExpr.Right)
					if err != nil {
						panic(err)
					}

					err = genExpressionRhs(loopScope, iterationVar, rangeExpr.Left)
					if err != nil {
						panic(err)
					}
					condVar := loopScope.newVar()
					gen.addOpt(labelInst(loopStart))
					gen.addOpt(ir.MakeBinaryInstWithAux(ir.Compare, iterationVar, endVar,
						ir.CompareExtra{
							How: ir.LesserOrEqual,
							Out: condVar,
						}))
					gen.addOpt(ir.MakeUnaryInstWithAux(ir.JumpIfFalse, condVar, loopEnd))
				}
			}
			if !usingRangeExpr {
				gen.addOpt(labelInst(loopStart))
				condVar := loopScope.newVar()
				err := genExpressionRhs(loopScope, condVar, node.Expression)
				if err != nil {
					panic(err)
				}
				gen.addOpt(ir.MakeUnaryInstWithAux(ir.JumpIfFalse, condVar, loopEnd))
			}

			// loop body
			i = genForProcSubSection(labelGen, order, loopScope, i)
			// continue code
			gen.addOpt(labelInst(loopStart + "_loopContinue"))
			if usingRangeExpr {
				gen.addOpt(ir.MakeUnaryInstWithAux(ir.Increment, iterationVar, nil))
				gen.addOpt(ir.MakeUnaryInstWithAux(ir.Jump, 0, loopStart))
			} else {
				gen.addOpt(ir.MakeUnaryInstWithAux(ir.Jump, 0, loopStart))
			}
			gen.addOpt(labelInst(loopEnd))
		case parsing.BlockEnd:
			return i
		case parsing.ContinueNode:
			if scope.loopLabel == "" {
				panic("continue outside of a loop")
			}
			gen.addOpt(ir.MakeUnaryInstWithAux(ir.Jump, 0, scope.loopLabel+"_loopContinue"))
		case parsing.BreakNode:
			if scope.loopLabel == "" {
				panic("break outside of a loop")
			}
			gen.addOpt(ir.MakeUnaryInstWithAux(ir.Jump, 0, scope.loopLabel+"_loopEnd"))
		case parsing.ReturnNode:
			var returnValues []int
			for i := 0; i < len(node.Values); i++ {
				returnValues = append(returnValues, scope.newVar())
			}
			for i, valueExpr := range node.Values {
				err := genExpressionRhs(scope, returnValues[i], valueExpr)
				if err != nil {
					panic(err)
				}
			}
			gen.addOpt(ir.Inst{Type: ir.Return, Extra: ir.ReturnExtra{returnValues}})
		default:
			err := genExpressionRhs(scope, scope.newVar(), node)
			if err != nil {
				panic(err)
			}
		}
	}
	return i
}

func genExpressionRhs(scope *scope, dest int, node interface{}) error {
	gen := scope.gen
	labelGen := gen.labelGen
	switch n := node.(type) {
	case parsing.Literal:
		var value interface{}
		switch n.Type {
		case parsing.Number:
			v, err := strconv.ParseInt(n.Value, 10, 64)
			if err == nil {
				value = v
			} else {
				value, err = strconv.ParseUint(n.Value, 10, 64)
				if err != nil {
					panic(err)
				}
			}
		case parsing.Boolean:
			value = boolStrToBool(n.Value)
		case parsing.String:
			value = n.Value
		}
		gen.addOpt(ir.MakeUnaryInstWithAux(ir.AssignImm, dest, value))
	case parsing.IdName:
		vn, found := scope.resolve(n)
		if !found {
			panic(fmt.Errorf("undefined var %s", n))
		}
		gen.addOpt(ir.MakeBinaryInstWithAux(ir.Assign, dest, vn, nil))
	case parsing.ProcCall:
		var argVars []int
		for _, argNode := range n.Args {
			argEval := scope.newVar()
			err := genExpressionRhs(scope, argEval, argNode)
			if err != nil {
				panic(err)
			}
			argVars = append(argVars, argEval)
		}
		gen.addOpt(ir.MakeUnaryInstWithAux(ir.Call, dest, ir.CallExtra{
			Name:    string(n.Callee),
			ArgVars: argVars,
		}))
	case parsing.ExprNode:
		switch n.Op {
		case parsing.Dereference:
			if n.Left != nil {
				panic("parser bug")
			}
			rightDest := scope.newVar()
			err := genExpressionRhs(scope, rightDest, n.Right)
			if err != nil {
				return err
			}
			gen.addOpt(ir.MakeBinaryInstWithAux(ir.IndirectLoad, rightDest, dest, nil))
			return nil
		case parsing.LogicalNot:
			if n.Left != nil {
				panic("parser bug")
			}
			rightDest := scope.newVar()
			err := genExpressionRhs(scope, rightDest, n.Right)
			if err != nil {
				return err
			}
			gen.addOpt(ir.MakeBinaryInstWithAux(ir.Not, rightDest, dest, nil))
		case parsing.LogicalAnd:
			err := genExpressionRhs(scope, dest, n.Left)
			if err != nil {
				return err
			}
			end := labelGen.GenLabel("andEnd_%d")
			gen.addOpt(ir.MakeUnaryInstWithAux(ir.JumpIfFalse, dest, end))
			rightDest := scope.newVar()
			err = genExpressionRhs(scope, rightDest, n.Right)
			if err != nil {
				return err
			}
			gen.addOpt(ir.MakeBinaryInstWithAux(ir.BoolAnd, dest, rightDest, nil))
			gen.addOpt(labelInst(end))
		case parsing.LogicalOr:
			err := genExpressionRhs(scope, dest, n.Left)
			if err != nil {
				return err
			}
			end := labelGen.GenLabel("orEnd_%d")
			gen.addOpt(ir.MakeUnaryInstWithAux(ir.JumpIfTrue, dest, end))
			rightDest := scope.newVar()
			err = genExpressionRhs(scope, rightDest, n.Right)
			if err != nil {
				return err
			}
			gen.addOpt(ir.MakeBinaryInstWithAux(ir.BoolOr, dest, rightDest, nil))
			gen.addOpt(labelInst(end))
		case parsing.Star, parsing.Minus, parsing.Plus, parsing.Divide,
			parsing.Greater, parsing.GreaterEqual, parsing.Lesser,
			parsing.LesserEqual, parsing.DoubleEqual, parsing.BangEqual, parsing.ArrayAccess:

			leftDest := scope.newVar()
			err := genExpressionRhs(scope, leftDest, n.Left)
			if err != nil {
				return err
			}
			rightDest := scope.newVar()
			err = genExpressionRhs(scope, rightDest, n.Right)
			if err != nil {
				return err
			}
			switch n.Op {
			case parsing.Star:
				gen.addOpt(ir.MakeBinaryInstWithAux(ir.Mult, leftDest, rightDest, nil))
				gen.addOpt(ir.MakeBinaryInstWithAux(ir.Assign, dest, leftDest, nil))
			case parsing.Divide:
				gen.addOpt(ir.MakeBinaryInstWithAux(ir.Div, leftDest, rightDest, nil))
				gen.addOpt(ir.MakeBinaryInstWithAux(ir.Assign, dest, leftDest, nil))
			case parsing.Plus:
				gen.addOpt(ir.MakeBinaryInstWithAux(ir.Add, leftDest, rightDest, nil))
				gen.addOpt(ir.MakeBinaryInstWithAux(ir.Assign, dest, leftDest, nil))
			case parsing.Minus:
				gen.addOpt(ir.MakeBinaryInstWithAux(ir.Sub, leftDest, rightDest, nil))
				gen.addOpt(ir.MakeBinaryInstWithAux(ir.Assign, dest, leftDest, nil))
			case parsing.Greater:
				gen.addOpt(ir.MakeBinaryInstWithAux(ir.Compare, leftDest, rightDest, ir.CompareExtra{How: ir.Greater, Out: dest}))
			case parsing.GreaterEqual:
				gen.addOpt(ir.MakeBinaryInstWithAux(ir.Compare, leftDest, rightDest, ir.CompareExtra{How: ir.GreaterOrEqual, Out: dest}))
			case parsing.Lesser:
				gen.addOpt(ir.MakeBinaryInstWithAux(ir.Compare, leftDest, rightDest, ir.CompareExtra{How: ir.Lesser, Out: dest}))
			case parsing.LesserEqual:
				gen.addOpt(ir.MakeBinaryInstWithAux(ir.Compare, leftDest, rightDest, ir.CompareExtra{How: ir.LesserOrEqual, Out: dest}))
			case parsing.DoubleEqual:
				gen.addOpt(ir.MakeBinaryInstWithAux(ir.Compare, leftDest, rightDest, ir.CompareExtra{How: ir.AreEqual, Out: dest}))
			case parsing.BangEqual:
				gen.addOpt(ir.MakeBinaryInstWithAux(ir.Compare, leftDest, rightDest, ir.CompareExtra{How: ir.NotEqual, Out: dest}))
			case parsing.ArrayAccess:
				dataPointer := scope.newVar()
				gen.addOpt(ir.MakeBinaryInstWithAux(ir.ArrayToPointer, leftDest, dataPointer, nil))
				gen.addOpt(ir.MakeBinaryInstWithAux(ir.Add, dataPointer, rightDest, nil))
				gen.addOpt(ir.MakeBinaryInstWithAux(ir.IndirectLoad, dataPointer, dest, nil))
			}
		case parsing.AddressOf:
			// TODO: incomplete: &(someStruct.foo.sdfsd)
			right := n.Right.(parsing.IdName)
			vn, found := scope.resolve(right)
			if !found {
				return errors.New("undefined var")
			}
			gen.addOpt(ir.MakeBinaryInstWithAux(ir.TakeAddress, vn, dest, nil))
		case parsing.Dot:
			left := scope.newVar()
			err := genExpressionRhs(scope, left, n.Left)
			if err != nil {
				return err
			}
			gen.addOpt(ir.MakeBinaryInstWithAux(ir.LoadStructMember, left, dest, string(n.Right.(parsing.IdName))))
		default:
			return errors.New(fmt.Sprintf("Unsupported value expression type %v", n.Op))
		}
	default:
		parsing.Dump(n)
		panic("unknown type of node")
	}
	return nil
}

// return a var number which stores a pointer
func genAssignmentTarget(scope *scope, node interface{}) (int, error) {
	gen := scope.gen
	switch n := node.(type) {
	case parsing.ExprNode:
		switch n.Op {
		case parsing.Dereference:
			if ident, bareDeref := n.Right.(parsing.IdName); bareDeref {
				vn, found := scope.resolve(ident)
				if !found {
					return 0, errors.New("undefined var")
				}
				return vn, nil
			}
			pointerVar := scope.newVar()
			err := genExpressionRhs(scope, pointerVar, n.Right)
			if err != nil {
				return 0, err
			}
			return pointerVar, nil
		case parsing.ArrayAccess:
			array := scope.newVar()
			if left, leftIsExpr := n.Left.(parsing.ExprNode); leftIsExpr && left.Op == parsing.Dot {
				// for foo.bar[324] = 234234
				structBase := scope.newVar()
				err := genExpressionRhs(scope, structBase, left.Left)
				if err != nil {
					return 0, err
				}
				member := string(left.Right.(parsing.IdName))
				gen.addOpt(ir.MakeBinaryInstWithAux(ir.StructMemberPtr, structBase, array, member))
			} else {
				err := genExpressionRhs(scope, array, n.Left)
				if err != nil {
					return 0, err
				}
			}
			position := scope.newVar()
			err := genExpressionRhs(scope, position, n.Right)
			if err != nil {
				return 0, err
			}
			dataPointer := scope.newVar()
			gen.addOpt(ir.MakeBinaryInstWithAux(ir.ArrayToPointer, array, dataPointer, nil))
			gen.addOpt(ir.MakeBinaryInstWithAux(ir.Add, dataPointer, position, nil))
			return dataPointer, nil
		case parsing.Dot:
			structBase := scope.newVar()
			err := genExpressionRhs(scope, structBase, n.Left)
			if err != nil {
				return 0, err
			}
			member := string(n.Right.(parsing.IdName))
			out := scope.newVar()
			gen.addOpt(ir.MakeBinaryInstWithAux(ir.StructMemberPtr, structBase, out, member))
			return out, nil
		}
	}
	parsing.Dump(node)
	return 0, errors.New("Can't assign to that")
}

func boolStrToBool(s string) bool {
	if s == "true" {
		return true
	}
	return false
}

func Prune(block *OptBlock) {
	if block.NumberOfVars == 0 {
		return
	}
	usageLog := make([]struct {
		count        int
		firstUseIdx  int
		secondUseIdx int
	}, block.NumberOfVars)
	recordUsage := func(varNum int, optIdx int) {
		if usageLog[varNum].count == 0 {
			usageLog[varNum].firstUseIdx = optIdx
		}
		if usageLog[varNum].count == 1 {
			usageLog[varNum].secondUseIdx = optIdx
		}
		usageLog[varNum].count++
	}
	for idx, opt := range block.Opts {
		if opt.Type > ir.UnaryInstructions {
			recordUsage(opt.Oprand1, idx)
		}
		if opt.Type > ir.BinaryInstructions {
			recordUsage(opt.Oprand2, idx)
		}
		if opt.Type == ir.Call {
			for _, vn := range opt.Extra.(ir.CallExtra).ArgVars {
				recordUsage(vn, idx)
			}
		}
		if opt.Type == ir.Return {
			for _, vn := range opt.Extra.(ir.ReturnExtra).Values {
				recordUsage(vn, idx)
			}
		}
		if opt.Type == ir.Compare {
			recordUsage(opt.Extra.(ir.CompareExtra).Out, idx)
		}
	}
	// keep track of all the uneeded assigns
	hollow := make([]int, 0, len(block.Opts)/2)
	sort.Slice(usageLog, func(i int, j int) bool {
		return usageLog[i].secondUseIdx < usageLog[j].secondUseIdx
	})
	for _, log := range usageLog {
		if log.count != 2 {
			continue
		}

		genesis := block.Opts[log.firstUseIdx]
		if genesis.Type == ir.AssignImm && block.Opts[log.secondUseIdx].Type == ir.Assign {
			hollow = append(hollow, log.secondUseIdx)
		}
		if genesis.Type == ir.Assign {
			hollow = append(hollow, log.firstUseIdx)

			second := block.Opts[log.secondUseIdx]
			switch second.Type {
			default:
				block.Opts[log.secondUseIdx].Swap(genesis.Left(), genesis.Right())
				if second.Type == ir.Compare {
					extra := second.Extra.(ir.CompareExtra)
					if extra.Out == genesis.Left() {
						extra.Out = genesis.Right()
						block.Opts[log.secondUseIdx].Extra = extra
					}
				}
			case ir.Call:
				extra := block.Opts[log.secondUseIdx].Extra.(ir.CallExtra)
				for i, vn := range extra.ArgVars {
					if vn == genesis.Left() {
						extra.ArgVars[i] = genesis.Right()
					}
				}
				block.Opts[log.secondUseIdx].Extra = extra
			case ir.Return:
				extra := block.Opts[log.secondUseIdx].Extra.(ir.ReturnExtra)
				for i, vn := range extra.Values {
					if vn == genesis.Left() {
						extra.Values[i] = genesis.Right()
					}
				}
				block.Opts[log.secondUseIdx].Extra = extra
			}
		}
	}
	sort.Ints(hollow)
	hollow = dedupSorted(hollow)
	pushDist := 0
	for i, j := 0, 0; i < len(block.Opts); i++ {
		if j < len(hollow) && hollow[j] == i {
			pushDist++
			j++
			continue
		}
		if pushDist > 0 {
			block.Opts[i-pushDist] = block.Opts[i]
		}
	}
	block.Opts = block.Opts[0 : len(block.Opts)-len(hollow)]

	// renumber all the vars

	allVarNums := make([]int, 0, block.NumberOfVars)
	for _, opt := range block.Opts {
		if opt.Oprand1 >= block.NumberOfArgs {
			allVarNums = append(allVarNums, opt.Oprand1)
		}
		if opt.Oprand2 >= block.NumberOfArgs {
			allVarNums = append(allVarNums, opt.Oprand2)
		}
		if opt.Type == ir.Call {
			allVarNums = append(allVarNums, opt.Extra.(ir.CallExtra).ArgVars...)
		}
		if opt.Type == ir.Return {
			allVarNums = append(allVarNums, opt.Extra.(ir.ReturnExtra).Values...)
		}
		if opt.Type == ir.Compare {
			allVarNums = append(allVarNums, opt.Extra.(ir.CompareExtra).Out)
		}
	}
	sort.Ints(allVarNums)
	allVarNums = dedupSorted(allVarNums)
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
		opt.Oprand1 = vnMap[opt.Oprand1]
		opt.Oprand2 = vnMap[opt.Oprand2]
		if opt.Type == ir.Call {
			extra := opt.Extra.(ir.CallExtra)
			for i, vn := range extra.ArgVars {
				extra.ArgVars[i] = vnMap[vn]
			}
			opt.Extra = extra
		}
		if opt.Type == ir.Compare {
			extra := opt.Extra.(ir.CompareExtra)
			extra.Out = vnMap[extra.Out]
			opt.Extra = extra
		}
		if opt.Type == ir.Return {
			extra := opt.Extra.(ir.ReturnExtra)
			for i, vn := range extra.Values {
				extra.Values[i] = vnMap[vn]
			}
			opt.Extra = extra
		}
		block.Opts[i] = opt
	}

	block.NumberOfVars = len(allVarNums) + block.NumberOfArgs
}

func labelInst(name string) ir.Inst {
	label := ir.Inst{Type: ir.Label}
	label.Extra = name
	return label
}

func dedupSorted(slice []int) []int {
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
