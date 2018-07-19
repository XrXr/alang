main :: proc() {
	a := false
	if !a {
		puts("1/4 success\n")
	}
	if !!a {
	} else {
		puts("2/4 success\n")
	}
	b := true
	if !b {
	} else {
		puts("3/4 success\n")
	}

	c := &a
	if !c {
	} else {
		puts("4/4 success\n")
	}
	// TODO add a testcase here when we zero out decls
}
