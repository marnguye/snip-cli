package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/marnguye/snip-cli/internal/app"
	"github.com/marnguye/snip-cli/internal/cli"
	tomlrepo "github.com/marnguye/snip-cli/internal/repository/toml"
	"github.com/marnguye/snip-cli/internal/state"
	"github.com/marnguye/snip-cli/internal/tui"
)

const version = "0.2.0"

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

	model, err := tui.NewModel(service, filename, filterQuery)
	if err != nil {
		return err
	}
	m, err := tea.NewProgram(model, tea.WithAltScreen()).Run()
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
		return copyCommand(finalCommand)
	}
	return nil
}

var writeClipboard = clipboard.WriteAll

func copyCommand(command string) error {
	if err := writeClipboard(command); err != nil {
		fmt.Println(command)
		return fmt.Errorf("clipboard unavailable (%w); command printed above, or run it with ctrl+r", err)
	}
	fmt.Printf("✓ Copied: %s\n", command)
	return nil
}
