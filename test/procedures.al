main :: proc () {
    thisBetterWork()
    booleans(false, true, false, false)
    numbers(221, 2, 3, 9)
    qwordToByte(2333, false, true)
    qwordToWordToByte(4929, true, true, true, true, false)
}

thisBetterWork :: proc () {
    puts("no args 1/1\n")
}


booleans :: proc (a bool, b bool, c bool, d bool) {
    if a {
        puts("bad\n")
    }
    if b {
        puts("bools 1/1\n")
    }
    if c {
        puts("bad\n")
    }
    if d {
        puts("bad\n")
    }
}

numbers :: proc (a int, b int, c int, d int) {
    if a == 221 {
        puts("numbers 1/4\n")
    }
    if b == 2 {
        puts("numbers 2/4\n")
    }
    if c == 3 {
        puts("numbers 3/4\n")
    }
    if d == 9 {
        puts("numbers 4/4\n")
    }
}

qwordToByte :: proc (a int, b bool, c bool) {
    if a == 2333 {
        puts("qwordToByte 1/3\n")
    }
    if b {
        puts("no!\n")
    } else {
        puts("qwordToByte 2/3\n")
    }

    if c {
        puts("qwordToByte 3/3\n")
    }
}

qwordToWordToByte :: proc (a int, b bool, c bool, d bool, e bool, f bool) {
    if a == 4929 {
        puts("qwordToWordToByte 1/6\n")
    }
    if b {
        puts("qwordToWordToByte 2/6\n")
    }
    if c {
        puts("qwordToWordToByte 3/6\n")
    }
    if d {
        puts("qwordToWordToByte 4/6\n")
    }
    if e {
        puts("qwordToWordToByte 5/6\n")
    }
    if f {
        puts("bad!\n")
    } else {
        puts("qwordToWordToByte 6/6\n")
    }
}
