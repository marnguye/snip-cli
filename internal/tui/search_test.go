package tui

import (
	"fmt"
	"slices"
	"testing"

	"github.com/marnguye/snip-cli/internal/app"
)

func titles(entries []entry, query string) []string {
	results, _ := rank(entries, query, nil, nil)
	out := make([]string, len(results))
	for i, index := range results {
		out[i] = entries[index].item.Title
	}
	return out
}

func entriesOf(items ...app.Item) []entry { return newEntries(items) }

func TestRank(t *testing.T) {
	all := entriesOf(
		app.Item{ID: "1", Title: "Run tests", Command: "go test ./...", Group: "go"},
		app.Item{ID: "2", Title: "Docker ps", Command: "docker ps", Group: "docker"},
		app.Item{ID: "3", Title: "Prune volumes", Command: "docker volume prune", Group: "docker", Tags: []string{"Cleanup"}},
		app.Item{ID: "4", Title: "Žlutý kůň", Command: "echo kůň", Group: "czech"},
		app.Item{ID: "5", Title: "Docker", Command: "docker", Group: "misc"},
	)
	cases := []struct {
		name, query string
		want        []string
	}{
		{"empty returns all in order", "", []string{"Run tests", "Docker ps", "Prune volumes", "Žlutý kůň", "Docker"}},
		{"blank returns all", "   ", []string{"Run tests", "Docker ps", "Prune volumes", "Žlutý kůň", "Docker"}},
		{"exact beats prefix, then group and command", "docker", []string{"Docker", "Docker ps", "Prune volumes"}},
		{"case insensitive", "DOCKER PS", []string{"Docker ps"}},
		{"title prefix", "run", []string{"Run tests", "Prune volumes"}},
		{"fuzzy title", "rntst", []string{"Run tests"}},
		{"command only", "volume", []string{"Prune volumes"}},
		{"command beats nothing", "./...", []string{"Run tests"}},
		{"group match", "czech", []string{"Žlutý kůň"}},
		{"tag match", "cleanup", []string{"Prune volumes"}},
		{"multi word across fields", "prune docker", []string{"Prune volumes"}},
		{"multi word must all match", "docker nothing", nil},
		{"unicode prefix", "žlu", []string{"Žlutý kůň"}},
		{"unicode contains", "KŮŇ", []string{"Žlutý kůň"}},
		{"no match", "zzzz", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := titles(all, tc.query); !slices.Equal(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRankTiers(t *testing.T) {
	all := entriesOf(
		app.Item{ID: "cmd", Title: "zzz", Command: "needle"},
		app.Item{ID: "group", Title: "yyy", Group: "needle"},
		app.Item{ID: "fuzzy", Title: "n-e-e-d-l-e"},
		app.Item{ID: "contains", Title: "a needle b"},
		app.Item{ID: "prefix", Title: "needle b"},
		app.Item{ID: "exact", Title: "needle"},
	)
	results, _ := rank(all, "needle", nil, nil)
	var got []string
	for _, index := range results {
		got = append(got, all[index].item.ID)
	}
	want := []string{"exact", "prefix", "contains", "fuzzy", "group", "cmd"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestRankStableForEqualScoresAndDuplicates(t *testing.T) {
	var items []app.Item
	for i := 0; i < 50; i++ {
		items = append(items, app.Item{ID: fmt.Sprint(i), Title: "same title", Command: fmt.Sprint("cmd ", i)})
	}
	all := newEntries(items)
	for run := 0; run < 3; run++ {
		results, _ := rank(all, "same", nil, nil)
		for i, index := range results {
			if i != index {
				t.Fatalf("run %d: position %d holds %d", run, i, index)
			}
		}
	}
}

func TestRankReusesBuffers(t *testing.T) {
	all := entriesOf(app.Item{Title: "alpha"}, app.Item{Title: "alps"})
	results, buffer := rank(all, "al", nil, nil)
	again, _ := rank(all, "alp", results, buffer)
	if len(again) != 2 || &again[0] != &results[0] {
		t.Fatalf("results not reused: %v", again)
	}
}
