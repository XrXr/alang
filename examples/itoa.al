// A convoluted way to convert numbers to strings
main :: proc () {
    buffer := asciiNumber()

    itoa(18446744073709551615, &buffer)
    puts(&buffer)
    puts("\n")

    itoa(1337, &buffer)
    puts(&buffer)
    puts("\n")

    itoa(8382, &buffer)
    puts(&buffer)
    puts("\n")

    itoa(65536, &buffer)
    puts(&buffer)
    puts("\n")
}

struct asciiNumber {
    length int
    buffer [20]u8
}

itoa :: proc (number int, out *asciiNumber) {
    var digits [20]int
    out.length = 0
    for i := 0..19 {
        out.buffer[i] = 0
        digits[i] = 0
    }

    effectTable := binToDecTable()
    for i := 0..63 {
        if testbit(number, i) {
            for j := 0..18 {
                digits[j] = digits[j] + @(effectTable + (i * 19 + j))
            }
        }
    }
    // do the carray
    for i := 0..18 {
        if digits[i] > 9 {
            carry := digits[i] / 10
            mod := digits[i] - carry*10
            digits[i+1] = digits[i+1] + carry
            digits[i] = mod
        }
    }
    i := 19
    found := false
    mostSignificantDigit := 0
    for i >= 0 {
        if digits[i] > 0 {
            if found {
            } else {
                found = true
                mostSignificantDigit = i
            }
        }
        i = i - 1
    }
    out.length = mostSignificantDigit + 1
    if found {
        // write ascii in reverse
        for i := 0..mostSignificantDigit {
            out.buffer[mostSignificantDigit-i] = digits[i] + 48
        }
    } else {
        out.buffer[0] = 48
    }
}
