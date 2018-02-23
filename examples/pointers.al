main :: proc() {
    var byte u8
    byte = 253
    a := 3
    b := &a
    c := &byte
    @b = 80
    @b = @b + 80
    @c = 123
    a = a - 3
    a = a - byte
    puts("exit code should be 34\n")
    exit(a)
}
