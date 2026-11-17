package migration

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/marnguye/snip-cli/internal/atomicfile"
	"github.com/marnguye/snip-cli/internal/domain"
	tomlrepo "github.com/marnguye/snip-cli/internal/repository/toml"
	"github.com/marnguye/snip-cli/internal/storage"
	"github.com/pelletier/go-toml/v2"
)

type Options struct {
	LegacyDir string
	DataDir   string
	DryRun    bool
}

type Report struct {
	LegacyFiles     int
	Snippets        int
	Duplicates      int
	Invalid         []string
	ExpectedRecords int
	Migrated        int
	AlreadyMigrated int
	Warnings        []string
	BackupDir       string
}

type planned struct {
	Key     string
	Hash    string
	Snippet domain.Snippet
}

type manifest struct {
	Version   int             `toml:"version"`
	Completed bool            `toml:"completed"`
	BackupDir string          `toml:"backup_dir"`
	Entries   []manifestEntry `toml:"entries"`
}

type manifestEntry struct {
	Key     string `toml:"key"`
	Hash    string `toml:"hash"`
	ID      string `toml:"id"`
	Title   string `toml:"title"`
	Group   string `toml:"group"`
	Command string `toml:"command"`
}

func Run(ctx context.Context, options Options) (Report, error) {
	if !filepath.IsAbs(options.LegacyDir) || !filepath.IsAbs(options.DataDir) {
		return Report{}, fmt.Errorf("migration paths must be absolute")
	}
	plan, report, err := buildPlan(options.LegacyDir)
	if err != nil {
		return report, err
	}
	if options.DryRun {
		return report, nil
	}
	repository, err := tomlrepo.Open(options.DataDir)
	if err != nil {
		return report, err
	}
	manifestPath := filepath.Join(options.DataDir, "migration-legacy-markdown.toml")
	current, err := loadManifest(manifestPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return report, err
	}
	if errors.Is(err, os.ErrNotExist) {
		backup, err := backup(options.LegacyDir, options.DataDir)
		if err != nil {
			return report, err
		}
		report.BackupDir = backup
		current = manifest{Version: 1, BackupDir: backup, Entries: make([]manifestEntry, len(plan))}
		for index, item := range plan {
			current.Entries[index] = manifestEntry{Key: item.Key, Hash: item.Hash, ID: item.Snippet.ID, Title: item.Snippet.Title, Group: item.Snippet.Group, Command: item.Snippet.Command}
		}
		if err := writeManifest(manifestPath, current); err != nil {
			return report, err
		}
	} else {
		if err := applyManifest(plan, current); err != nil {
			return report, err
		}
	}
	for _, item := range plan {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		if existing, err := repository.Get(ctx, item.Snippet.ID); err == nil {
			if existing.Title != item.Snippet.Title || existing.Command != item.Snippet.Command || existing.Group != item.Snippet.Group {
				return report, fmt.Errorf("migration conflict for %s", item.Key)
			}
			report.AlreadyMigrated++
			continue
		} else if !errors.Is(err, tomlrepo.ErrNotFound) {
			return report, err
		}
		if err := repository.Create(ctx, item.Snippet); err != nil {
			return report, fmt.Errorf("migrate %s: %w", item.Key, err)
		}
		report.Migrated++
	}
	current.Completed = true
	if err := writeManifest(manifestPath, current); err != nil {
		return report, err
	}
	return report, nil
}

func Verify(ctx context.Context, legacyDir, dataDir string) error {
	plan, _, err := buildPlan(legacyDir)
	if err != nil {
		return err
	}
	if len(plan) == 0 {
		return nil
	}
	current, err := loadManifest(filepath.Join(dataDir, "migration-legacy-markdown.toml"))
	if err != nil {
		return fmt.Errorf("migration not verified: %w", err)
	}
	if !current.Completed || current.BackupDir == "" {
		return fmt.Errorf("migration is incomplete or backup is missing")
	}
	if info, err := os.Stat(current.BackupDir); err != nil || !info.IsDir() {
		return fmt.Errorf("migration backup is unavailable")
	}
	if err := applyManifest(plan, current); err != nil {
		return err
	}
	repository, err := tomlrepo.Open(dataDir)
	if err != nil {
		return err
	}
	for _, item := range plan {
		stored, err := repository.Get(ctx, item.Snippet.ID)
		if err != nil || stored.Title != item.Snippet.Title || stored.Command != item.Snippet.Command || stored.Group != item.Snippet.Group {
			return fmt.Errorf("migration target record %s is invalid", item.Key)
		}
	}
	return nil
}

