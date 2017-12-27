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
		panic("gen didn't process whole proc")
	}
	// TODO: framesize is wrong
	order.Out <- OptBlock{gen.nextVarNum * 8, gen.nextVarNum, gen.opts}
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
		case parsing.ExprNode:
			switch node.Op {
			case parsing.Declare:
				varNum := scope.newNamedVar(node.Left.(parsing.IdName))
				err := genSimpleValuedExpression(scope, varNum, node.Right)
				if err != nil {
					panic(err)
				}
			case parsing.Assign:
				leftVarNum, varFound := scope.resolve(node.Left.(parsing.IdName))
				if !varFound {
					panic("bug in user program! assign to undefined var")
				}
				err := genSimpleValuedExpression(scope, leftVarNum, node.Right)
				if err != nil {
					panic(err)
				}
			default:
				//TODO issue warning here
			}
		case parsing.IfNode:
			sawIf = true
			condVar := scope.newVar()
			genSimpleValuedExpression(scope, condVar, node.Condition)
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
		case parsing.ProcCall:
			var argVars []int
			for _, argNode := range node.Args {
				argEval := scope.newVar()
				err := genSimpleValuedExpression(scope, argEval, argNode)
				if err != nil {
					panic(err)
				}
				argVars = append(argVars, argEval)
			}
			gen.addOpt(ir.Call{string(node.Callee), argVars})
		case parsing.BlockEnd:
			return i
		}
	}
	return i
}

func genSimpleValuedExpression(scope *scope, dest int, node interface{}) error {
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
			return errors.New("undefined var")
		}
		gen.addOpt(ir.Assign{dest, vn})
	case parsing.ExprNode:
		switch n.Op {
		case parsing.Star, parsing.Minus, parsing.Plus, parsing.Divide:
			leftDest := scope.newVar()
			err := genSimpleValuedExpression(scope, leftDest, n.Left)
			if err != nil {
				return err
			}
			rightDest := scope.newVar()
			err = genSimpleValuedExpression(scope, rightDest, n.Right)
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
		default:
			return errors.New(fmt.Sprintf("Unsupported value expression type %v", n.Op))
		}
	}
	return nil
}

func boolStrToInt(s string) (ret int) {
	if s == "true" {
		ret = 1
	}
	return
}
