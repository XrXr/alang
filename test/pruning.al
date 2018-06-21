main :: proc () {
    a := 0
    b := a
    c := b
    d := c
    e := d
    print_int(e)
    f := 1
    g := 2
    a = f + g
    print_int(a)
}
