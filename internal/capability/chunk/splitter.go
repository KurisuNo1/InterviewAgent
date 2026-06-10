package chunk

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
)

// Strategy is the interface for text chunking algorithms.
// Returns schema.Document with Content set to the chunk text.
type Strategy interface {
	Split(text string) []*schema.Document
}

func newChunkID() string {
	return "chunk-" + uuid.New().String()[:8]
}

// SelectStrategy picks the best chunking strategy based on file extension.
func SelectStrategy(fileName string) string {
	ext := strings.ToLower(filepath.Ext(fileName))
	switch ext {
	case ".md", ".markdown":
		return "markdown"
	case ".txt", ".log":
		return "fixed"
	default:
		return "recursive"
	}
}

// NewSplitter creates a splitter with the default (recursive) strategy.
func NewSplitter(chunkSize, overlap int) *RecursiveSplitter {
	return NewRecursiveSplitter(chunkSize, overlap)
}

// NewSplitterForFile creates the appropriate splitter for a given file type.
func NewSplitterForFile(fileName string, chunkSize, overlap int) Strategy {
	switch SelectStrategy(fileName) {
	case "markdown":
		return NewMarkdownSplitter(chunkSize, overlap)
	case "fixed":
		return NewFixedSplitter(chunkSize, overlap)
	default:
		return NewRecursiveSplitter(chunkSize, overlap)
	}
}

// newDoc creates a schema.Document with the given text content.
func newDoc(text string) *schema.Document {
	return &schema.Document{
		ID:      newChunkID(),
		Content: text,
	}
}

// newDocWithMeta creates a schema.Document with metadata.
func newDocWithMeta(text string, meta map[string]any) *schema.Document {
	return &schema.Document{
		ID:       newChunkID(),
		Content:  text,
		MetaData: meta,
	}
}

// ═══════════════════════════════════════════
// FixedSplitter — fixed-length chunking with overlap
// ═══════════════════════════════════════════

type FixedSplitter struct {
	ChunkSize    int
	ChunkOverlap int
}

func NewFixedSplitter(chunkSize, overlap int) *FixedSplitter {
	return &FixedSplitter{ChunkSize: chunkSize, ChunkOverlap: overlap}
}

func (s *FixedSplitter) Split(text string) []*schema.Document {
	if text == "" {
		return nil
	}
	runes := []rune(text)
	total := len(runes)
	if total <= s.ChunkSize {
		return []*schema.Document{newDoc(text)}
	}

	var chunks []*schema.Document
	step := s.ChunkSize - s.ChunkOverlap
	if step <= 0 {
		step = s.ChunkSize
	}

	for start := 0; start < total; start += step {
		end := start + s.ChunkSize
		if end > total {
			end = total
		}
		chunkText := strings.TrimSpace(string(runes[start:end]))
		if chunkText != "" {
			chunks = append(chunks, newDoc(chunkText))
		}
	}
	return chunks
}

// ═══════════════════════════════════════════
// RecursiveSplitter — recursive character splitting
// ═══════════════════════════════════════════

type RecursiveSplitter struct {
	ChunkSize    int
	ChunkOverlap int
	Separators   []string
}

func NewRecursiveSplitter(chunkSize, overlap int) *RecursiveSplitter {
	return &RecursiveSplitter{
		ChunkSize:    chunkSize,
		ChunkOverlap: overlap,
		Separators:   []string{"\n\n", "\n", "。", ". ", "，", ", ", " "},
	}
}

func (s *RecursiveSplitter) Split(text string) []*schema.Document {
	return s.splitRecursive(text, 0)
}

func (s *RecursiveSplitter) splitRecursive(text string, sepIdx int) []*schema.Document {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	if utf8.RuneCountInString(text) <= s.ChunkSize {
		return []*schema.Document{newDoc(text)}
	}

	if sepIdx >= len(s.Separators) {
		return s.hardSplit(text)
	}

	sep := s.Separators[sepIdx]
	parts := strings.Split(text, sep)

	var chunks []*schema.Document
	var current strings.Builder

	for i, part := range parts {
		candidate := ""
		if current.Len() > 0 {
			candidate = current.String() + sep + part
		} else {
			candidate = part
		}

		candidateLen := utf8.RuneCountInString(candidate)

		if candidateLen <= s.ChunkSize {
			if current.Len() > 0 {
				current.WriteString(sep)
			}
			current.WriteString(part)
		} else {
			if current.Len() > 0 {
				chunkText := strings.TrimSpace(current.String())
				if chunkText != "" {
					chunks = append(chunks, newDoc(chunkText))
				}
				current.Reset()
			}

			partLen := utf8.RuneCountInString(part)
			if partLen <= s.ChunkSize {
				current.WriteString(part)
			} else {
				subChunks := s.splitRecursive(part, sepIdx+1)
				chunks = append(chunks, subChunks...)
			}
		}

		if i < len(parts)-1 && current.Len() > 0 {
			currentStr := current.String()
			if utf8.RuneCountInString(currentStr) > s.ChunkSize {
				chunkText := strings.TrimSpace(currentStr)
				if chunkText != "" {
					chunks = append(chunks, newDoc(chunkText))
				}
				current.Reset()
			}
		}
	}

	if current.Len() > 0 {
		chunkText := strings.TrimSpace(current.String())
		if chunkText != "" {
			chunks = append(chunks, newDoc(chunkText))
		}
	}

	return chunks
}

