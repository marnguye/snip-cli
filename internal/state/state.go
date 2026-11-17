package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/marnguye/snip-cli/internal/atomicfile"
	"github.com/marnguye/snip-cli/internal/storage"
	"github.com/pelletier/go-toml/v2"
)

type Backend string

const (
	Legacy Backend = "legacy"
	TOML   Backend = "toml"
)

type Resolution struct {
	Backend  Backend
	Explicit bool
}

type file struct {
	SchemaVersion  int     `toml:"schema_version"`
	StorageBackend Backend `toml:"storage_backend"`
}

func ResolveConfigDir() (string, error) {
	if value := os.Getenv("XDG_CONFIG_HOME"); value != "" {
		if !filepath.IsAbs(value) {
			return "", fmt.Errorf("XDG_CONFIG_HOME must be absolute")
		}
		return filepath.Join(value, "snip"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "snip"), nil
}

func Resolve(configDir, legacyDir string) (Resolution, error) {
	path := filepath.Join(configDir, "state.toml")
	data, err := os.ReadFile(path)
	if err == nil {
		var value file
		if err := toml.Unmarshal(data, &value); err != nil {
			return Resolution{}, fmt.Errorf("parse storage state: %w", err)
		}
		if value.SchemaVersion != 1 || (value.StorageBackend != Legacy && value.StorageBackend != TOML) {
			return Resolution{}, fmt.Errorf("invalid storage state")
		}
		return Resolution{Backend: value.StorageBackend, Explicit: true}, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return Resolution{}, fmt.Errorf("read storage state: %w", err)
	}
	files, err := storage.ListSnippets(legacyDir)
	if errors.Is(err, os.ErrNotExist) {
		return Resolution{Backend: TOML}, nil
	}
	if err != nil {
		return Resolution{}, fmt.Errorf("inspect legacy storage: %w", err)
	}
	if len(files) > 0 {
		return Resolution{Backend: Legacy}, nil
	}
	return Resolution{Backend: TOML}, nil
}

func Save(configDir string, backend Backend) error {
	if backend != Legacy && backend != TOML {
		return fmt.Errorf("invalid storage backend")
	}
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return err
	}
	data, err := toml.Marshal(file{SchemaVersion: 1, StorageBackend: backend})
	if err != nil {
		return err
	}
	return atomicfile.Write(filepath.Join(configDir, "state.toml"), data)
}
