main :: proc () {
	a := "this is a string"
	print_int(a.length)
	ptr := a.data
	// should be 'a'
	print_int(@(ptr + 8))
	// should be 0
	print_int(@(ptr + a.length))

	ap := &a
	ptr = ap.data
	print_int(ap.length)
	print_int(@(ptr + 8))
	print_int(@(ptr + a.length))
}
