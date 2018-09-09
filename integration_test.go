// +build integration

package main_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"testing"
)

func TestHandMadeCases(t *testing.T) {
	gopath := os.Getenv("GOPATH")
	fixturePath := path.Join(gopath, "src/github.com/XrXr/alang/test")
	files, err := ioutil.ReadDir(fixturePath)
	if err != nil {
		t.Error("Failed to list files in fixture directory")
		return
	}
	for _, f := range files {
		name := f.Name()
		if path.Ext(name) != ".al" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			sourcePath := path.Join(fixturePath, name)
			outFixturePath := sourcePath + ".out"
			expected, err := ioutil.ReadFile(outFixturePath)
			if err != nil {
				t.Fatal(err)
			}
			compileAndAssertOutput(t, sourcePath, expected)
		})
	}
}

var signedTypes = []string{"s8", "s16", "s32", "s64"}
var unsignedTypes = []string{"u8", "u16", "u32", "u64"}

func TestIndirectSignAndZeroExtensions(t *testing.T) {
	for smallerSize := 8; smallerSize < 64; smallerSize *= 2 {
		smallerSigned := fmt.Sprintf("s%d", smallerSize)
		smallerUnsigned := fmt.Sprintf("u%d", smallerSize)
		for biggerSize := smallerSize * 2; biggerSize <= 64; biggerSize *= 2 {
			biggerSigned := fmt.Sprintf("s%d", biggerSize)
			biggerUnsigned := fmt.Sprintf("u%d", biggerSize)
			t.Run(fmt.Sprintf("Load *%s into %s", smallerSigned, biggerSigned), makeIndirectLoadFromSmall(smallerSigned, biggerSigned, -107))
			t.Run(fmt.Sprintf("Load %s into *%s", smallerSigned, biggerSigned), makeIndirectWriteToBig(smallerSigned, biggerSigned, -98))

			t.Run(fmt.Sprintf("Load *%s into %s", smallerUnsigned, biggerUnsigned), makeIndirectLoadFromSmall(smallerUnsigned, biggerUnsigned, 201))
			t.Run(fmt.Sprintf("Load %s into *%s", smallerUnsigned, biggerUnsigned), makeIndirectWriteToBig(smallerUnsigned, biggerUnsigned, 201))
		}
	}
}

func makeIndirectLoadFromSmall(smallerType string, biggerType string, magicConstant int) func(*testing.T) {
	const template = `
	main :: proc () {
		var a %[1]s
		var b %[2]s
		ap := &a
		a = %[3]d
		b = -1

		b = @ap
		if b == %[3]d {
			puts("good\n")
		}

		testPointerOriginUnkonwn(ap)
	}

	testPointerOriginUnkonwn :: proc (small *%[1]s) {
		var big %[2]s
		big = -1
		big = @small
		if big == %[3]d {
			puts("good2\n")
		}
	}
`

	program := fmt.Sprintf(template, smallerType, biggerType, magicConstant)
	return func(t *testing.T) {
		testSourceString(t, program, []byte("good\ngood2\n"))
	}
}

func makeIndirectWriteToBig(smallerType string, biggerType string, magicConstant int) func(*testing.T) {
	const template = `
	main :: proc () {
		var a %s
		var b %s
		bp := &b
		a = %[3]d
		b = -1

		@bp = a
		if b == %[3]d {
			puts("good\n")
		}

		b = -1
		writeSmallTo(bp, %[3]d)
		if b == %[3]d {
			puts("good2\n")
		}
	}

	writeSmallTo :: proc (ptr *%[2]s, constant %[1]s) {
		@ptr = constant
	}
`
	program := fmt.Sprintf(template, smallerType, biggerType, magicConstant)
	return func(t *testing.T) {
		testSourceString(t, program, []byte("good\ngood2\n"))
	}
}

func testSourceString(t *testing.T, program string, expectedOutput []byte) {
	sourcePath := path.Join(os.TempDir(), "alang_test_source")

	sourceFile, err := os.OpenFile(sourcePath, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		t.Fatal("Failed to open source file for writing")
	}
	err = sourceFile.Truncate(0)
	if err != nil {
		t.Fatal("Failed to truncate the source file before writing")
	}
	_, err = sourceFile.WriteString(program)
	if err != nil {
		t.Fatal("Failed to write source code to file")
	}
	sourceFile.Close()
	compileAndAssertOutput(t, sourcePath, expectedOutput)
}

func compileAndAssertOutput(t *testing.T, sourcePath string, expected []byte) {
	gopath := os.Getenv("GOPATH")
	binPath := path.Join(os.TempDir(), "alang_test_bin")
	compilerCommand := exec.Command(path.Join(gopath, "bin", "alang"), "-o", binPath, sourcePath)
	compilerOutput, err := compilerCommand.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to compile. Compiler output:\n%s\n", compilerOutput)
	}
	compiled := exec.Command(binPath)
	outPipe, err := compiled.StdoutPipe()
	if err != nil {
		t.Fatal("Failed to create pipe to the compiled command", err)
	}
	err = compiled.Start()
	if err != nil {
		t.Fatal("Failed to start the compiled executable")
	}
	outBytes, err := ioutil.ReadAll(outPipe)
	if err != nil {
		t.Fatal("Failed to read from stdout of compiled binary")
	}
	err = compiled.Wait()
	if err != nil {
		t.Fatal("Compiled executable failed to finish")
	}
	defer os.Remove(binPath)

	if !bytes.Equal(outBytes, expected) {
		t.Fatal("Compiled binary's output differs from expectation")
	}
}

func TestMain(m *testing.M) {
	log.SetFlags(0)
	gopath := os.Getenv("GOPATH")
	if len(gopath) == 0 {
		log.Fatalln("GOPATH environmental variable not set")
	}
	log.Println("Running go install on the compiler...")
	cmd := exec.Command("go", "install", "github.com/XrXr/alang")
	err := cmd.Start()
	if err != nil {
		log.Fatalln("Failed to start `go install`")
	}
	err = cmd.Wait()
	if err != nil {
		log.Fatalln("`go install` failed to complete")
	}
	os.Exit(m.Run())
}
