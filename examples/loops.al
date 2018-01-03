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

    fib2last := 0
    fib2 := 1
    for 1..55 {
        next := fib2 + fib2last
        fib2last = fib2
        fib2 = next
    }

    puts("exit code should be zero\n")
    exit(fib-fib2)
}
