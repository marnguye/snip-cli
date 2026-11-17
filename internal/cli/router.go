package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/marnguye/snip-cli/internal/app"
	"github.com/marnguye/snip-cli/internal/migration"
	"github.com/marnguye/snip-cli/internal/state"
)

func Execute(args []string, service *app.Service) (filterQuery string, filename string, shouldRunUI bool, err error) {
	if len(args) < 1 {
		return "", "", true, nil
	}

	switch args[0] {
	case "storage":
		if len(args) == 2 && args[1] == "status" {
			status, err := service.StorageStatus(context.Background())
			if err != nil {
				return "", "", false, err
			}
			source := "automatic"
			if status.Explicit {
				source = "explicit"
			}
			fmt.Printf("active backend: %s\nselection: %s\nlegacy directory: %s\ntoml directory: %s\nlegacy records: %d\ntoml records: %d\nmigration manifest: %s\n", status.Backend, source, service.LegacyDir, service.DataDir, status.LegacyCount, status.TOMLCount, status.Manifest)
			return "", "", false, nil
		}
		if len(args) >= 3 && args[1] == "use" {
			backend := state.Backend(args[2])
			confirm := len(args) == 4 && args[3] == "--confirm"
			if len(args) > 4 || (len(args) == 4 && !confirm) {
				return "", "", false, fmt.Errorf("usage: snip storage use <toml|legacy> [--confirm]")
			}
			if err := service.Use(context.Background(), backend, confirm); err != nil {
				return "", "", false, err
			}
			fmt.Printf("storage backend set to %s\n", backend)
			return "", "", false, nil
		}
		return "", "", false, fmt.Errorf("usage: snip storage status | snip storage use <toml|legacy> [--confirm]")
	case "--list", "_list-descriptions":
		items, err := service.List(context.Background())
		if err != nil {
			return "", "", false, err
		}
		for _, item := range items {
			fmt.Fprintln(os.Stdout, item.Description)
		}
		return "", "", false, nil

	case "completion":
		fmt.Println("find run add remove edit migrate storage")
		return "", "", false, nil

	case "run":
		if len(args) < 2 {
			return "", "", false, fmt.Errorf("usage: snip run <description>")
		}
		return "", "", false, RunCommand(service, args[1])

	case "find":
		if len(args) < 2 {
			return "", "", false, fmt.Errorf("usage: snip find <query>")
		}
		return args[1], "", true, nil

	case "add":
		if len(args) < 4 {
			return "", "", false, fmt.Errorf("usage: snip add <file.md> <description> <command>")
		}
		if _, err := service.Create(context.Background(), args[1], args[2], args[3]); err != nil {
			return "", "", false, err
		}
		fmt.Printf("✓ Snippet added\n")
		return "", "", false, nil

	case "remove":
		if len(args) < 3 {
			return "", "", false, fmt.Errorf("usage: snip remove <file.md> <description>")
		}
		item, err := resolveLegacyOrUnique(service, args[1], args[2])
		if err != nil {
			return "", "", false, err
		}
		if err := service.Delete(context.Background(), item); err != nil {
			return "", "", false, err
		}
		fmt.Printf("✓ Snippet removed\n")
		return "", "", false, nil

	case "edit":
		if len(args) < 4 {
			return "", "", false, fmt.Errorf("usage: snip edit <file.md> <description> <new_command>")
		}
		item, err := resolveLegacyOrUnique(service, args[1], args[2])
		if err != nil {
			return "", "", false, err
		}
		if _, err := service.Update(context.Background(), item, args[3]); err != nil {
			return "", "", false, err
		}
		fmt.Printf("✓ Snippet updated\n")
		return "", "", false, nil

	case "reload":
		return "", "", false, fmt.Errorf("reload is no longer supported; run go install ./cmd/snip instead")

	case "migrate":
		if len(args) > 2 || len(args) == 2 && args[1] != "--dry-run" {
			return "", "", false, fmt.Errorf("usage: snip migrate [--dry-run]")
		}
		report, err := migration.Run(context.Background(), migration.Options{LegacyDir: service.LegacyDir, DataDir: service.DataDir, DryRun: len(args) == 2})
		fmt.Printf("legacy files: %d\nsnippets: %d\nduplicate descriptions: %d\ninvalid records: %d\nexpected records: %d\nmigrated: %d\nalready migrated: %d\n", report.LegacyFiles, report.Snippets, report.Duplicates, len(report.Invalid), report.ExpectedRecords, report.Migrated, report.AlreadyMigrated)
		for _, warning := range append(report.Warnings, report.Invalid...) {
			fmt.Println("warning:", warning)
		}
		if report.BackupDir != "" {
			fmt.Println("backup:", report.BackupDir)
		}
		return "", "", false, err

	default:
		return "", args[0], true, nil
	}
}

func resolveLegacyOrUnique(service *app.Service, group, title string) (app.Item, error) {
	items, err := service.List(context.Background())
	if err != nil {
		return app.Item{}, err
	}
	var found []app.Item
	for _, item := range items {
		if item.Group == group && item.Title == title {
			found = append(found, item)
		}
	}
	if len(found) != 1 {
		return app.Item{}, fmt.Errorf("snippet lookup is ambiguous or missing")
	}
	return found[0], nil
}
