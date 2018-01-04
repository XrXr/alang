main :: proc() {
    var array [5]int
    base := cast(&array)
    @(base +  0) = 100
    @(base +  1) = 25
    @(base +  2) = 2
    @(base +  3) = 3
    @(base +  4) = 1

    ret := @(base + 0)
    ret = ret / @(base + 1)
    ret = ret / @(base + 2)
    ret = ret * @(base + 3)
    ret = ret - @(base + 4)
    puts("exit code should be 5\n")
    exit(ret)
}
