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

    fib3last := 0
    fib3 := 1
    i = 0
    for {
        if i >= 55 {
            break
        }
        next := fib3 + fib3last
        fib3last = fib3
        fib3 = next
        i += 1
    }

    puts("should be 0\n")
    print_int(fib-fib2+fib3-fib)
}
