package ingestion

import (
	"context"
	"fmt"
	"log"
	"strconv"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/schema"

	"github.com/KurisuNo1/InterviewAgent/internal/capability/chunk"
	"github.com/KurisuNo1/InterviewAgent/internal/capability/keyword"
	"github.com/KurisuNo1/InterviewAgent/internal/capability/resume"
	"github.com/KurisuNo1/InterviewAgent/internal/capability/vector"
)

// Result summarizes a document ingestion operation.
type Result struct {
	FileName string `json:"file_name"`
	Chunks   int    `json:"chunks"`
	Strategy string `json:"strategy"`
}

// DocumentIngestor orchestrates parse -> chunk -> embed -> dual-index.
type DocumentIngestor struct {
	chunkSize    int
	chunkOverlap int
	embedder     embedding.Embedder
	vector       vector.VectorIndexer
	keyword      keyword.KeywordIndex
}

// NewDocumentIngestor creates a new document ingestion service.
func NewDocumentIngestor(chunkSize, chunkOverlap int, embedder embedding.Embedder, vector vector.VectorIndexer, keyword keyword.KeywordIndex) *DocumentIngestor {
	return &DocumentIngestor{
		chunkSize:    chunkSize,
		chunkOverlap: chunkOverlap,
		embedder:     embedder,
		vector:       vector,
		keyword:      keyword,
	}
}

// docEntry is stored to support document listing.
type docEntry struct {
	SourceFile string `json:"source_file"`
	Chunks     int    `json:"chunks"`
}

// Ingest processes a single file: parse, chunk, embed, and dual-index.
func (d *DocumentIngestor) Ingest(ctx context.Context, fileName string, fileData []byte) (*Result, error) {
	// 1. Extract plain text
	text, err := resume.Parse(fileData)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", fileName, err)
	}
	if text == "" {
		return nil, fmt.Errorf("no extractable text in %s", fileName)
	}

	// 2. Create Eino transformer for chunking
	strategyName := chunk.SelectStrategy(fileName)
	strategy := chunk.NewSplitterForFile(fileName, d.chunkSize, d.chunkOverlap)
	transformer := chunk.NewTransformer(strategy)

	inputDoc := &schema.Document{
		Content:  text,
		MetaData: map[string]any{"source_file": fileName},
	}
	chunkDocs, err := transformer.Transform(ctx, []*schema.Document{inputDoc})
	if err != nil {
		return nil, fmt.Errorf("chunk %s: %w", fileName, err)
	}
	if len(chunkDocs) == 0 {
		return nil, fmt.Errorf("no chunks produced for %s", fileName)
	}
	log.Printf("[ingestion] %s: strategy=%s, chunks=%d", fileName, strategyName, len(chunkDocs))

	// 3. Embed all chunks
	chunkTexts := make([]string, len(chunkDocs))
	for i, c := range chunkDocs {
		chunkTexts[i] = c.Content
	}
	vectors, err := d.embedder.EmbedStrings(ctx, chunkTexts)
	if err != nil {
		return nil, fmt.Errorf("embed %s: %w", fileName, err)
	}
	if len(vectors) != len(chunkDocs) {
		return nil, fmt.Errorf("embedding count mismatch for %s", fileName)
	}

	// 4. Attach embeddings and metadata to chunk docs
	for i, c := range chunkDocs {
		c.WithDenseVector(vectors[i])
		if c.MetaData == nil {
			c.MetaData = make(map[string]any)
		}
		c.MetaData["source_file"] = fileName
		c.MetaData["chunk_index"] = strconv.Itoa(i)
		c.MetaData["strategy"] = strategyName
	}

	// 5. Concurrently index into both stores
	errCh := make(chan error, 2)
	if d.vector != nil {
		go func() { errCh <- d.vector.Insert(ctx, chunkDocs) }()
	} else {
		errCh <- nil
	}
	if d.keyword != nil {
		go func() { errCh <- d.keyword.Index(ctx, chunkDocs) }()
	} else {
		errCh <- nil
	}

	for i := 0; i < 2; i++ {
		if e := <-errCh; e != nil {
			return nil, fmt.Errorf("index %s: %w", fileName, e)
		}
	}

	return &Result{
		FileName: fileName,
		Chunks:   len(chunkDocs),
		Strategy: strategyName,
	}, nil
}

// ListDocuments returns all indexed documents in a simple format.
func (d *DocumentIngestor) ListDocuments(ctx context.Context) ([]docEntry, error) {
	if d.keyword == nil {
		return nil, nil
	}
	docs, err := d.keyword.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	var entries []docEntry
	for _, doc := range docs {
		sourceFile := ""
		if sf, ok := doc.MetaData["source_file"]; ok {
			if sfs, ok := sf.(string); ok {
				sourceFile = sfs
			}
		}
		if sourceFile == "" {
			continue
		}
		if !seen[sourceFile] {
			seen[sourceFile] = true
			entries = append(entries, docEntry{SourceFile: sourceFile, Chunks: 1})
		} else {
			for i := range entries {
				if entries[i].SourceFile == sourceFile {
					entries[i].Chunks++
					break
				}
			}
		}
	}
	return entries, nil
}

// DeleteDocument removes all chunks associated with a source file.
func (d *DocumentIngestor) DeleteDocument(ctx context.Context, docID string) error {
	if d.vector != nil {
		if err := d.vector.Delete(ctx, []string{docID}); err != nil {
			return err
		}
	}
	if d.keyword != nil {
		if err := d.keyword.Delete(ctx, docID); err != nil {
			return err
		}
	}
	return nil
}
