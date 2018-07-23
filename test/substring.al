main :: proc () {
	match("so", "jjjjjjjjsojjjjj")
	match("so", "jjjjjjjjs")
	match("so", "jjojjjsjjjs")
}

// I know the code can be simpler, but this is a regression test
// so please don't edit this proc
match :: proc (sub string, whole string) {
	match := false
	for i := 0..whole.length-sub.length {
		substringMatch := true
		for j := 0..sub.length-1 {
			if @(sub.data + j) != @(whole.data + i + j) {
				substringMatch = false
			}
		}
		if substringMatch {
			match = true
			break
		}
	}
	puts(sub)
	puts(" is ")
	if !match {
		puts("not ")
	}
	puts("a substring of ")
	puts(whole)
	puts("\n")
}
