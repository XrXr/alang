struct foo {
    byte u8
    josh int
    wendy int
    kai *bar
}

struct bar {
    jolly int
    word u16
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
    a.kai.word = 65535
    ap.byte = 255
    puts("should be 202\n")
    print_int(ap.josh + a.wendy + a.kai.jolly + a.kai.cooperation)
    print_int(a.byte)
    print_int(b.word)
}
