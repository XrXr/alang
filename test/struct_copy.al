struct foo {
    josh int
    wendy int
    bob int
    arr [3]int
}

main :: proc() {
	a := foo()
	a.josh = 300
	a.wendy = 383
	a.bob = 999999
	a.arr[0] = 141414
	a.arr[1] = 969696
	a.arr[2] = 818181

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

	arrFromA := a.arr
	puts("arrFromA:\n")
	print_int(arrFromA[0])
	print_int(arrFromA[1])
	print_int(arrFromA[2])

	arrFromAp := ap.arr
	puts("arrFromAp:\n")
	print_int(arrFromAp[0])
	print_int(arrFromAp[1])
	print_int(arrFromAp[2])

	arrFromAp[0] = 777666
	arrFromAp[1] = 555444
	arrFromAp[2] = 333222
	b.arr = arrFromAp
	puts("b's arr after assigning:\n")
	print_int(b.arr[0])
	print_int(b.arr[1])
	print_int(b.arr[2])

	arrFromAp[0] = 717171
	arrFromAp[1] = 424242
	arrFromAp[2] = 191919
	ap.arr = arrFromAp
	puts("a's arr after assigning through ar:\n")
	print_int(a.arr[0])
	print_int(a.arr[1])
	print_int(a.arr[2])
}
