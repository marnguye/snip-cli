package cli

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/marnguye/snip-cli/internal/app"
	"github.com/marnguye/snip-cli/internal/state"
	"github.com/marnguye/snip-cli/internal/storage"
)

func legacyService(t *testing.T, dir string) *app.Service {
	t.Helper()
	service, err := app.New(state.Legacy, dir, t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func writeSnippet(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestExecuteCompatibility(t *testing.T) {
	dir := t.TempDir()
	service := legacyService(t, dir)
	writeSnippet(t, dir, "work.md", "## Two words\nprintf safe\n")
	cases := []struct {
		name                 string
		args                 []string
		wantFilter, wantFile string
		wantUI               bool
		wantErr              bool
	}{
		{"empty", nil, "", "", true, false},
		{"file fallback", []string{"work.md"}, "", "work.md", true, false},
		{"unknown fallback", []string{"unknown"}, "", "unknown", true, false},
		{"list alias", []string{"--list"}, "", "", false, false},
		{"completion helper", []string{"_list-descriptions"}, "", "", false, false},
		{"completion", []string{"completion"}, "", "", false, false},
		{"find", []string{"find", "two words", "ignored"}, "two words", "", true, false},
		{"find missing", []string{"find"}, "", "", false, true},
		{"run missing", []string{"run"}, "", "", false, true},
		{"add missing", []string{"add", "work.md"}, "", "", false, true},
		{"edit missing", []string{"edit", "work.md"}, "", "", false, true},
		{"remove missing", []string{"remove", "work.md"}, "", "", false, true},
		{"reload blocked", []string{"reload"}, "", "", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			filter, file, ui, err := Execute(tc.args, service)
			if filter != tc.wantFilter || file != tc.wantFile || ui != tc.wantUI || (err != nil) != tc.wantErr {
				t.Fatalf("got filter=%q file=%q ui=%t err=%v", filter, file, ui, err)
			}
		})
	}
}

func TestExecuteMutationsAndErrors(t *testing.T) {
	dir := t.TempDir()
	service := legacyService(t, dir)
	if _, _, _, err := Execute([]string{"add", "work.md", "two words", "printf safe", "ignored"}, service); err != nil {
		t.Fatal(err)
	}
	content, err := storage.ReadSnippet(dir, "work.md")
	if err != nil || !strings.Contains(content, "printf safe") {
		t.Fatalf("content=%q err=%v", content, err)
	}
	if _, _, _, err := Execute([]string{"edit", "work.md", "two words", "printf changed"}, service); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := Execute([]string{"remove", "work.md", "two words"}, service); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := Execute([]string{"add", "../outside.md", "unsafe", "printf safe"}, service); err == nil {
		t.Fatal("expected storage failure")
	}
}

func TestRunCommandMatchingAndExitStatus(t *testing.T) {
	dir := t.TempDir()
	service := legacyService(t, dir)
	writeSnippet(t, dir, "a.md", "## Match first\ntrue\n")
	writeSnippet(t, dir, "b.md", "## Match second\nexit 7\n")
	if err := RunCommand(service, "match"); err != nil {
		t.Fatalf("first legacy match should succeed: %v", err)
	}
	if err := RunCommand(service, "second"); err == nil {
		t.Fatal("expected child failure")
	} else {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
			t.Fatalf("want exit 7, got %v", err)
		}
	}
	if err := RunCommand(service, "missing"); err == nil {
		t.Fatal("expected missing snippet error")
	}
}

func TestDescriptionsAndPlaceholderInjectionRisk(t *testing.T) {
	dir := t.TempDir()
	writeSnippet(t, dir, "work.md", "## Two words\nprintf safe\n")
	var output bytes.Buffer
	command, err := resolvePlaceholders("printf '%s\\n' <value>", strings.NewReader("safe; printf injected\n"), &output)
	if err != nil {
		t.Fatal(err)
	}
	if command != "printf '%s\\n' safe; printf injected" {
		t.Fatalf("unexpected substitution %q", command)
	}
}
