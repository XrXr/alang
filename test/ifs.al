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

    if 9 > 8 {
        puts("greater\n")
    }

    if 8 > 9 {
        puts("no\n")
    }

    if 3 < 2 {
        puts("no\n")
    }

    if 2 < 3 {
        puts("smaller\n")
    }

    if 7 <= 7 {
        puts("less equal equal\n")
    }

    if 7 <= 8 {
        puts("less equal lesser\n")
    }

    if 100 <= 12 {
        puts("no")
    }

    if 10 >= 10 {
        puts("greater equal equal\n")
    }

    if 12 >= 11 {
        puts("greater equal greater\n")
    }

    if 10 >= 12 {
        puts("no")
    }

    if 12 == 12 {
        puts("equal\n")
    }

    if 12 == 123 {
        puts("no\n")
    }
}
