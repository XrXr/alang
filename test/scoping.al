main :: proc () {
    a := "outter a\n"
    c := "sea"
    puts(a)
    if true {
        a := "inner a\n"
        puts(a)
        a = "modified inner a\n"
        b := "shouldn't print"

        if true {
            puts(a)
            if true {
                b = "must go deeper!\n"
                if true {
                    puts(b)
                    c = "reaching out\n"
                }
            }
        }
    }
    puts(a)
    puts(c)
}
