main :: proc() {
    var array [5]int
    array [] 0 = 100
    array [] 1 = 25
    array [] 2 = 2
    array [] 3 = 3
    array [] 4 = 1

    ret := array [] 0
    ret = ret / array [] 1
    ret = ret / array [] 2
    ret = ret * array [] 3
    ret = ret - array [] 4
    puts("exit code should be 5\n")
    exit(ret)
}
