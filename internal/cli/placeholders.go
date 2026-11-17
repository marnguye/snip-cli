package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
)

var placeholderRegex = regexp.MustCompile(`<([^>]+)>`)

var ErrCancelled = fmt.Errorf("cancelled")

func ResolvePlaceholders(command string) (string, error) {
	return resolvePlaceholders(command, os.Stdin, os.Stdout)
}

func resolvePlaceholders(command string, input io.Reader, output io.Writer) (string, error) {
	matches := placeholderRegex.FindAllStringSubmatch(command, -1)
	if len(matches) == 0 {
		return command, nil
	}

	scanner := bufio.NewScanner(input)
	cancelled := false
	result := placeholderRegex.ReplaceAllStringFunc(command, func(match string) string {
		if cancelled {
			return match
		}
		fmt.Fprintf(output, "Enter value for [%s]: ", match[1:len(match)-1])
		if !scanner.Scan() {
			cancelled = true
			return match
		}
		return scanner.Text()
	})
	if cancelled {
		return "", ErrCancelled
	}
	return result, nil
}
