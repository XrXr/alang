struct foo {
    josh int
    wendy int
}

main :: proc() {
    a := foo()
    a.josh = 100
    a.wendy = 67
    puts("exit code should be 167\n")
    exit(a.josh + a.wendy)
}
