struct nums {
	a int
	b int
	c int
}

main :: proc () {
	put_junk()
	verify_numbers()
	put_junk()
	verify_struct()
	put_junk()
	verify_struct2()
	put_junk()
	verify_array()
}

verify_array :: proc() {
	var empty [8]u8
	for i := 0..7 {
		if empty[i] != 0 {
			puts("junk in fresh array\n")
		}
	}
	var empty2 [6]u8
	for i := 0..5 {
		if empty2[i] != 0 {
			puts("junk in fresh array\n")
		}
	}
}

verify_struct :: proc() {
	var empty nums
	print_int(empty.a)
	print_int(empty.b)
	print_int(empty.c)
}

verify_struct2 :: proc() {
	empty := nums()
	print_int(empty.a)
	print_int(empty.b)
	print_int(empty.c)
}

verify_numbers :: proc() {
	var a int
	var b s32
	var c s64
	print_int(a)
	print_int(b)
	print_int(c)
}

put_junk :: proc() {
	var junk [1000]u8
	for i := 0..999 {
		junk[i] = 255
	}
}
