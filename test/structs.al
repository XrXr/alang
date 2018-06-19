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

    print_int(a.josh)
    print_int(ap.josh)

 //   b := bar()
 //   a.josh = 100
 //   ap.wendy = 67

 //   a.kai = &b
 //   a.kai.jolly = a.josh - a.wendy
 //   b.cooperation = 2
 //   puts("should be 202\n")
 //   print_int(ap.josh + a.wendy + a.kai.jolly + a.kai.cooperation)
}
