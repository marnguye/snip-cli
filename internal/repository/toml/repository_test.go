package tomlrepo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/marnguye/snip-cli/internal/domain"
)

func testSnippet(t *testing.T) domain.Snippet {
	t.Helper()
	snippet, err := domain.New("Title", "printf safe", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return snippet
}

func TestCRUDConflictAndPermissions(t *testing.T) {
	repository, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	snippet := testSnippet(t)
	if err := repository.Create(ctx, snippet); err != nil {
		t.Fatal(err)
	}
	if err := repository.Create(ctx, snippet); !errors.Is(err, ErrExists) {
		t.Fatalf("err=%v", err)
	}
	got, err := repository.Get(ctx, snippet.ID)
	if err != nil || got.ID != snippet.ID {
		t.Fatalf("got=%#v err=%v", got, err)
	}
	info, err := os.Stat(filepath.Join(repository.root, snippet.ID+".toml"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%v err=%v", info.Mode(), err)
	}
	got.Title = "Changed"
	updated, err := repository.Update(ctx, got, got.Revision)
	if err != nil || updated.Revision != 2 {
		t.Fatalf("updated=%#v err=%v", updated, err)
	}
	if _, err := repository.Update(ctx, got, 1); !errors.Is(err, ErrConflict) {
		t.Fatalf("err=%v", err)
	}
	if err := repository.Delete(ctx, snippet.ID, 1); !errors.Is(err, ErrConflict) {
		t.Fatalf("err=%v", err)
	}
	if err := repository.Delete(ctx, snippet.ID, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Get(ctx, snippet.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err=%v", err)
	}
}

func TestRejectsUnsafeAndMalformedRecords(t *testing.T) {
	repository, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	snippet := testSnippet(t)
	if err := repository.Create(ctx, snippet); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(repository.root, snippet.ID+".toml")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(repository.root, "missing"), path); err != nil {
		t.Skip(err)
	}
	if _, err := repository.Get(ctx, snippet.ID); err == nil {
		t.Fatal("symlink accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("schema_version = 99"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Get(ctx, snippet.ID); err == nil {
		t.Fatal("malformed record accepted")
	}
	if _, err := repository.Get(ctx, "../bad"); err == nil {
		t.Fatal("path traversal accepted")
	}
}

func TestRejectsSymlinkDataDirectory(t *testing.T) {
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "data")
	if err := os.Symlink(target, link); err != nil {
		t.Skip(err)
	}
	repository, err := Open(link)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Create(context.Background(), testSnippet(t)); err == nil {
		t.Fatal("symlink data directory accepted")
	}
}

func TestConcurrentCreateDoesNotOverwrite(t *testing.T) {
	repository, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	snippet := testSnippet(t)
	ctx := context.Background()
	results := make(chan error, 2)
	go func() { results <- repository.Create(ctx, snippet) }()
	go func() { results <- repository.Create(ctx, snippet) }()
	first, second := <-results, <-results
	if (first == nil) == (second == nil) {
		t.Fatalf("results %v %v", first, second)
	}
}

func newSnippet(t *testing.T, title string) domain.Snippet {
	t.Helper()
	snippet, err := domain.New(title, "printf safe", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return snippet
}

func TestConcurrentCreatesKeepEverySnippet(t *testing.T) {
	dir := t.TempDir()
	const writers = 25
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			repository, err := Open(dir)
			if err == nil {
				err = repository.Create(context.Background(), newSnippet(t, fmt.Sprintf("title %d", i)))
			}
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	repository, _ := Open(dir)
	snippets, err := repository.List(context.Background())
	if err != nil || len(snippets) != writers {
		t.Fatalf("snippets=%d err=%v", len(snippets), err)
	}
}

func TestConcurrentUpdatesAllowOneWinnerPerRevision(t *testing.T) {
	dir := t.TempDir()
	repository, _ := Open(dir)
	snippet := newSnippet(t, "shared")
	if err := repository.Create(context.Background(), snippet); err != nil {
		t.Fatal(err)
	}
	const writers = 10
	var wg sync.WaitGroup
	results := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			next := snippet
			next.Command = fmt.Sprintf("printf %d", i)
			_, err := repository.Update(context.Background(), next, snippet.Revision)
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	won := 0
	for err := range results {
		if err == nil {
			won++
		} else if !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
	}
	if won != 1 {
		t.Fatalf("winners=%d, want 1", won)
	}
}

func TestLockRecoversFromLeftoverLockFiles(t *testing.T) {
	for name, setup := range map[string]func(string) error{
		"file":      func(path string) error { return os.WriteFile(path, []byte("pid"), 0o600) },
		"directory": func(path string) error { return os.Mkdir(path, 0o700) },
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			repository, _ := Open(dir)
			if err := repository.Create(context.Background(), newSnippet(t, "first")); err != nil {
				t.Fatal(err)
			}
			lock := filepath.Join(dir, "snippets", ".lock")
			os.RemoveAll(lock)
			if err := setup(lock); err != nil {
				t.Fatal(err)
			}
			old := time.Now().Add(-time.Hour)
			os.Chtimes(lock, old, old)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := repository.Create(ctx, newSnippet(t, "second")); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLockHeldTimesOutAndThenRecovers(t *testing.T) {
	dir := t.TempDir()
	repository, _ := Open(dir)
	if err := repository.Create(context.Background(), newSnippet(t, "first")); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(dir, "snippets", ".lock")
	held, err := acquireLock(context.Background(), lockPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err = repository.Create(ctx, newSnippet(t, "blocked"))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want deadline exceeded, got %v", err)
	}
	held.Close()
	if err := repository.Create(context.Background(), newSnippet(t, "after")); err != nil {
		t.Fatal(err)
	}
}

func TestLockRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	repository, _ := Open(dir)
	if err := repository.Create(context.Background(), newSnippet(t, "first")); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(dir, "snippets", ".lock")
	os.RemoveAll(lock)
	if err := os.Symlink(filepath.Join(dir, "elsewhere"), lock); err != nil {
		t.Fatal(err)
	}
	if err := repository.Create(context.Background(), newSnippet(t, "second")); err == nil {
		t.Fatal("expected symlink lock rejection")
	}
}
