phase :: foreign proc(a int, b int, print bool) -> int

main :: proc () {
	ret := phase(2, 31, true)
	print_int(ret)
	ret = phase(2, 20, false)
	print_int(ret)
}
