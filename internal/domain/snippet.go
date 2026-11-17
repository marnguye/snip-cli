package domain

import (
	"crypto/rand"
	"fmt"
	"strings"
	"time"
)

const SchemaVersion = 1

const (
	maxTitle       = 200
	maxDescription = 4000
	maxCommand     = 65536
	maxGroup       = 100
	maxTags        = 20
	maxTagLength   = 64
)

type ExecutionMode string

const ExecutionModeShell ExecutionMode = "shell"

type Snippet struct {
	ID            string
	Title         string
	Description   string
	Command       string
	Group         string
	Tags          []string
	ExecutionMode ExecutionMode
	CreatedAt     time.Time
	UpdatedAt     time.Time
	SchemaVersion int
	Revision      uint64
}

func New(title, command string, now time.Time) (Snippet, error) {
	id, err := NewID()
	if err != nil {
		return Snippet{}, err
	}
	snippet := Snippet{
		ID:            id,
		Title:         title,
		Command:       command,
		ExecutionMode: ExecutionModeShell,
		CreatedAt:     now.UTC(),
		UpdatedAt:     now.UTC(),
		SchemaVersion: SchemaVersion,
		Revision:      1,
	}
	return snippet, snippet.Validate()
}

func NewID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate snippet ID: %w", err)
	}
	bytes[6] = bytes[6]&0x0f | 0x40
	bytes[8] = bytes[8]&0x3f | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:16]), nil
}

func ValidateID(id string) error {
	if len(id) != 36 {
		return fmt.Errorf("invalid snippet ID")
	}
	for index, char := range id {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if char != '-' {
				return fmt.Errorf("invalid snippet ID")
			}
			continue
		}
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f') {
			return fmt.Errorf("invalid snippet ID")
		}
	}
	return nil
}

func (snippet Snippet) Validate() error {
	if err := ValidateID(snippet.ID); err != nil {
		return err
	}
	if strings.TrimSpace(snippet.Title) == "" || len(snippet.Title) > maxTitle {
		return fmt.Errorf("invalid snippet title")
	}
	if len(snippet.Description) > maxDescription {
		return fmt.Errorf("snippet description is too long")
	}
	if strings.TrimSpace(snippet.Command) == "" || len(snippet.Command) > maxCommand {
		return fmt.Errorf("invalid snippet command")
	}
	if len(snippet.Group) > maxGroup || !validLabel(snippet.Group) {
		return fmt.Errorf("invalid snippet group")
	}
	if len(snippet.Tags) > maxTags {
		return fmt.Errorf("too many snippet tags")
	}
	seen := map[string]bool{}
	for _, tag := range snippet.Tags {
		if len(tag) == 0 || len(tag) > maxTagLength || !validLabel(tag) || seen[tag] {
			return fmt.Errorf("invalid or duplicate snippet tag")
		}
		seen[tag] = true
	}
	if snippet.ExecutionMode != ExecutionModeShell {
		return fmt.Errorf("invalid execution mode")
	}
	if snippet.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported snippet schema version %d", snippet.SchemaVersion)
	}
	if snippet.CreatedAt.IsZero() || snippet.UpdatedAt.IsZero() || snippet.UpdatedAt.Before(snippet.CreatedAt) {
		return fmt.Errorf("invalid snippet timestamps")
	}
	if snippet.Revision == 0 {
		return fmt.Errorf("invalid snippet revision")
	}
	return nil
}

func validLabel(value string) bool {
	if value == "" {
		return true
	}
	for _, char := range value {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-' || char == '_') {
			return false
		}
	}
	return true
}
