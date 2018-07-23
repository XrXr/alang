main :: proc () {
    a := true
    b := true
    if a && b {
        puts("basic 1\n")
    }
    c := a && false
    if c == false {
        puts("basic 2\n")
    }
    d := false && tripwire() && tripwire() && tripwire()
    if d == false {
        puts("shortcircuit 3\n")
    }
    e := tick1() && tick2() && tick3() && tripwire()
    if e == false {
        puts("shortcircuit 4\n")
    }
}

tripwire :: proc () -> bool {
    puts("bad\n")
    return true
}

tick1 :: proc () -> bool {
    puts("tick 1\n")
    return true
}

tick2 :: proc () -> bool {
    puts("tick 2\n")
    return true
}

tick3 :: proc () -> bool {
    puts("tick 3, return false\n")
    return false
}
