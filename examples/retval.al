main :: proc () {
    puts("exit should be 27\n")
    fr := foo()
    br := bar()
    r := fr + br
    exit(r)
}

foo :: proc () -> int {
    a := 2 + 3
    b := 3
    return a + b
}

bar :: proc () -> int {
    return 19
}
