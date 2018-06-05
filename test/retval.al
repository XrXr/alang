main :: proc () {
    // do this first to see if it corrupts the stack
    arr := retarray()
    assertBool(arr[2])
    assertBool(arr[10])
    assertBool(arr[20])
    assertBool(arr[25])
    assertBool(arr[30])

    for i := 0..30 {
        if arr[i] && !(i == 2 || i == 10 || i == 20 || i == 25 || i == 30) {
            puts("bad! Everything else should be false\n")
        }
    }

    a := foo(3) + bar()
    if a == 27 {
        puts("simple values pass\n")
    } else {
        puts("simple values fail\n")
    }

    if rettrue() {
        if retfalse() == false {
            puts("simple bool pass\n")
        } else {
            puts("simple bool fail\n")
        }
    } else {
        puts("simple bool fail")
    }
}

retarray :: proc () -> [31]bool {
    var arr [31]bool
    for i := 0..30 {
        arr[i] = false
    }
    arr[2] = true
    arr[10] = true
    arr[20] = true
    arr[25] = true
    arr[30] = true
    return arr
}

foo :: proc (para int) -> int {
    a := 2 + 3
    return a + para
}

bar :: proc () -> int {
    return 19
}

rettrue :: proc () -> bool {
    a := 38
    b := 2
    return b < a
}

retfalse :: proc () -> bool {
    a := 382
    b := 213
    return b > a
}

assertBool :: proc (a bool) {
    if a {
        puts("good\n")
    } else {
        puts("bad\n")
    }
}
