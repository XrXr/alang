package frontend

import (
	"errors"
	"fmt"
	"github.com/XrXr/alang/ir"
	"github.com/XrXr/alang/parsing"
	"strconv"
)

func GenForProc(labelGen *LabelIdGen, order *ProcWorkOrder) {
	var gen procGen
	gen.rootScope = &scope{
		gen:      &gen,
		varTable: make(map[parsing.IdName]int), parentScope: nil}

	gen.addOpt(ir.StartProc{order.Label})
	ret := genForProcSubSection(labelGen, order, gen.rootScope, 0)
	gen.addOpt(ir.EndProc{})
	if ret != len(order.In) {
		parsing.Dump(order.In)
		panic("gen didn't process whole proc")
	}
	order.Out <- OptBlock{NumberOfVars: gen.nextVarNum, Opts: gen.opts}
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
			gen.addOpt(ir.AssignImm{Var: newVar, Val: node.Type})
		case parsing.ExprNode:
			switch node.Op {
			case parsing.Declare:
				varNum := scope.newNamedVar(node.Left.(parsing.IdName))
				err := genExpressionRhs(scope, varNum, node.Right)
				if err != nil {
					panic(err)
				}
			case parsing.Assign:
				leftAsIdent, leftIsIdent := node.Left.(parsing.IdName)
				if leftIsIdent {
					leftVarNum, varFound := scope.resolve(leftAsIdent)
					if !varFound {
						panic(fmt.Sprintf("bug in user program! assign to undefined variable \"%s\"", leftAsIdent))
					}
					err := genExpressionRhs(scope, leftVarNum, node.Right)
					if err != nil {
						panic(err)
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
					gen.addOpt(ir.IndirectWrite{Pointer: leftResult, Data: rightResult})
				}
			default:
				//TODO issue warning here
			}
		case parsing.IfNode:
			sawIf = true
			condVar := scope.newVar()
			genExpressionRhs(scope, condVar, node.Condition)
			labelForIf := labelGen.GenLabel("if_%d")
			gen.addOpt(ir.JumpIfFalse{condVar, labelForIf})
			i = genForProcSubSection(labelGen, order, scope.inherit(), i)
			gen.addOpt(ir.Label{labelForIf})
		case parsing.ElseNode:
			if !sawIfLastTime {
				panic("Bare else. Should've been caught by the parser")
			}
			elseLabel := labelGen.GenLabel("else_%d")
			ifLabel := gen.opts[len(gen.opts)-1]
			gen.opts[len(gen.opts)-1] = ir.Jump{elseLabel}
			gen.addOpt(ifLabel)
			i = genForProcSubSection(labelGen, order, scope.inherit(), i)
			gen.addOpt(ir.Label{elseLabel})
		case parsing.BlockEnd:
			return i
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
	switch n := node.(type) {
	case parsing.Literal:
		var value interface{}
		switch n.Type {
		case parsing.Number:
			v, err := strconv.Atoi(n.Value)
			if err != nil {
				panic(err)
			}
			value = v
		case parsing.Boolean:
			value = boolStrToInt(n.Value)
		case parsing.String:
			value = n.Value
		}
		gen.addOpt(ir.AssignImm{dest, value})
	case parsing.IdName:
		vn, found := scope.resolve(n)
		if !found {
			panic(fmt.Errorf("undefined var %s", n))
		}
		gen.addOpt(ir.Assign{dest, vn})
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
		gen.addOpt(ir.Call{string(n.Callee), argVars, dest})
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
			gen.addOpt(ir.IndirectLoad{Pointer: rightDest, Out: dest})
			return nil
		case parsing.Star, parsing.Minus, parsing.Plus, parsing.Divide:
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
				gen.addOpt(ir.Mult{leftDest, rightDest})
			case parsing.Divide:
				gen.addOpt(ir.Div{leftDest, rightDest})
			case parsing.Plus:
				gen.addOpt(ir.Add{leftDest, rightDest})
			case parsing.Minus:
				gen.addOpt(ir.Sub{leftDest, rightDest})
			}
			gen.addOpt(ir.Assign{dest, leftDest})
		case parsing.AddressOf:
			// TODO: incomplete: &(someStruct.foo.sdfsd)
			right := n.Right.(parsing.IdName)
			vn, found := scope.resolve(right)
			if !found {
				return errors.New("undefined var")
			}
			gen.addOpt(ir.TakeAddress{Var: vn, Out: dest})
		case parsing.Dot:
			left := scope.newVar()
			err := genExpressionRhs(scope, left, n.Left)
			if err != nil {
				return err
			}
			gen.addOpt(ir.LoadStructMember{Base: left, Member: string(n.Right.(parsing.IdName)), Out: dest})
		default:
			return errors.New(fmt.Sprintf("Unsupported value expression type %v", n.Op))
		}
	default:
		panic(n)
		panic("unknown type of node")
	}
	return nil
}

// return a var number which stores a pointer
func genAssignmentTarget(scope *scope, node interface{}) (int, error) {
	gen := scope.gen
	switch n := node.(type) {
	case parsing.ExprNode:
		if n.Op == parsing.Dereference {
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
		} else if n.Op == parsing.Dot {
			structBase := scope.newVar()
			err := genExpressionRhs(scope, structBase, n.Left)
			if err != nil {
				return 0, err
			}
			member := string(n.Right.(parsing.IdName))
			out := scope.newVar()
			gen.addOpt(ir.StructMemberPtr{Base: structBase, Member: member, Out: out})
			return out, nil
		}
	}
	return 0, errors.New("Can't resolve expression to lvalue")
}

func boolStrToInt(s string) (ret int) {
	if s == "true" {
		ret = 1
	}
	return
}

func Prune(block *OptBlock) {
	usageLog := make([]struct {
		count        int
		fristUseIdx  int
		secondUseIdx int
	}, block.NumberOfVars)
	use := func(varNum int, optIdx int) {
		if usageLog[varNum].count == 0 {
			usageLog[varNum].fristUseIdx = optIdx
		}
		if usageLog[varNum].count == 1 {
			usageLog[varNum].secondUseIdx = optIdx
		}
		usageLog[varNum].count++
	}
	uses := make([]int, 3)
	for idx, opt := range block.Opts {
		switch opt := opt.(type) {
		case ir.Assign:
			opt.Uses(uses)
			use(uses[0], idx)
			use(uses[1], idx)
		case ir.LoadStructMember:
			opt.Uses(uses)
			use(uses[0], idx)
			use(uses[1], idx)
		case ir.StructMemberPtr:
			opt.Uses(uses)
			use(uses[0], idx)
			use(uses[1], idx)
		}
	}
	for _, log := range usageLog {
		if log.count != 2 {
			continue
		}
		genesisAssign, fromAssign := block.Opts[log.fristUseIdx].(ir.Assign)
		if fromAssign {
			switch opt := block.Opts[log.secondUseIdx].(type) {
			case ir.LoadStructMember:
				opt.Swap(genesisAssign.Left, genesisAssign.Right)
				block.Opts[log.secondUseIdx] = opt
			case ir.StructMemberPtr:
				opt.Swap(genesisAssign.Left, genesisAssign.Right)
				block.Opts[log.secondUseIdx] = opt
			}
		}
	}
}
