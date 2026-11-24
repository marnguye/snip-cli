package tomlrepo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/marnguye/snip-cli/internal/atomicfile"
	"github.com/marnguye/snip-cli/internal/domain"
	"github.com/pelletier/go-toml/v2"
)

var (
	ErrNotFound = errors.New("snippet not found")
	ErrExists   = errors.New("snippet already exists")
	ErrConflict = errors.New("snippet revision conflict")
)

type Repository struct {
	dataDir string
	root    string
}

type record struct {
	SchemaVersion int       `toml:"schema_version"`
	ID            string    `toml:"id"`
	Title         string    `toml:"title"`
	Description   string    `toml:"description"`
	Command       string    `toml:"command"`
	Group         string    `toml:"group"`
	Tags          []string  `toml:"tags"`
	ExecutionMode string    `toml:"execution_mode"`
	CreatedAt     time.Time `toml:"created_at"`
	UpdatedAt     time.Time `toml:"updated_at"`
	Revision      uint64    `toml:"revision"`
}

func ResolveDataDir() (string, error) {
	if dataHome := os.Getenv("XDG_DATA_HOME"); dataHome != "" {
		if !filepath.IsAbs(dataHome) {
			return "", fmt.Errorf("XDG_DATA_HOME must be absolute")
		}
		return filepath.Join(dataHome, "snip"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".local", "share", "snip"), nil
}

func Open(dataDir string) (*Repository, error) {
	if !filepath.IsAbs(dataDir) {
		return nil, fmt.Errorf("data directory must be absolute")
	}
	return &Repository{dataDir: filepath.Clean(dataDir), root: filepath.Join(filepath.Clean(dataDir), "snippets")}, nil
}

func (repository *Repository) Create(ctx context.Context, snippet domain.Snippet) error {
	if err := snippet.Validate(); err != nil {
		return err
	}
	return repository.withLock(ctx, func() error {
		path, err := repository.pathFor(snippet.ID, false)
		if err != nil {
			return err
		}
		if _, err := os.Lstat(path); err == nil {
			return ErrExists
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect snippet: %w", err)
		}
		return atomicWrite(path, snippet)
	})
}

func (repository *Repository) Get(ctx context.Context, id string) (domain.Snippet, error) {
	if err := ctx.Err(); err != nil {
		return domain.Snippet{}, err
	}
	path, err := repository.pathFor(id, true)
	if err != nil {
		return domain.Snippet{}, err
	}
	return readRecord(path)
}

func (repository *Repository) List(ctx context.Context) ([]domain.Snippet, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := repository.ensureRoot(false)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read snippet directory: %w", err)
	}
	snippets := make([]domain.Snippet, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if entry.Name() == ".lock" {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() || !entry.Type().IsRegular() {
			return nil, fmt.Errorf("unsafe snippet entry %q", entry.Name())
		}
		if filepath.Ext(entry.Name()) != ".toml" {
			return nil, fmt.Errorf("unexpected snippet entry %q", entry.Name())
		}
		id := strings.TrimSuffix(entry.Name(), ".toml")
		if err := domain.ValidateID(id); err != nil {
			return nil, fmt.Errorf("invalid snippet filename %q: %w", entry.Name(), err)
		}
		snippet, err := readRecord(filepath.Join(root, entry.Name()))
		if err != nil {
			return nil, err
		}
		if snippet.ID != id {
			return nil, fmt.Errorf("snippet filename and ID differ for %q", entry.Name())
		}
		snippets = append(snippets, snippet)
	}
	sort.Slice(snippets, func(i, j int) bool {
		if !snippets[i].CreatedAt.Equal(snippets[j].CreatedAt) {
			return snippets[i].CreatedAt.Before(snippets[j].CreatedAt)
		}
		return snippets[i].ID < snippets[j].ID
	})
	return snippets, nil
}

func (repository *Repository) Update(ctx context.Context, snippet domain.Snippet, expectedRevision uint64) (domain.Snippet, error) {
	if err := snippet.Validate(); err != nil {
		return domain.Snippet{}, err
	}
	var updated domain.Snippet
	err := repository.withLock(ctx, func() error {
		path, err := repository.pathFor(snippet.ID, true)
		if err != nil {
			return err
		}
		current, err := readRecord(path)
		if err != nil {
			return err
		}
		if current.Revision != expectedRevision || snippet.CreatedAt != current.CreatedAt {
			return ErrConflict
		}
		snippet.Revision = current.Revision + 1
		snippet.UpdatedAt = time.Now().UTC()
		if err := snippet.Validate(); err != nil {
			return err
		}
		if err := atomicWrite(path, snippet); err != nil {
			return err
		}
		updated = snippet
		return nil
	})
	return updated, err
}

func (repository *Repository) Delete(ctx context.Context, id string, expectedRevision uint64) error {
	return repository.withLock(ctx, func() error {
		path, err := repository.pathFor(id, true)
		if err != nil {
			return err
		}
		snippet, err := readRecord(path)
		if err != nil {
			return err
		}
		if snippet.Revision != expectedRevision {
			return ErrConflict
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("delete snippet: %w", err)
		}
		return nil
	})
}

func (repository *Repository) pathFor(id string, mustExist bool) (string, error) {
	if err := domain.ValidateID(id); err != nil {
		return "", err
	}
	root, err := repository.ensureRoot(false)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && !mustExist {
			root, err = repository.ensureRoot(true)
		}
		if errors.Is(err, os.ErrNotExist) && mustExist {
			return "", ErrNotFound
		}
		if err != nil {
			return "", err
		}
	}
	path := filepath.Join(root, id+".toml")
	if filepath.Dir(path) != root {
		return "", fmt.Errorf("snippet path escapes storage root")
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if mustExist {
			return "", ErrNotFound
		}
		return path, nil
	}
	if err != nil {
		return "", fmt.Errorf("inspect snippet: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", fmt.Errorf("snippet file must be regular")
	}
	return path, nil
}

func (repository *Repository) ensureRoot(create bool) (string, error) {
	if create {
		if err := os.MkdirAll(repository.dataDir, 0o700); err != nil {
			return "", fmt.Errorf("create data directory: %w", err)
		}
	}
	dataInfo, err := os.Lstat(repository.dataDir)
	if err != nil {
		return "", err
	}
	if dataInfo.Mode()&os.ModeSymlink != 0 || !dataInfo.IsDir() {
		return "", fmt.Errorf("data directory must be a real directory")
	}
	if create {
		if err := os.Mkdir(repository.root, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("create snippet directory: %w", err)
		}
	}
	info, err := os.Lstat(repository.root)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", fmt.Errorf("snippet directory must be a real directory")
	}
	resolved, err := filepath.EvalSymlinks(repository.root)
	if err != nil {
		return "", fmt.Errorf("resolve snippet directory: %w", err)
	}
	return filepath.Clean(resolved), nil
}