func buildPlan(legacyDir string) ([]planned, Report, error) {
	var report Report
	info, err := os.Lstat(legacyDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, report, nil
	}
	if err != nil {
		return nil, report, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, report, fmt.Errorf("legacy directory must be a real directory")
	}
	files, err := storage.ListSnippets(legacyDir)
	if err != nil {
		return nil, report, err
	}
	sort.Strings(files)
	seen := map[string]int{}
	var plan []planned
	for _, file := range files {
		report.LegacyFiles++
		content, err := storage.ReadSnippet(legacyDir, file)
		if err != nil {
			return nil, report, err
		}
		snippets, err := storage.ParseSnippetFile(content)
		if err != nil {
			report.Invalid = append(report.Invalid, file+": "+err.Error())
			continue
		}
		if len(snippets) == 0 {
			report.Warnings = append(report.Warnings, file+": no snippets found")
		}
		for index, legacy := range snippets {
			id, err := domain.NewID()
			if err != nil {
				return nil, report, err
			}
			item := planned{Key: fmt.Sprintf("%s:%d", file, index), Hash: hash(content), Snippet: domain.Snippet{ID: id, Title: legacy.Description, Description: legacy.Description, Command: legacy.Command, Group: strings.TrimSuffix(file, ".md"), ExecutionMode: domain.ExecutionModeShell, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), SchemaVersion: domain.SchemaVersion, Revision: 1}}
			if err := item.Snippet.Validate(); err != nil {
				report.Invalid = append(report.Invalid, item.Key+": "+err.Error())
				continue
			}
			seen[legacy.Description]++
			plan = append(plan, item)
		}
	}
	for _, count := range seen {
		if count > 1 {
			report.Duplicates += count - 1
		}
	}
	report.Snippets = len(plan)
	report.ExpectedRecords = len(plan)
	return plan, report, nil
}

func applyManifest(plan []planned, current manifest) error {
	if current.Version != 1 || len(plan) != len(current.Entries) {
		return fmt.Errorf("legacy migration manifest conflicts with current source")
	}
	entries := map[string]manifestEntry{}
	for _, entry := range current.Entries {
		entries[entry.Key] = entry
	}
	for index := range plan {
		entry, ok := entries[plan[index].Key]
		if !ok || entry.Hash != plan[index].Hash || entry.Title != plan[index].Snippet.Title || entry.Command != plan[index].Snippet.Command || entry.Group != plan[index].Snippet.Group {
			return fmt.Errorf("legacy migration manifest conflicts with %s", plan[index].Key)
		}
		plan[index].Snippet.ID = entry.ID
	}
	return nil
}

func loadManifest(path string) (manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return manifest{}, err
	}
	var current manifest
	if err := toml.Unmarshal(data, &current); err != nil {
		return manifest{}, fmt.Errorf("parse migration manifest: %w", err)
	}
	return current, nil
}

func writeManifest(path string, current manifest) error {
	data, err := toml.Marshal(current)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return atomicfile.Write(path, data)
}

func backup(legacy, data string) (string, error) {
	stamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	destination := filepath.Join(data, "backups", "legacy-markdown-"+stamp)
	if err := os.MkdirAll(destination, 0o700); err != nil {
		return "", err
	}
	files, err := storage.ListSnippets(legacy)
	if err != nil {
		return "", err
	}
	for _, file := range files {
		content, err := storage.ReadSnippet(legacy, file)
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(destination, file), []byte(content), 0o600); err != nil {
			return "", err
		}
		copied, err := os.ReadFile(filepath.Join(destination, file))
		if err != nil || string(copied) != content {
			return "", fmt.Errorf("verify backup %q", file)
		}
	}
	return destination, nil
}

func hash(value string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(value))) }
