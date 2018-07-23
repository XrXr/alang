main :: proc () {
    if true || true {
        puts("basic 1\n")
    }
    if false || true {
        puts("basic 2\n")
    }
    if false || false {
        tripwire()
    } else {
        puts("basic 3\n")
    }
    if true || tripwire() || tripwire() || tripwire() {
        puts("shortcircuit 4\n")
    }
    e := tick1() || tick2() || tick3() || tripwire()
    if e == true {
        puts("shortcircuit 5\n")
    }
}

tripwire :: proc () -> bool {
    puts("bad\n")
    return false
}

tick1 :: proc () -> bool {
    puts("tick 1\n")
    return false
}

tick2 :: proc () -> bool {
    puts("tick 2\n")
    return false
}

tick3 :: proc () -> bool {
    puts("tick 3, return true\n")
    return true
}
