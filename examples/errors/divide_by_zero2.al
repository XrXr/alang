main :: proc () {
	// even though we don't evaluate this call until runtime, we should still be able to catch this
	a := foo() / 0
}

foo :: proc () -> int {
	return 3828
}
