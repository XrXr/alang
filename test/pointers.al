main :: proc() {
    var byte u8
    byte = 253
    a := 3
    ap := &a
    bytep := &byte
    @ap = 80
    @ap = @ap + 80
    @bytep = 123
    a = a - 3
    a = a - byte
    puts("should be 34\n")
    print_int(a)
}
