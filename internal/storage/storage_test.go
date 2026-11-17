package storage

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeSnippet(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestParseSnippetFile(t *testing.T) {
	snippets, err := ParseSnippetFile("# My Snippets\n\n## List files\nls -la\n\n## Remove file\nrm -f file.txt\n")
	if err != nil {
		t.Fatal(err)
	}
	want := []Snippet{{Description: "List files", Command: "ls -la"}, {Description: "Remove file", Command: "rm -f file.txt"}}
	if !reflect.DeepEqual(snippets, want) {
		t.Fatalf("got %#v, want %#v", snippets, want)
	}
}

func TestParseSnippetFileLongLineAndError(t *testing.T) {
	long := strings.Repeat("x", 70*1024)
	snippets, err := ParseSnippetFile("## Long\n" + long)
	if err != nil || len(snippets) != 1 || snippets[0].Command != long {
		t.Fatalf("long line: snippets=%d err=%v", len(snippets), err)
	}
	if _, err := ParseSnippetFile("## Too long\n" + strings.Repeat("x", maxSnippetLine+1)); err == nil {
		t.Fatal("expected scanner error")
	}
}

func TestAppendReadAndPrivatePermissions(t *testing.T) {
	dir := t.TempDir()
	if err := AppendSnippet(dir, "test.md", "New Command", "echo 'hello'"); err != nil {
		t.Fatal(err)
	}
	content, err := ReadSnippet(dir, "test.md")
	if err != nil {
		t.Fatal(err)
	}
	snippets, err := ParseSnippetFile(content)
	if err != nil || len(snippets) != 1 || snippets[0].Description != "New Command" {
		t.Fatalf("snippets=%#v err=%v", snippets, err)
	}
	info, err := os.Stat(filepath.Join(dir, "test.md"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%#o, want 0600", info.Mode().Perm())
	}
}

func TestInvalidCategoryNamesRejected(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"../outside.md", "/tmp/outside.md", "nested/file.md", `nested\\file.md`, "notes.txt", ".md"} {
		if err := AppendSnippet(dir, name, "safe", "printf safe"); err == nil {
			t.Errorf("%q accepted", name)
		}
	}
}

func TestRejectsSymlinkFileAndRoot(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	writeSnippet(t, dir, "target", "keep")
	if err := os.Symlink(target, filepath.Join(dir, "linked.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	for _, operation := range []func(string, string) error{
		func(d, n string) error { _, err := ReadSnippet(d, n); return err },
		func(d, n string) error { return AppendSnippet(d, n, "safe", "printf safe") },
		func(d, n string) error { return EditSnippet(d, n, "safe", "printf safe") },
		func(d, n string) error { return RemoveSnippet(d, n, "safe") },
	} {
		if err := operation(dir, "linked.md"); err == nil {
			t.Fatal("symlink accepted")
		}
	}
	if _, err := ListSnippets(dir); err == nil {
		t.Fatal("symlink listing accepted")
	}
	rootLink := filepath.Join(t.TempDir(), "root")
	if err := os.Symlink(dir, rootLink); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := AppendSnippet(rootLink, "safe.md", "safe", "printf safe"); err == nil {
		t.Fatal("symlink root accepted")
	}
}

func TestRejectsDirectoryTarget(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "folder.md"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadSnippet(dir, "folder.md"); err == nil {
		t.Fatal("directory accepted as snippet")
	}
	if _, err := ListSnippets(dir); err == nil {
		t.Fatal("directory accepted during listing")
	}
}

func TestEditAndRemovePreserveMarkdownCompatibility(t *testing.T) {
	dir := t.TempDir()
	writeSnippet(t, dir, "test.md", "# Test\n\n## First\n\ncmd1\n\n## Second\n\ncmd2\n")
	if err := EditSnippet(dir, "test.md", "First", "new_command"); err != nil {
		t.Fatal(err)
	}
	if err := RemoveSnippet(dir, "test.md", "Second"); err != nil {
		t.Fatal(err)
	}
	content, err := ReadSnippet(dir, "test.md")
	if err != nil {
		t.Fatal(err)
	}
	snippets, err := ParseSnippetFile(content)
	if err != nil || !reflect.DeepEqual(snippets, []Snippet{{Description: "First", Command: "new_command"}}) {
		t.Fatalf("snippets=%#v err=%v", snippets, err)
	}
}

func TestRemoveKeepsDuplicateCompatibility(t *testing.T) {
	dir := t.TempDir()
	writeSnippet(t, dir, "test.md", "## Same\none\n\n## Same\ntwo\n")
	if err := RemoveSnippet(dir, "test.md", "Same"); err != nil {
		t.Fatal(err)
	}
	content, err := ReadSnippet(dir, "test.md")
	if err != nil {
		t.Fatal(err)
	}
	snippets, err := ParseSnippetFile(content)
	if err != nil || len(snippets) != 0 {
		t.Fatalf("snippets=%#v err=%v", snippets, err)
	}
}

func TestWriteRejectsLineBreaksAndHashCommands(t *testing.T) {
	dir := t.TempDir()
	writeSnippet(t, dir, "test.md", "## Keep\nold\n")
	for _, c := range [][2]string{{"a\n## b", "x"}, {"a", "x\ny"}, {"a", "# note"}, {"", "x"}, {"a", " "}} {
		if err := AppendSnippet(dir, "test.md", c[0], c[1]); err == nil {
			t.Fatalf("append accepted %q", c)
		}
	}
	for _, command := range []string{"x\ny", "# note", " "} {
		if err := EditSnippet(dir, "test.md", "Keep", command); err == nil {
			t.Fatalf("edit accepted %q", command)
		}
	}
	content, _ := os.ReadFile(filepath.Join(dir, "test.md"))
	if string(content) != "## Keep\nold\n" {
		t.Fatalf("file changed: %q", content)
	}
}
