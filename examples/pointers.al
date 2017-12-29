main :: proc() {
    a := 3
    b := &a
    *b = 80
    *b = *b + 80
    a = a - 3
    puts("exit code should be 157\n")
    exit(a)
}
