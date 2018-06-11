package main

import (
	"fmt"
	"io"
)

func writeAssemblyPrologue(out io.Writer) {
	fmt.Fprintln(out, `global _start
	section .text
_start:
	call proc_main
	xor rdi, rdi
	jmp proc_exit
	`)
}

func writeLibcPrologue(out io.Writer) {
	// sub rsp, 8 to align rsp
	fmt.Fprintln(out, `global main
	section .text
main:
	sub rsp, 8
	call proc_main
	xor rax, rax
	add rsp, 8
	ret`)
}

func writeBuiltins(out io.Writer) {
	fmt.Fprintln(out, `proc_exit:
	mov eax, 60
	syscall

proc_testbit:
    xor rcx, rcx
    clc
    bt rdi, rsi
    adc rcx, 0
    mov rax, rcx
    ret

proc_puts:
	mov rdx, [rdi]
	lea rsi, [rdi+8]
	mov rax, 1
	mov rdi, 1
	syscall
	ret

proc_print_int:
	push rbp
	mov rbp, rsp
	sub rsp, 29

	xor r9, r9
	xor r10, r10
	mov rcx, 10000000000000000000
	lea rbx, [rbp-21]
	mov rax, rdi
.divide:
; divide and store current digit
	xor rdx, rdx
	div rcx
	mov r8, rdx
	cmp r10, 0
	jnz .write_ascii 
	cmp rax, 0
	jz .write_ascii
	inc r10
	mov r9, rbx
.write_ascii:
	add al, 48
	mov [rbx], al
; divide the dividen by 10
	mov rax, rcx
	mov rcx, 10
	xor rdx, rdx
	div rcx
	mov rcx, rax
	mov rax, r8
	inc rbx
	cmp rcx, 0
	jnz .divide

	cmp r10, 0
	jnz .shorten
	mov byte[rbp-1],10
	mov qword[rbp-10],2
	lea rdi, [rbp-10]
	jmp .end
.shorten:
	mov byte [rbx], 10
	mov rax, rbx
	sub rax, r9
	inc rax
	sub r9, 8
	mov [r9], rax
	mov rdi, r9
.end:
	call proc_puts

	mov rsp, rbp
	pop rbp
	ret

; rdi is dest, rsi is source, rdx is size
_intrinsic_memcpy:
	mov rcx, rdx
	cld
	rep movsb
	ret`)
}

