struct foo {
    josh int
    wendy int
}

main :: proc() {
    a := foo()
    b := &a
    a.josh = 100
    b.wendy = 67
    puts("exit code should be 167\n")
    exit(b.josh + b.wendy)
}
