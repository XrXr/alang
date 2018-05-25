main :: proc () {
	sum := 0
	for i := 1..100 {
		if i == 73 {
			continue
		}
		sum = sum + i
	}
	if sum == (5050 - 73) {
		puts("continue works\n")
	} else {
		puts("Bad\n")
	}
	sum = 0
	for i := 1..100 {
		sum = sum + i
		if i == 20 {
			break
		}
	}
	if sum == 210 {
		puts("break works\n")
	} else {
		puts("Bad\n")
	}
}