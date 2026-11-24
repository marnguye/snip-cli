package tui

import (
	"slices"
	"strings"

	"github.com/marnguye/snip-cli/internal/app"
)

const minFuzzyLength = 3

type entry struct {
	item        app.Item
	title       string
	titleRunes  []rune
	description string
	command     string
	group       string
	tags        []string
}

type scored struct {
	index, score int
}

func newEntry(item app.Item) entry {
	title := strings.ToLower(item.Title)
	e := entry{
		item:       item,
		title:      title,
		titleRunes: []rune(title),
		command:    strings.ToLower(item.Command),
		group:      strings.ToLower(item.Group),
	}
	if description := strings.ToLower(item.Description); description != title {
		e.description = description
	}
	for _, tag := range item.Tags {
		e.tags = append(e.tags, strings.ToLower(tag))
	}
	return e
}

func rank(entries []entry, query string, results []int, buffer []scored) ([]int, []scored) {
	results = results[:0]
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		for i := range entries {
			results = append(results, i)
		}
		return results, buffer
	}
	phrase := strings.Join(terms, " ")
	runes := make([][]rune, len(terms))
	for i, term := range terms {
		runes[i] = []rune(term)
	}
	buffer = buffer[:0]
	for i := range entries {
		if score := entries[i].score(terms, runes, phrase); score >= 0 {
			buffer = append(buffer, scored{i, score})
		}
	}
	slices.SortStableFunc(buffer, func(a, b scored) int { return b.score - a.score })
	for _, s := range buffer {
		results = append(results, s.index)
	}
	return results, buffer
}

func (e *entry) score(terms []string, runes [][]rune, phrase string) int {
	total := 0
	for i, term := range terms {
		score := e.termScore(term, runes[i])
		if score < 0 {
			return -1
		}
		total += score
	}
	if len(terms) > 1 {
		switch {
		case e.title == phrase:
			total += 500
		case strings.HasPrefix(e.title, phrase):
			total += 300
		}
	}
	return total
}

func (e *entry) termScore(term string, termRunes []rune) int {
	switch {
	case e.title == term:
		return 1000
	case strings.HasPrefix(e.title, term):
		return 800
	case strings.Contains(e.title, term):
		return 600
	}
	if len(termRunes) >= minFuzzyLength {
		if gap := subsequenceGap(e.titleRunes, termRunes); gap >= 0 {
			return max(400-gap, 350)
		}
	}
	if strings.Contains(e.group, term) || strings.Contains(e.description, term) || slices.ContainsFunc(e.tags, func(tag string) bool { return strings.Contains(tag, term) }) {
		return 300
	}
	if strings.Contains(e.command, term) {
		return 200
	}
	return -1
}

func subsequenceGap(text, term []rune) int {
	if len(term) == 0 {
		return 0
	}
	matched, first := 0, -1
	for i, r := range text {
		if r != term[matched] {
			continue
		}
		if first < 0 {
			first = i
		}
		matched++
		if matched == len(term) {
			return i - first + 1 - len(term)
		}
	}
	return -1
}
