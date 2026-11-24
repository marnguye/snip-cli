package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"

	"github.com/marnguye/snip-cli/internal/app"
)

func FindCommand(service *app.Service, query string) error {
	snippets, err := service.Find(context.Background(), query)
	if err != nil {
		return fmt.Errorf("load snippets: %w", err)
	}

	found := false
	for _, s := range snippets {
		fmt.Printf("[%s] %s -> %s\n", s.Group, s.Description, s.Command)
		found = true
	}
	if !found {
		fmt.Println("No snippets found.")
	}
	return nil
}

func RunCommand(service *app.Service, query string) error {
	snippet, err := service.ResolveRun(context.Background(), query)
	if err != nil {
		return err
	}
	finalCommand, err := ResolvePlaceholders(snippet.Command)
	if err != nil {
		return fmt.Errorf("resolve placeholders: %w", err)
	}
	fmt.Printf("Running: %s\n", finalCommand)
	return RunShell(finalCommand)
}

func RunShell(command string) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	defer signal.Stop(interrupts)
	return cmd.Run()
}
