## `alang`

`alang` is a toy compiler written for a statically typed langugage similar to C in terms of features.
`alang` only targets 64-bit X86-64 Linux and generates free standing binaries that don't depend on libc by default.

## What does the lanaguage look like?

[souvenir](https://github.com/XrXr/souvenir) excersises all of alang's features. Please take a look if you are interested.

## Dependencies

- `import "github.com/davecgh/go-spew/spew"` for printf debugging during development
- `nasm` as a runtime dependency for assembling object files
- `ld` for linking

## Notes

- Run `go test -tags integration` to run integration tests
- Run `go generate parsing/*go` to get proper parse tree printing
- Run `go generate ir/*go` to get proper ir printing

Here is what the compiler does to generate a binary:
  `parse into ast -> frontend generates ir -> type check/inference happens on ir -> generate nasm asm -> pass asm to nasm and run ld`

- Instructions from ir operate on variables numbered from 0. Variables can be of any size
- The first few variables are procedure arguments
- While most irs have all the variables they use in the main struct body, there are some exceptions. See `frontend.Prune()`. The Extra field of ir.Inst comes in handy

