main :: proc () {
    foo := false
    if foo {
        puts("I should not be printed")
    }

    bar := true
    if bar {
        puts("bar\n")
    }

    if true {
        puts("literal\n")
    }
}
