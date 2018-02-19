package frontend

import "fmt"

func DumpIr(block OptBlock) {
	fmt.Println("IR Dump:")
	for i, opt := range block.Opts {
		fmt.Printf("%d %v\n", i, opt)
	}
}
