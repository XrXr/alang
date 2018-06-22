main :: proc () {
	i := 9
	if true {
		i := i + 1
		print_int(i)
	}
	print_int(i)
}
