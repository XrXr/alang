main :: proc () {
	puts("assigns\n")
	big := 228282982198
	var a s32
	var small u8
	var small_signed s8
	var foo s64

	a = -1
	small = 255
	a = small
	print_int(a)

	a = -1
	small_signed = -123
	a = small_signed
	print_int(a)

	puts("\naddition\n")

	a = 626
	print_int(a)
	a += big
	print_int(a)

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

	b = big
	a = -382
	b = b - a
	print_int(b)

	small = -122
	b = big
	b -= small
	print_int(b)

	puts("\nmultiplication:\n")

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

	puts("\ndivision:\n")

	small = 29
	b = 3
	small = small / b
	print_int(small)

	a = 281924
    // 96 * 4 = 384 which is -128 in s8 with one extra bit. Testing sign extenion.
	small = 96
	small = small * 4
	print_int(small)
	a = a / small
	print_int(a)
}
