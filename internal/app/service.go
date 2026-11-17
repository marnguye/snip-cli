package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/marnguye/snip-cli/internal/domain"
	"github.com/marnguye/snip-cli/internal/migration"
	tomlrepo "github.com/marnguye/snip-cli/internal/repository/toml"
	"github.com/marnguye/snip-cli/internal/state"
	"github.com/marnguye/snip-cli/internal/storage"
)

type Item struct {
	ID, Title, Description, Command, Group string
	Revision                               uint64
	Legacy                                 bool
}

type StorageStatus struct {
	Backend                state.Backend
	Explicit               bool
	LegacyCount, TOMLCount int
	Manifest               string
}

func (service *Service) StorageStatus(ctx context.Context) (StorageStatus, error) {
	resolution, err := state.Resolve(service.ConfigDir, service.LegacyDir)
	if err != nil {
		return StorageStatus{}, err
	}
	legacy := *service
	legacy.Backend = state.Legacy
	legacyItems, err := legacy.List(ctx)
	if err != nil {
		return StorageStatus{}, err
	}
	toml := *service
	toml.Backend = state.TOML
	tomlItems, err := toml.List(ctx)
	if err != nil {
		return StorageStatus{}, err
	}
	manifest := "absent"
	if _, err := os.Stat(filepath.Join(service.DataDir, "migration-legacy-markdown.toml")); err == nil {
		manifest = "present"
	} else if !errors.Is(err, os.ErrNotExist) {
		return StorageStatus{}, err
	}
	return StorageStatus{Backend: resolution.Backend, Explicit: resolution.Explicit, LegacyCount: len(legacyItems), TOMLCount: len(tomlItems), Manifest: manifest}, nil
}

func (service *Service) Use(ctx context.Context, backend state.Backend, confirm bool) error {
	if backend == state.TOML {
		legacy := *service
		legacy.Backend = state.Legacy
		items, err := legacy.List(ctx)
		if err != nil {
			return err
		}
		if len(items) > 0 {
			if err := migration.Verify(ctx, service.LegacyDir, service.DataDir); err != nil {
				return fmt.Errorf("cannot use toml: %w; run snip migrate", err)
			}
		}
	} else if backend == state.Legacy {
		toml := *service
		toml.Backend = state.TOML
		items, err := toml.List(ctx)
		if err != nil {
			return err
		}
		if len(items) > 0 && !confirm {
			return fmt.Errorf("toml data will be hidden; rerun with --confirm")
		}
	} else {
		return fmt.Errorf("invalid storage backend")
	}
	return state.Save(service.ConfigDir, backend)
}

type Service struct {
	Backend    state.Backend
	LegacyDir  string
	DataDir    string
	ConfigDir  string
	Repository *tomlrepo.Repository
}

func New(backend state.Backend, legacyDir, dataDir, configDir string) (*Service, error) {
	if backend != state.Legacy && backend != state.TOML {
		return nil, fmt.Errorf("invalid storage backend")
	}
	repository, err := tomlrepo.Open(dataDir)
	if err != nil {
		return nil, err
	}
	return &Service{Backend: backend, LegacyDir: legacyDir, DataDir: dataDir, ConfigDir: configDir, Repository: repository}, nil
}

func (service *Service) Group(ctx context.Context, group string) ([]Item, error) {
	items, err := service.List(ctx)
	if err != nil {
		return nil, err
	}
	if service.Backend == state.TOML {
		group = strings.TrimSuffix(group, ".md")
	}
	var found []Item
	for _, item := range items {
		if item.Group == group {
			found = append(found, item)
		}
	}
	if len(found) == 0 {
		return nil, fmt.Errorf("snippet file %q not found", group)
	}
	return found, nil
}

func (service *Service) List(ctx context.Context) ([]Item, error) {
	if service.Backend == state.TOML {
		snippets, err := service.Repository.List(ctx)
		if err != nil {
			return nil, err
		}
		items := make([]Item, len(snippets))
		for i, snippet := range snippets {
			items[i] = fromDomain(snippet)
		}
		return items, nil
	}
	return service.legacyList()
}

