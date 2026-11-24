package main

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestExitCodePreservesChildStatus(t *testing.T) {
	err := exec.Command("sh", "-c", "exit 7").Run()
	if got := exitCode(err); got != 7 {
		t.Fatalf("exitCode=%d, want 7", got)
	}
}

func TestVersionFlag(t *testing.T) {
	if err := run([]string{"--version"}); err != nil {
		t.Fatal(err)
	}
}

func TestCopyCommandReportsClipboardFailure(t *testing.T) {
	original := writeClipboard
	t.Cleanup(func() { writeClipboard = original })
	writeClipboard = func(string) error { return errors.New("no clipboard") }
	err := copyCommand("printf safe")
	if err == nil || !strings.Contains(err.Error(), "no clipboard") {
		t.Fatalf("err=%v", err)
	}
	writeClipboard = func(string) error { return nil }
	if err := copyCommand("printf safe"); err != nil {
		t.Fatal(err)
	}
}
