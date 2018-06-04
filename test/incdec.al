main :: proc() {
	a := 100
	a += 30
	a -= 27

	b := &a
	@b += 299
	@b -= 20
	if a == 382 {
		puts("Correct\n")
	}
}