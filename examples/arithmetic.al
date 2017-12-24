main :: proc() {
    puts("exit code should be 49\n")
    a := (3 - 23) * 20
    a = 283 + 2347 + a
    b := 83
    a = a / b + 23
    exit(a)
}
