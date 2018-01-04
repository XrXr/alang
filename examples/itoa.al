main :: proc () {
    buffer := asciiNumber()
    itoa(18446744073709551615, &buffer)
    puts(&buffer)
    itoa(1337, &buffer)
    puts(&buffer)
    itoa(8382, &buffer)
    puts(&buffer)
    itoa(65536, &buffer)
    puts(&buffer)
}

struct asciiNumber {
    length int
    buffer [20]u8
}

itoa :: proc (number int, out *asciiNumber) {
    out.length = 0
    for i := 0..19 {
        out.buffer [] i = 0
    }

    effectTable := binToDecTable()
    var digits [20]int
    for i := 0..63 {
        if testbit(number, i) {
            for j := 0..18 {
                digits [] j = (digits [] j) + @(effectTable + (i * 19 + j))
            }
        }
    }
    // do the carray
    for i := 0..18 {
        if digits[i] > 9 {
            carry := digits [] i / 10
            mod := (digits [] i) - carry*10
            digits [] (i+1) = (digits [] (i+1)) + carry
            digits [] i = mod
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
        for i..mostSignificantDigit {
            outBuf[](mostSignificantDigit-i) = (digits[]i) + 48
        }
    }
}
