struct point {
	x int
	y int
}

struct shelf {
	id int
	foo [3]russian
}

struct russian {
    a int
    b doll
    c int
}

struct doll {
	bar int
	back *shelf
	box [2]point
}

main :: proc () {
    var arr [3]point
    arr[2].x = 100
    arr[2].y = 7171
    print_int(arr[2].x)
    print_int(arr[2].y)

    var room [2]shelf

    room[0].id = 1
    room[1].id = 2

    room[0].foo[2].a = 79797979
    room[0].foo[2].c = 91919191
    room[0].foo[2].b.box[1].x = 565656
    room[0].foo[2].b.back = &room
    room[0].foo[2].b.back += 1
    puts("The id of room[1] is: ")
    print_int(room[0].foo[2].b.back.id)
    // jump from shelf 0 to shelf 1 and then back again
              room[0].foo[2].b.back.foo[1].b.back = &room
              room[0].foo[2].b.back.foo[1].b.back.foo[2].b.box[1].y = 726
    print_int(room[0].foo[2].b.back.foo[1].b.back.foo[2].a)
    print_int(room[0].foo[2].b.back.foo[1].b.back.foo[2].c)
    print_int(room[0].foo[2].b.back.foo[1].b.back.foo[2].b.box[1].x)

    print_int(room[0].foo[2].b.box[1].y)
}
