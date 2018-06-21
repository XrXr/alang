// go build; and ./alang -c -libc xhello.al; and gcc -g -no-pie a.o -lX11

struct XDisplay {
	ext_data *void
	private1 *void
	fd s32
	private2 s32
	proto_major_version s32
	proto_minor_version s32
	vendor *u8
	private3 u64
	private4 u64
	private5 u64
	private6 s32
	resource_alloc *void
	byte_order s32
	bitmap_unit s32
	bitmap_pad s32
	bitmap_bit_order s32
	nformats s32
	pixmap_format *void
	private8 s32
	release s32
	private9 *void
	private10 *void
	qlen s32
	last_request_read u64
	request u64
	private11 *u8
	private12 *u8
	private13 *u8
	private14 *u8
	max_request_size u32
	db *void
	private15 *void
	display_name *u8
	default_screen s32
	nscreens s32
	screens *Screen
	motion_buffer u64
	private16 u64
	min_keycode s32
	max_keycode s32
	private17 *u8
	private18 *u8
	private19 s32
	xdefaults *u8

}

struct Screen {
	ext_data *void
	display *XDisplay
	root u64
	width s32
	height s32
	mwidth s32
	mheight s32
    ndepths s32
	depths *void
	root_depth s32
	root_visual *void
	default_gc *void
	cmap u64
	white_pixel u64
	black_pixel u64
	max_maps s32
	min_maps s32
	backing_store s32
	save_unders s32
	root_input_mask s64
}

struct XEvent {
	type s32
	fill [188]u8
}

XOpenDisplay :: foreign proc (name *u8) -> *XDisplay

XCreateSimpleWindow :: foreign proc (display *XDisplay, window u64, x s32, y s32, width u32, height u32, border_width u32, border u64, background u64) -> u64

XMapWindow :: foreign proc (display *XDisplay, w u64) -> s32

XNextEvent :: foreign proc (display *XDisplay, event *XEvent) -> s32

XFillRectangle :: foreign proc (display *XDisplay, d u64, gc *void, x s32, y s32, width u32, height u32) -> s32

XDrawString :: foreign proc (display *XDisplay, d u64, gc *void, x s32, y s32, str *u8, length s32) -> s32

XSelectInput :: foreign proc (display *XDisplay, w u64, event_mask s64) -> int

XCloseDisplay :: foreign proc (display *XDisplay) -> s32

main :: proc () {
	// hack to get a pointer to XDisplay without the compiler freaking out
	hack := XDisplay()
	d := &hack
	var w u64
	e := XEvent()
	msg := "he's done it"

	var s s32

  	d = XOpenDisplay(0)
  	if !d {
  		puts("Can't open display\n")
  		exit(1)
  	}

  	s = d.default_screen
  	screen := d.screens + s
  	rootWindow := screen.root

  	gc := screen.default_gc

    w = XCreateSimpleWindow(d, rootWindow, 100, 100, 500, 500, 1, 777215, 111111)
    XSelectInput(d, w, 32769)
  	XMapWindow(d, w)

  	Expose := 12
  	KeyPress := 2
  	for true {
		XNextEvent(d, &e)
		if e.type == Expose {
        	XFillRectangle(d, w, gc, 20, 20, 10, 10)
        	XDrawString(d, w, gc, 10, 50, msg.data, msg.length)
		}
		if e.type == KeyPress {
			break
		}
  	}

   XCloseDisplay(d)
}
