struct foo {
    josh int
    wendy int
    kai *bar
}

struct bar {
    jolly int
    cooperation int
}

main :: proc() {
    a := foo()
    ap := &a
    b := bar()
    a.josh = 100
    ap.wendy = 67

    a.kai = &b
    a.kai.jolly = a.josh - a.wendy
    b.cooperation = 2
    puts("exit code should be 202\n")
    exit(ap.josh + a.wendy + a.kai.jolly + a.kai.cooperation)
}
