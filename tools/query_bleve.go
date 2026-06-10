// +build ignore

package main

import (
	"context"
	"fmt"

	"github.com/KurisuNo1/InterviewAgent/internal/capability/keyword"
)

func main() {
	idx, err := keyword.NewBleveIndex(keyword.BleveConfig{
		IndexPath: "./data/bleve_index",
	})
	if err != nil {
		fmt.Println("open failed:", err)
		return
	}
	defer idx.Close()

	docs, err := idx.Search(context.Background(), "*", 5)
	if err != nil {
		fmt.Println("search failed:", err)
		return
	}
	fmt.Printf("Bleve index doc count (top 5 wildcard): %d\n", len(docs))
	for i, d := range docs {
		content := d.Content
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		fmt.Printf("\n--- Doc %d ---\nID: %s\nContent: %s\n", i+1, d.ID, content)
	}
}
