package storage

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/marnguye/snip-cli/internal/atomicfile"
)

type Snippet struct {
	Description string
	Command     string
}

const maxSnippetLine = 1024 * 1024

func ValidateCategoryFilename(filename string) error {
	if filename == "" || filepath.IsAbs(filename) || filepath.Base(filename) != filename ||
		strings.ContainsAny(filename, `/\\`) || filepath.Ext(filename) != ".md" || filename == ".md" {
		return fmt.Errorf("invalid snippet filename %q: use a single .md filename", filename)
	}
	return nil
}

func validateField(desc, command string) error {
	if strings.TrimSpace(desc) == "" || strings.TrimSpace(command) == "" {
		return fmt.Errorf("description and command are required")
	}
	if strings.ContainsAny(desc, "\r\n") || strings.ContainsAny(command, "\r\n") {
		return fmt.Errorf("markdown snippets cannot contain line breaks")
	}
	if strings.HasPrefix(strings.TrimSpace(command), "#") {
		return fmt.Errorf("markdown snippet commands cannot start with #")
	}
	return nil
}

func storageRoot(dirPath string) (string, error) {
	root, err := filepath.Abs(dirPath)
	if err != nil {
		return "", fmt.Errorf("resolve storage directory: %w", err)
	}
	info, err := os.Lstat(root)
	if err != nil {
		return "", fmt.Errorf("inspect storage directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", fmt.Errorf("storage directory must be a real directory")
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve storage directory: %w", err)
	}
	return filepath.Clean(resolved), nil
}

func snippetPath(dirPath, filename string, mustExist bool) (string, error) {
	if err := ValidateCategoryFilename(filename); err != nil {
		return "", err
	}
	root, err := storageRoot(dirPath)
	if err != nil {
		return "", err
	}
	// shortcut: checks race with hostile filesystem changes; use descriptor-relative APIs if untrusted writers share this directory.
	path := filepath.Join(root, filename)
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("snippet path escapes storage directory")
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) && !mustExist {
		return path, nil
	}
	if err != nil {
		return "", fmt.Errorf("inspect snippet %q: %w", filename, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || info.IsDir() || !info.Mode().IsRegular() {
		return "", fmt.Errorf("snippet %q must be a regular file", filename)
	}
	return path, nil
}

func ListSnippets(dirPath string) ([]string, error) {
	root, err := storageRoot(dirPath)
	if err != nil {
		return nil, err
	}
	files, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read storage directory: %w", err)
	}

	var snippetFiles []string
	for _, file := range files {
		if file.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("snippet entry %q must not be a symlink", file.Name())
		}
		if file.IsDir() {
			if filepath.Ext(file.Name()) == ".md" {
				return nil, fmt.Errorf("snippet entry %q must be a regular file", file.Name())
			}
			continue
		}
		info, err := file.Info()
		if err != nil {
			return nil, fmt.Errorf("inspect snippet entry %q: %w", file.Name(), err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("snippet entry %q must be a regular file", file.Name())
		}
		if filepath.Ext(file.Name()) == ".md" {
			snippetFiles = append(snippetFiles, file.Name())
		}
	}
	return snippetFiles, nil
}

func ReadSnippet(dirPath, filename string) (string, error) {
	path, err := snippetPath(dirPath, filename, true)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read snippet %q: %w", filename, err)
	}
	return string(data), nil
}

func ParseSnippetFile(content string) ([]Snippet, error) {
	var list []Snippet
	var currentDesc string

	scanner := bufio.NewScanner(strings.NewReader(content))
	scanner.Buffer(make([]byte, 64*1024), maxSnippetLine)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if after, found := strings.CutPrefix(line, "##"); found {
			currentDesc = strings.TrimSpace(after)
			continue
		}

		if line != "" && !strings.HasPrefix(line, "#") && currentDesc != "" {
			list = append(list, Snippet{Description: currentDesc, Command: line})
			currentDesc = ""
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parse snippet file: %w", err)
	}
	return list, nil
}

func AppendSnippet(dirPath, filename, desc, command string) error {
	if err := validateField(desc, command); err != nil {
		return err
	}
	path, err := snippetPath(dirPath, filename, false)
	if err != nil {
		return err
	}
	content := ""
	if _, err := os.Lstat(path); err == nil {
		content, err = ReadSnippet(dirPath, filename)
		if err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect snippet %q: %w", filename, err)
	}
	block := fmt.Sprintf("\n## %s\n\n%s\n", strings.TrimSpace(desc), strings.TrimSpace(command))
	return atomicfile.Write(path, []byte(content+block))
}

func RemoveSnippet(dirPath, filename, desc string) error {
	content, err := ReadSnippet(dirPath, filename)
	if err != nil {
		return err
	}
	snippets, err := ParseSnippetFile(content)
	if err != nil {
		return err
	}
	updated := make([]Snippet, 0, len(snippets))
	found := false
	for _, snip := range snippets {
		if snip.Description == desc {
			found = true
			continue
		}
		updated = append(updated, snip)
	}
	if !found {
		return fmt.Errorf("snippet with description %q not found", desc)
	}
	return saveAllSnippets(dirPath, filename, updated)
}

func EditSnippet(dirPath, filename, desc, newCommand string) error {
	if err := validateField(desc, newCommand); err != nil {
		return err
	}
	content, err := ReadSnippet(dirPath, filename)
	if err != nil {
		return err
	}
	snippets, err := ParseSnippetFile(content)
	if err != nil {
		return err
	}
	changed := false
	for i, snip := range snippets {
		if snip.Description == desc {
			snippets[i].Command = newCommand
			changed = true
			break
		}
	}
	if !changed {
		return fmt.Errorf("snippet with description %q not found", desc)
	}
	return saveAllSnippets(dirPath, filename, snippets)
}

func saveAllSnippets(dirPath, filename string, snippets []Snippet) error {
	path, err := snippetPath(dirPath, filename, true)
	if err != nil {
		return err
	}
	var content strings.Builder
	content.WriteString("# " + strings.TrimSuffix(filename, ".md") + " Snippets\n")
	for _, snip := range snippets {
		fmt.Fprintf(&content, "\n## %s\n\n%s\n", snip.Description, snip.Command)
	}
	return atomicfile.Write(path, []byte(content.String()))
}
