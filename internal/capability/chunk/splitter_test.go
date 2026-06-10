package chunk

import (
	"strings"
	"testing"
)

func TestFixedSplitter(t *testing.T) {
	s := NewFixedSplitter(100, 20)
	text := strings.Repeat("hello world ", 50)
	chunks := s.Split(text)
	if len(chunks) == 0 {
		t.Fatal("expected chunks, got none")
	}
	for _, c := range chunks {
		if len([]rune(c.Content)) > 100 {
			t.Errorf("chunk too large: %d runes", len([]rune(c.Content)))
		}
	}
	t.Logf("fixed: %d chunks from %d chars", len(chunks), len(text))
}

func TestRecursiveSplitter(t *testing.T) {
	s := NewRecursiveSplitter(200, 40)
	text := "Paragraph one with some content.\n\nParagraph two with more content.\n\nParagraph three."
	chunks := s.Split(text)
	if len(chunks) == 0 {
		t.Fatal("expected chunks, got none")
	}
	for _, c := range chunks {
		if len([]rune(c.Content)) > 250 { // allow slight overshoot from accumulation
			t.Errorf("chunk too large: %d runes", len([]rune(c.Content)))
		}
	}
	t.Logf("recursive: %d chunks", len(chunks))
}

func TestMarkdownSplitter(t *testing.T) {
	s := NewMarkdownSplitter(500, 50)
	text := `# Introduction
This is the intro section.

## Getting Started
Some content about getting started.

### Prerequisites
You need Go 1.21 or later.

## Advanced Topics
This section covers advanced material with a lot of content.
` + strings.Repeat("More detailed content here. ", 20)

	chunks := s.Split(text)
	if len(chunks) < 3 {
		t.Fatalf("expected at least 3 chunks, got %d", len(chunks))
	}
	// Check that at least one chunk has heading metadata
	hasMeta := false
	for _, c := range chunks {
		if c.MetaData != nil && (c.MetaData["h1"] != "" || c.MetaData["h2"] != "") {
			hasMeta = true
			t.Logf("chunk metadata: h1=%q h2=%q h3=%q", c.MetaData["h1"], c.MetaData["h2"], c.MetaData["h3"])
		}
	}
	if !hasMeta {
		t.Error("expected at least one chunk with heading metadata")
	}
	t.Logf("markdown: %d chunks", len(chunks))
}

func TestMarkdownSplitter_NoHeadings(t *testing.T) {
	s := NewMarkdownSplitter(500, 50)
	text := "This is just plain text with no markdown headings.\nIt should fall back to recursive splitting."
	chunks := s.Split(text)
	if len(chunks) == 0 {
		t.Fatal("expected chunks, got none")
	}
	t.Logf("markdown (no headings): %d chunks", len(chunks))
}

func TestSelectStrategy(t *testing.T) {
	tests := []struct{ file, expected string }{
		{"doc.md", "markdown"},
		{"readme.markdown", "markdown"},
		{"notes.txt", "fixed"},
		{"server.log", "fixed"},
		{"resume.pdf", "recursive"},
		{"report.docx", "recursive"},
		{"unknown.xyz", "recursive"},
		{"noextension", "recursive"},
	}
	for _, tt := range tests {
		got := SelectStrategy(tt.file)
		if got != tt.expected {
			t.Errorf("SelectStrategy(%q) = %q, want %q", tt.file, got, tt.expected)
		}
	}
}

func TestNewSplitterForFile(t *testing.T) {
	tests := []struct{ file string }{
		{"doc.md"},
		{"notes.txt"},
		{"report.pdf"},
	}
	for _, tt := range tests {
		s := NewSplitterForFile(tt.file, 500, 50)
		if s == nil {
			t.Errorf("NewSplitterForFile(%q) returned nil", tt.file)
		}
	}
}
