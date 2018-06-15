main :: proc () {
	print_int(83)
	a := 58
	puts("a: ")
	print_int(a)
	b := 83
	puts("b: ")
	print_int(b)
	a = b
	puts("a after a = b: ")
	print_int(a)
	b = 25
	puts("b after b = 25: ")
	print_int(b)
	puts("a after b = 25: ")
	print_int(a)
	puts("a after a += 873: ")
	a += 873
	print_int(a)
	puts("a after a = a-626+873: ")
	a = a - 626 + 873
	print_int(a)
}
