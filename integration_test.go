// +build integration

package main_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"testing"
)

func TestIntegration(t *testing.T) {
	gopath := os.Getenv("GOPATH")
	if len(gopath) == 0 {
		t.Error("GOPATH environmental variable not set")
		return
	}
	fmt.Println("running go install...")
	cmd := exec.Command("go", "install")
	err := cmd.Start()
	if err != nil {
		t.Error("Failed to start `go install`")
		return
	}
	err = cmd.Wait()
	if err != nil {
		t.Error("`go install` failed to complete")
		return
	}

	fixturePath := path.Join(gopath, "src/github.com/XrXr/alang/test")
	files, err := ioutil.ReadDir(fixturePath)
	if err != nil {
		t.Error("Failed to list files in fixture directory")
		return
	}
	compiledBinPath := path.Join(os.TempDir(), "alang_integration_bin")
	for _, f := range files {
		name := f.Name()
		if path.Ext(name) != ".al" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			inFixturePath := path.Join(fixturePath, name)
			outFixturePath := inFixturePath + ".out"
			expected, err := ioutil.ReadFile(outFixturePath)
			if err != nil {
				t.Error(err)
				return
			}
			compilerCommand := exec.Command(path.Join(gopath, "bin", "alang"), "-o", compiledBinPath, inFixturePath)
			err = compilerCommand.Start()
			if err != nil {
				t.Error("Failed to invoke compiler for")
				return
			}
			err = compilerCommand.Wait()
			if err != nil {
				t.Error("Failed to compile")
				return
			}
			compiled := exec.Command(compiledBinPath)
			out, err := compiled.StdoutPipe()
			if err != nil {
				t.Error("Failed to create pipe to the compiled command", err)
				return
			}
			err = compiled.Start()
			if err != nil {
				t.Error("Failed to start the compiled executable")
				return
			}
			outBytes, err := ioutil.ReadAll(out)
			if err != nil {
				t.Error("Failed read from stdout of compiled binary")
				return
			}
			err = compiled.Wait()
			if err != nil {
				t.Error("Compiled executable failed to finish")
				return
			}
			if !bytes.Equal(outBytes, expected) {
				t.Error("Output from compiled binary is incorrect")
				return
			}
			err = os.Remove(compiledBinPath)
			if err != nil {
				t.Error("Failed to clean out compiled binary")
			}
		})
	}
}
