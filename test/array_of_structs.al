struct pack {
    a int
    b int
}

main :: proc () {
    var arr [20]pack

    var arrPtr *pack
    arrPtr = &arr

    (arrPtr+2).a = 100
    print_int(arr[2].a)
}