func writeDecimalTable(out io.Writer) {
	fmt.Fprintln(out, `
_binToDecTable:
	dq 1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0
	dq 2,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0
	dq 4,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0
	dq 8,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0
	dq 6,1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0
	dq 2,3,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0
	dq 4,6,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0
	dq 8,2,1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0
	dq 6,5,2,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0
	dq 2,1,5,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0
	dq 4,2,0,1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0
	dq 8,4,0,2,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0
	dq 6,9,0,4,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0
	dq 2,9,1,8,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0
	dq 4,8,3,6,1,0,0,0,0,0,0,0,0,0,0,0,0,0,0
	dq 8,6,7,2,3,0,0,0,0,0,0,0,0,0,0,0,0,0,0
	dq 6,3,5,5,6,0,0,0,0,0,0,0,0,0,0,0,0,0,0
	dq 2,7,0,1,3,1,0,0,0,0,0,0,0,0,0,0,0,0,0
	dq 4,4,1,2,6,2,0,0,0,0,0,0,0,0,0,0,0,0,0
	dq 8,8,2,4,2,5,0,0,0,0,0,0,0,0,0,0,0,0,0
	dq 6,7,5,8,4,0,1,0,0,0,0,0,0,0,0,0,0,0,0
	dq 2,5,1,7,9,0,2,0,0,0,0,0,0,0,0,0,0,0,0
	dq 4,0,3,4,9,1,4,0,0,0,0,0,0,0,0,0,0,0,0
	dq 8,0,6,8,8,3,8,0,0,0,0,0,0,0,0,0,0,0,0
	dq 6,1,2,7,7,7,6,1,0,0,0,0,0,0,0,0,0,0,0
	dq 2,3,4,4,5,5,3,3,0,0,0,0,0,0,0,0,0,0,0
	dq 4,6,8,8,0,1,7,6,0,0,0,0,0,0,0,0,0,0,0
	dq 8,2,7,7,1,2,4,3,1,0,0,0,0,0,0,0,0,0,0
	dq 6,5,4,5,3,4,8,6,2,0,0,0,0,0,0,0,0,0,0
	dq 2,1,9,0,7,8,6,3,5,0,0,0,0,0,0,0,0,0,0
	dq 4,2,8,1,4,7,3,7,0,1,0,0,0,0,0,0,0,0,0
	dq 8,4,6,3,8,4,7,4,1,2,0,0,0,0,0,0,0,0,0
	dq 6,9,2,7,6,9,4,9,2,4,0,0,0,0,0,0,0,0,0
	dq 2,9,5,4,3,9,9,8,5,8,0,0,0,0,0,0,0,0,0
	dq 4,8,1,9,6,8,9,7,1,7,1,0,0,0,0,0,0,0,0
	dq 8,6,3,8,3,7,9,5,3,4,3,0,0,0,0,0,0,0,0
	dq 6,3,7,6,7,4,9,1,7,8,6,0,0,0,0,0,0,0,0
	dq 2,7,4,3,5,9,8,3,4,7,3,1,0,0,0,0,0,0,0
	dq 4,4,9,6,0,9,7,7,8,4,7,2,0,0,0,0,0,0,0
	dq 8,8,8,3,1,8,5,5,7,9,4,5,0,0,0,0,0,0,0
	dq 6,7,7,7,2,6,1,1,5,9,9,0,1,0,0,0,0,0,0
	dq 2,5,5,5,5,2,3,2,0,9,9,1,2,0,0,0,0,0,0
	dq 4,0,1,1,1,5,6,4,0,8,9,3,4,0,0,0,0,0,0
	dq 8,0,2,2,2,0,3,9,0,6,9,7,8,0,0,0,0,0,0
	dq 6,1,4,4,4,0,6,8,1,2,9,5,7,1,0,0,0,0,0
	dq 2,3,8,8,8,0,2,7,3,4,8,1,5,3,0,0,0,0,0
	dq 4,6,6,7,7,1,4,4,7,8,6,3,0,7,0,0,0,0,0
	dq 8,2,3,5,5,3,8,8,4,7,3,7,0,4,1,0,0,0,0
	dq 6,5,6,0,1,7,6,7,9,4,7,4,1,8,2,0,0,0,0
	dq 2,1,3,1,2,4,3,5,9,9,4,9,2,6,5,0,0,0,0
	dq 4,2,6,2,4,8,6,0,9,9,9,8,5,2,1,1,0,0,0
	dq 8,4,2,5,8,6,3,1,8,9,9,7,1,5,2,2,0,0,0
	dq 6,9,4,0,7,3,7,2,6,9,9,5,3,0,5,4,0,0,0
	dq 2,9,9,0,4,7,4,5,2,9,9,1,7,0,0,9,0,0,0
	dq 4,8,9,1,8,4,9,0,5,8,9,3,4,1,0,8,1,0,0
	dq 8,6,9,3,6,9,8,1,0,7,9,7,8,2,0,6,3,0,0
	dq 6,3,9,7,2,9,7,3,0,4,9,5,7,5,0,2,7,0,0
	dq 2,7,8,5,5,8,5,7,0,8,8,1,5,1,1,4,4,1,0
	dq 4,4,7,1,1,7,1,5,1,6,7,3,0,3,2,8,8,2,0
	dq 8,8,4,3,2,4,3,0,3,2,5,7,0,6,4,6,7,5,0
	dq 6,7,9,6,4,8,6,0,6,4,0,5,1,2,9,2,5,1,1
	dq 2,5,9,3,9,6,3,1,2,9,0,0,3,4,8,5,0,3,2
	dq 4,0,9,7,8,3,7,2,4,8,1,0,6,8,6,1,1,6,4
	dq 8,0,8,5,7,7,4,5,8,6,3,0,2,7,3,3,2,2,9
proc_binToDecTable:
	mov rdi, _binToDecTable
	ret`)
}
