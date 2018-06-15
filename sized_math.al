main :: proc () {
	puts("addition\n")
	big := 228282982198

	var a s32
	a = 626
	print_int(a)
	a += big
	print_int(a)

	var small s8
	small = 232
	small += big
	print_int(small)

	b := big
	a = -382
	b = b + a
	print_int(b)

	small = -122
	b = big
	b += small
	print_int(b)

	puts("\nsubtration:\n")

	a = 626
	a -= big
	print_int(a)

	small = 232
	small -= big
	print_int(small)

	b := big
	a = -382
	b = b - a
	print_int(b)

	puts("\nmultiplication:\n")

	small = -122
	b = big
	b -= small
	print_int(b)

	var foo s64
	foo = 251
	a = -1234125
	foo = foo * a
	print_int(foo)

	a = 251
	small = -113
	a = a * small
	print_int(a)

	small = 82
	small = small * big
	// the result is funny because after the multiply there is
	// junk in the upper half of the register. It will be fixed when we type check function call arguments
	print_int(small)
}
