main :: proc () {
    // make sure we are not clobbering anything
    a := 50
    var b [3]u8
    b [] 0 = 100
    b [] 1 = 255
    b [] 2 = 150 + 7

    if b [] 0 == 100 {
        puts("good 1/2\n")
    } else {
        puts("bad\n")
    }

    if b [] 1 == 255 {
        puts("good 2/3\n")
    } else {
        puts("bad\n")
    }

    if b [] 2 == 157 {
        puts("good 3/3\n")
    } else {
        puts("bad\n")
    }

    puts("exit code should be 50\n")
    exit(a)
}
