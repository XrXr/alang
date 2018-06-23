struct foo {
    josh int
    wendy int
    bob int
}

main :: proc() {
	a := foo()
	a.josh = 300
	a.wendy = 383
	a.bob = 999999

	b := a
	b.wendy = 777777

	ap := &a
	c := @ap
	c.josh = 333333
	c.bob = 111111

	var d foo
	dp := &d
	@dp = c
	d.wendy = 222222

	puts("a:\n")
	print_int(a.josh)
	print_int(a.wendy)
	print_int(a.bob)
	puts("b:\n")
	print_int(b.josh)
	print_int(b.wendy)
	print_int(b.bob)
	puts("c:\n")
	print_int(c.josh)
	print_int(c.wendy)
	print_int(c.bob)
	puts("d:\n")
	print_int(d.josh)
	print_int(d.wendy)
	print_int(d.bob)
}
