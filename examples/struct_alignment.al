struct a {
	hoho bool
	faker s32
	wrench bool
	jojo int
	another bool
	fun s32
}

fillStruct :: foreign proc (*a)

main :: proc () {
	meow := a()

//	var hack s32
//	hack = 2912
//	meow.hoho = true
//	meow.faker = hack
//	meow.wrench = false
//	meow.jojo = 2821
//	meow.another = true

	fillStruct(&meow)

	print_int(meow.hoho)
	print_int(meow.faker)
	print_int(meow.wrench)
	print_int(meow.jojo)
	print_int(meow.another)
	print_int(meow.fun)
}