func (s *RecursiveSplitter) hardSplit(text string) []*schema.Document {
	runes := []rune(text)
	total := len(runes)
	if total <= s.ChunkSize {
		return []*schema.Document{newDoc(text)}
	}

	var chunks []*schema.Document
	step := s.ChunkSize - s.ChunkOverlap
	if step <= 0 {
		step = s.ChunkSize
	}

	for start := 0; start < total; start += step {
		end := start + s.ChunkSize
		if end > total {
			end = total
		}
		chunkText := strings.TrimSpace(string(runes[start:end]))
		if chunkText != "" {
			chunks = append(chunks, newDoc(chunkText))
		}
	}
	return chunks
}

// ═══════════════════════════════════════════
// MarkdownSplitter — structural chunking by headers
// ═══════════════════════════════════════════

type MarkdownSplitter struct {
	ChunkSize         int
	ChunkOverlap      int
	recursiveSplitter *RecursiveSplitter
}

func NewMarkdownSplitter(chunkSize, overlap int) *MarkdownSplitter {
	return &MarkdownSplitter{
		ChunkSize:         chunkSize,
		ChunkOverlap:      overlap,
		recursiveSplitter: NewRecursiveSplitter(chunkSize, overlap),
	}
}

func (s *MarkdownSplitter) Split(text string) []*schema.Document {
	lines := strings.Split(text, "\n")
	headers := make(map[int]string)
	var currentLines []string
	var currentLevel int
	var currentStartLine int
	var chunks []*schema.Document

	flushSection := func() {
		if len(currentLines) == 0 {
			return
		}
		content := strings.TrimSpace(strings.Join(currentLines, "\n"))
		if content == "" {
			currentLines = nil
			return
		}

		var headerPath []string
		for lvl := 1; lvl <= currentLevel; lvl++ {
			if title, ok := headers[lvl]; ok {
				headerPath = append(headerPath, title)
			}
		}
		headerPrefix := ""
		meta := make(map[string]any)
		if len(headerPath) > 0 {
			headerPrefix = strings.Join(headerPath, " > ") + "\n\n"
			for lvl := 1; lvl <= 3 && lvl <= currentLevel; lvl++ {
				if title, ok := headers[lvl]; ok {
					key := "h" + string(rune('0'+lvl))
					meta[key] = title
				}
			}
		}

		fullText := headerPrefix + content

		if utf8.RuneCountInString(fullText) > s.ChunkSize {
			subChunks := s.recursiveSplitter.Split(fullText)
			for _, sc := range subChunks {
				if sc.MetaData == nil {
					sc.MetaData = make(map[string]any)
				}
				for k, v := range meta {
					sc.MetaData[k] = v
				}
				if cs, ok := sc.MetaData["chunk_start_line"]; ok {
					sc.MetaData["chunk_start_line"] = strings.TrimSpace(
						strings.Join([]string{
							fmtAny(cs),
							fmtInt(currentStartLine),
						}, ","),
					)
				} else {
					sc.MetaData["chunk_start_line"] = fmtInt(currentStartLine)
				}
				chunks = append(chunks, sc)
			}
		} else {
			meta["chunk_start_line"] = fmtInt(currentStartLine)
			chunks = append(chunks, newDocWithMeta(fullText, meta))
		}
		currentLines = nil
	}

	for lineNum, line := range lines {
		level := headingLevel(line)
		if level > 0 && level <= 3 {
			flushSection()

			title := strings.TrimSpace(line[level:])
			title = strings.TrimLeft(title, " \t")
			headers[level] = title
			for l := level + 1; l <= 3; l++ {
				delete(headers, l)
			}
			currentLevel = level
			currentStartLine = lineNum + 1

			currentLines = append(currentLines, line)
		} else {
			if currentLines == nil {
				currentLines = []string{}
				currentStartLine = lineNum + 1
			}
			currentLines = append(currentLines, line)
		}
	}

	flushSection()

	if len(chunks) == 0 && text != "" {
		return s.recursiveSplitter.Split(text)
	}

	return chunks
}

func headingLevel(line string) int {
	trimmed := strings.TrimLeft(line, " \t")
	if !strings.HasPrefix(trimmed, "#") {
		return 0
	}
	level := 0
	for _, ch := range trimmed {
		if ch == '#' {
			level++
		} else if ch == ' ' || ch == '\t' {
			break
		} else {
			return 0
		}
	}
	if level == 0 || level > 6 {
		return 0
	}
	if len(trimmed) <= level || (trimmed[level] != ' ' && trimmed[level] != '\t') {
		return 0
	}
	return level
}

func fmtInt(n int) string {
	return strconv.Itoa(n)
}

func fmtAny(v any) string {
	return strconv.QuoteToASCII(strings.Trim(fmt.Sprintf("%v", v), `"`))
}