func (service *Service) Create(ctx context.Context, group, title, command string) (Item, error) {
	if service.Backend == state.Legacy {
		if err := os.MkdirAll(service.LegacyDir, 0o700); err != nil {
			return Item{}, err
		}
		filename := group
		if filepath.Ext(filename) != ".md" {
			filename += ".md"
		}
		if err := storage.AppendSnippet(service.LegacyDir, filename, title, command); err != nil {
			return Item{}, err
		}
		items, err := service.legacyList()
		if err != nil {
			return Item{}, err
		}
		for index := len(items) - 1; index >= 0; index-- {
			if items[index].Group == filename && items[index].Title == title && items[index].Command == command {
				return items[index], nil
			}
		}
		return Item{}, fmt.Errorf("created legacy snippet cannot be located")
	}
	if group == "" {
		group = "default"
	}
	group = strings.TrimSuffix(group, ".md")
	snippet, err := domain.New(title, command, time.Now())
	if err != nil {
		return Item{}, err
	}
	snippet.Description = title
	snippet.Group = group
	if err := snippet.Validate(); err != nil {
		return Item{}, err
	}
	if err := service.Repository.Create(ctx, snippet); err != nil {
		return Item{}, err
	}
	return fromDomain(snippet), nil
}

func (service *Service) Update(ctx context.Context, item Item, command string) (Item, error) {
	if item.Legacy != (service.Backend == state.Legacy) {
		return Item{}, fmt.Errorf("snippet belongs to inactive storage backend")
	}
	if item.Legacy {
		if err := service.uniqueLegacy(item); err != nil {
			return Item{}, err
		}
		if err := storage.EditSnippet(service.LegacyDir, item.Group, item.Title, command); err != nil {
			return Item{}, err
		}
		item.Command = command
		return item, nil
	}
	snippet, err := service.Repository.Get(ctx, item.ID)
	if err != nil {
		return Item{}, err
	}
	snippet.Command = command
	updated, err := service.Repository.Update(ctx, snippet, item.Revision)
	if err != nil {
		return Item{}, err
	}
	return fromDomain(updated), nil
}

func (service *Service) Delete(ctx context.Context, item Item) error {
	if item.Legacy != (service.Backend == state.Legacy) {
		return fmt.Errorf("snippet belongs to inactive storage backend")
	}
	if item.Legacy {
		if err := service.uniqueLegacy(item); err != nil {
			return err
		}
		return storage.RemoveSnippet(service.LegacyDir, item.Group, item.Title)
	}
	return service.Repository.Delete(ctx, item.ID, item.Revision)
}

func (service *Service) Find(ctx context.Context, query string) ([]Item, error) {
	items, err := service.List(ctx)
	if err != nil {
		return nil, err
	}
	query = strings.ToLower(query)
	var found []Item
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.Title), query) || strings.Contains(strings.ToLower(item.Description), query) || strings.Contains(strings.ToLower(item.Command), query) {
			found = append(found, item)
		}
	}
	return found, nil
}

func (service *Service) ResolveRun(ctx context.Context, query string) (Item, error) {
	found, err := service.Find(ctx, query)
	if err != nil {
		return Item{}, err
	}
	var matches []Item
	for _, item := range found {
		if strings.Contains(strings.ToLower(item.Title), strings.ToLower(query)) || strings.Contains(strings.ToLower(item.Description), strings.ToLower(query)) {
			matches = append(matches, item)
		}
	}
	if len(matches) == 0 {
		return Item{}, fmt.Errorf("snippet not found")
	}
	if len(matches) != 1 && service.Backend == state.TOML {
		return Item{}, fmt.Errorf("snippet lookup is ambiguous")
	}
	return matches[0], nil
}

func (service *Service) legacyList() ([]Item, error) {
	files, err := storage.ListSnippets(service.LegacyDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var items []Item
	for _, file := range files {
		content, err := storage.ReadSnippet(service.LegacyDir, file)
		if err != nil {
			return nil, err
		}
		snippets, err := storage.ParseSnippetFile(content)
		if err != nil {
			return nil, err
		}
		for index, snippet := range snippets {
			items = append(items, Item{ID: fmt.Sprintf("legacy:%s:%d", file, index), Title: snippet.Description, Description: snippet.Description, Command: snippet.Command, Group: file, Revision: 1, Legacy: true})
		}
	}
	return items, nil
}

func (service *Service) uniqueLegacy(item Item) error {
	items, err := service.legacyList()
	if err != nil {
		return err
	}
	count := 0
	for _, candidate := range items {
		if candidate.Group == item.Group && candidate.Title == item.Title {
			count++
		}
	}
	if count != 1 {
		return fmt.Errorf("legacy snippet selection is ambiguous")
	}
	return nil
}
func fromDomain(snippet domain.Snippet) Item {
	return Item{ID: snippet.ID, Title: snippet.Title, Description: snippet.Description, Command: snippet.Command, Group: snippet.Group, Revision: snippet.Revision}
}
