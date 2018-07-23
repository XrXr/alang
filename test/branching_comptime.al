main :: proc () {
	puts("--basic--\n")
	basic(true)
	basic(false)

	puts("--nested--\n")
	nested(false, false)
	nested(true, false)
	nested(false, true)
	nested(true, true)

	puts("--pre-determined control flow--\n")
	c := 40
	// normally this would prevent us from precomputing c, but since we know
	// the condition at compile time, c stays compile time computabe.
	if false {
		c -= runtimeInt()
	} else {
		c += 2828
	}
	// c = 2868
	if true {
		c -= 7
	} else {
		c -= runtimeInt()
	}
	// c = 2861
	if false {
		c += runtimeInt()
	}
	print_int(c)
}

basic :: proc(cond bool) {
	foo := 500
	bar := 300
	// foo and bar should be constant within both of these blocks
	// even though we don't know `cond` until runtime
	if cond {
		foo += 399
		print_int(foo)
		print_int(bar)
	} else {
		bar = 999
		foo -= 200
		print_int(foo)
		print_int(bar)
	}
	// foo and bar stop being constant here since we don't know which branch happens at compile time
	print_int(foo)
	print_int(bar)
}

nested :: proc (cond bool, secondCond bool) {
	d := 300
	if cond {
		d += 1000
		// d == 1300
		if false {
			d += runtimeInt()
		} else {
			d += 10
			// d == 1310
			if secondCond {
				d -= 210
				// d == 1100
				print_int(d)
			} else {
				d -= 1
				// d == 1309
				print_int(d)
			}
		}
	} else {
		d += 2000
		// d == 2300
		if false {
			d += runtimeInt()
		} else {
			d += 20
			// d == 2320
			if secondCond {
				d -= 2
				// d == 2318
				print_int(d)
			} else {
				d -= 3
				// d == 2317
				print_int(d)
			}
		}
	}

	print_int(d)
}

runtimeTrue :: proc () -> bool{
	return true
}

runtimeFalse :: proc () -> bool{
	return false
}

runtimeInt :: proc () -> int {
	return 9292
}
