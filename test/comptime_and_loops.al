main :: proc () {
	a := 20
	// we don't know the value of a at the end of the loop because
	// we don't how many iterations the loop runs
	for 1..runtimeValue() {
		a += 3
	}
	print_int(a)

	b := 20
	for 1..3 {
		b += 1
		// there is not a single value of b we can subsitute and be correct
		print_int(b)
	}

	// nested scope
	c := 200
	for 1..100 {
		if runtimeValue() < 5 {
			if runtimeValue() < 4 {
				if runtimeValue() < 3 {
					c -= 1
				}
			}
		}
		if runtimeValue() > 1 {
			c += 1
		}
	}
	print_int(c)

	// pointers
	ptr := &c
	for 1..5 {
		if runtimeValue() > 100 {
			ptr = &b
		}
	}
	print_int(@ptr)

	ptr2 := &c
	for 1..5 {
		if runtimeValue() > 100 {
			ptr2 = runtimePointer(ptr2)
		}
	}
	print_int(@ptr2)
}

runtimeValue :: proc () -> int {
	return 1
}

runtimePointer :: proc (a *int) -> *int {
	return a
}
