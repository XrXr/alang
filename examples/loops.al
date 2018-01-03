main :: proc() {
    last := 0
    fib := 1
    i := 0
    for i < 55 {
        next := fib + last
        last = fib
        fib = next
        i = i + 1
    }
    puts("exit code should be zero\n")
    fib = fib - 225851433717
    exit(fib)
}
