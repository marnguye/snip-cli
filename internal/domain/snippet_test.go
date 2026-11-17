package domain

import (
	"strings"
	"testing"
	"time"
)

func TestNewAndValidation(t *testing.T) {
	snippet, err := New("Title", "printf safe", time.Now())
	if err != nil || ValidateID(snippet.ID) != nil || snippet.Revision != 1 {
		t.Fatalf("snippet=%#v err=%v", snippet, err)
	}
	for _, change := range []func(*Snippet){
		func(s *Snippet) { s.Title = " " }, func(s *Snippet) { s.Command = " " }, func(s *Snippet) { s.ID = "bad" },
		func(s *Snippet) { s.ExecutionMode = "bad" }, func(s *Snippet) { s.SchemaVersion = 2 }, func(s *Snippet) { s.UpdatedAt = s.CreatedAt.Add(-time.Second) },
		func(s *Snippet) { s.Tags = []string{"same", "same"} }, func(s *Snippet) { s.Group = "bad/group" }, func(s *Snippet) { s.Title = strings.Repeat("x", maxTitle+1) },
	} {
		copy := snippet
		change(&copy)
		if err := copy.Validate(); err == nil {
			t.Fatal("invalid snippet accepted")
		}
	}
}
