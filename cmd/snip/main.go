package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/marnguye/snip-cli/internal/app"
	"github.com/marnguye/snip-cli/internal/cli"
	tomlrepo "github.com/marnguye/snip-cli/internal/repository/toml"
	"github.com/marnguye/snip-cli/internal/state"
	"github.com/marnguye/snip-cli/internal/tui"
)

const version = "0.1.0"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(exitCode(err))
	}
}

func exitCode(err error) int {
	var exitErr interface{ ExitCode() int }
	if errors.As(err, &exitErr) && exitErr.ExitCode() > 0 {
		return exitErr.ExitCode()
	}
	return 1
}

func run(args []string) error {
	if len(args) == 1 && args[0] == "--version" {
		fmt.Println("snip", version)
		return nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}
	legacyDir := filepath.Join(homeDir, ".config", "snip", "snippets")
	configDir, err := state.ResolveConfigDir()
	if err != nil {
		return err
	}
	dataDir, err := tomlrepo.ResolveDataDir()
	if err != nil {
		return err
	}
	resolution, err := state.Resolve(configDir, legacyDir)
	if err != nil {
		return err
	}
	service, err := app.New(resolution.Backend, legacyDir, dataDir, configDir)
	if err != nil {
		return err
	}

	filterQuery, filename, shouldRunUI, err := cli.Execute(args, service)
	if err != nil {
		if errors.Is(err, cli.ErrCancelled) {
			fmt.Println("\n✕ Cancelled")
			return nil
		}
		return err
	}
	if !shouldRunUI {
		return nil
	}

	var stored []app.Item
	if filename != "" {
		stored, err = service.Group(context.Background(), filename)
	} else {
		stored, err = service.List(context.Background())
	}
	if err != nil {
		return err
	}
	var items []list.Item
	for _, snippet := range stored {
		items = append(items, tui.NewItem(snippet))
	}

	p := tea.NewProgram(tui.NewModel(items, service, filterQuery), tea.WithAltScreen())
	m, err := p.Run()
	if err != nil {
		return err
	}

	if finalModel, ok := m.(tui.Model); ok && finalModel.Selected != "" {
		finalCommand, err := cli.ResolvePlaceholders(finalModel.Selected)
		if err != nil {
			if errors.Is(err, cli.ErrCancelled) {
				fmt.Println("\n✕ Cancelled")
				return nil
			}
			return err
		}

		if finalModel.Execute {
			return cli.RunShell(finalCommand)
		}
		if err := clipboard.WriteAll(finalCommand); err != nil {
			return fmt.Errorf("copy command: %w", err)
		}
		fmt.Printf("✓ Copied: %s\n", finalCommand)
	}
	return nil
}