func (repository *Repository) withLock(ctx context.Context, action func() error) error {
	root, err := repository.ensureRoot(true)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, lockTimeout)
	defer cancel()
	lock, err := acquireLock(ctx, filepath.Join(root, ".lock"))
	if err != nil {
		return err
	}
	defer lock.Close()
	return action()
}

func readRecord(path string) (domain.Snippet, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return domain.Snippet{}, ErrNotFound
		}
		return domain.Snippet{}, fmt.Errorf("inspect snippet: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return domain.Snippet{}, fmt.Errorf("snippet file must be regular")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return domain.Snippet{}, fmt.Errorf("read snippet: %w", err)
	}
	var stored record
	decoder := toml.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&stored); err != nil {
		return domain.Snippet{}, fmt.Errorf("parse snippet TOML: %w", err)
	}
	snippet := domain.Snippet{ID: stored.ID, Title: stored.Title, Description: stored.Description, Command: stored.Command, Group: stored.Group, Tags: stored.Tags, ExecutionMode: domain.ExecutionMode(stored.ExecutionMode), CreatedAt: stored.CreatedAt, UpdatedAt: stored.UpdatedAt, SchemaVersion: stored.SchemaVersion, Revision: stored.Revision}
	if err := snippet.Validate(); err != nil {
		return domain.Snippet{}, err
	}
	return snippet, nil
}

func atomicWrite(path string, snippet domain.Snippet) error {
	stored := record{SchemaVersion: snippet.SchemaVersion, ID: snippet.ID, Title: snippet.Title, Description: snippet.Description, Command: snippet.Command, Group: snippet.Group, Tags: snippet.Tags, ExecutionMode: string(snippet.ExecutionMode), CreatedAt: snippet.CreatedAt, UpdatedAt: snippet.UpdatedAt, Revision: snippet.Revision}
	data, err := toml.Marshal(stored)
	if err != nil {
		return fmt.Errorf("encode snippet TOML: %w", err)
	}
	if err := atomicfile.Write(path, data); err != nil {
		return fmt.Errorf("write snippet: %w", err)
	}
	return nil
}
