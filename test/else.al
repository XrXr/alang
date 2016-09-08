main :: proc () {
    if true {
        puts("print me\n")
    }
    else {
        puts("ignore me")
    }
    if false {
        puts("not printed\n")
    } else {
        puts("printed\n")
    }
}
