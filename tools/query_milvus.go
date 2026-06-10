//go:build ignore
// +build ignore

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/KurisuNo1/InterviewAgent/internal/capability/vector"
)

func main() {
	ctx := context.Background()
	store, err := vector.NewMilvusStore(ctx, vector.MilvusConfig{
		Host:       "localhost",
		Port:       19530,
		Username:   "root",
		Password:   "Milvus",
		Database:   "default",
		Collection: "interview_question_bank",
		Dimension:  1024,
		IndexType:  "IVF_FLAT",
		MetricType: "IP",
	})
	if err != nil {
		fmt.Println("connect failed:", err)
		os.Exit(1)
	}
	defer store.Close()

	queryVec := make([]float32, 1024)
	docs, err := store.Search(ctx, queryVec, 100, nil)
	if err != nil {
		fmt.Println("search failed:", err)
		os.Exit(1)
	}

	fmt.Printf("Collection: interview_question_bank\nTotal results: %d\n\n", len(docs))
	for i, doc := range docs {
		fmt.Printf("--- Doc %d ---\n", i+1)
		content := doc.Content
		if len(content) > 300 {
			content = content[:300] + "..."
		}
		fmt.Printf("ID: %s\nContent: %s\n\n", doc.ID, content)
	}
}
