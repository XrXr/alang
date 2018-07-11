main :: proc () {
	var arr [1]int
	foo(2323, &arr, "I'm a string", false)
}

foo :: proc(foo *string, bar *[500]int, baz s16, ball u8) {
}
