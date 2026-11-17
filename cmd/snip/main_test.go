package main

import (
	"os/exec"
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
