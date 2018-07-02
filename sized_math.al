main :: proc () {
	puts("assigns\n")
	big := 228282982198
	var a s32
	var dword u32
	var word u16
	var byte u8
	var byte_signed s8
	var full s64

	// sign extension and zero extension
	a = -1
	byte = 255
	a = byte
	print_int(a)

	a = -1
	word = 65535
	a = word
	print_int(a)

	full = -1
	dword = 4294967295
	full = dword
	print_int(full)

	a = -1
	byte_signed = -123
	a = byte_signed
	print_int(a)

	puts("\naddition\n")

	a = 626
	print_int(a)
	a += big
	print_int(a)

	byte = 232
	byte += big
	print_int(byte)

	b := big
	a = -382
	b = b + a
	print_int(b)

	byte_signed = -122
	b = big
	b += byte_signed
	print_int(b)

	puts("\nsubtration:\n")

	a = 626
	a -= big
	print_int(a)

	byte = 232
	byte -= big
	print_int(byte)

	b = big
	a = -382
	b = b - a
	print_int(b)

	byte_signed = -122
	b = big
	b -= byte_signed
	print_int(b)

	puts("\nmultiplication:\n")

	full = 251
	a = -1234125
	full = full * a
	print_int(full)

	a = 251
	byte = 143
	a = a * byte
	print_int(a)

	a = 251
	byte_signed = -112
	a = a * byte_signed
	print_int(a)

	byte = 82
	byte = byte * big
	// the result is funny because after the multiply there is
	// junk in the upper half of the register. It will be fixed when we type check function call arguments
	print_int(byte)

	puts("\ndivision:\n")

	byte = 29
	b = 3
	byte = byte / b
	print_int(byte)

	a = 281924
    // 96 * 4 = 384 which is -128 in s8 with one extra bit. Testing sign extenion.
	byte_signed = 96
	byte_signed = byte_signed * 4
	print_int(byte_signed)
	a = a / byte_signed
	print_int(a)
}
