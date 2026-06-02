package vector

import (
	"context"
	"fmt"

	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

// MilvusConfig holds configuration for the Milvus connection.
type MilvusConfig struct {
	Host       string
	Port       int
	Username   string
	Password   string
	Database   string
	Collection string
	Dimension  int
	IndexType  string
	MetricType string
}

// MilvusStore implements both VectorRetriever and VectorIndexer using Milvus.
type MilvusStore struct {
	cfg    MilvusConfig
	client client.Client
}

// NewMilvusStore creates a new Milvus vector store.
func NewMilvusStore(ctx context.Context, cfg MilvusConfig) (*MilvusStore, error) {
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	c, err := client.NewClient(ctx, client.Config{
		Address:  addr,
		Username: cfg.Username,
		Password: cfg.Password,
		DBName:   cfg.Database,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Milvus at %s: %w", addr, err)
	}

	store := &MilvusStore{cfg: cfg, client: c}
	if err := store.ensureCollection(ctx); err != nil {
		c.Close()
		return nil, fmt.Errorf("failed to ensure collection exists: %w", err)
	}

	return store, nil
}

// ensureCollection checks if the collection exists and creates it if not.
func (m *MilvusStore) ensureCollection(ctx context.Context) error {
	has, err := m.client.HasCollection(ctx, m.cfg.Collection)
	if err != nil {
		return err
	}
	if has {
		return nil
	}

	dimStr := fmt.Sprintf("%d", m.cfg.Dimension)
	schema := &entity.Schema{
		CollectionName: m.cfg.Collection,
		Fields: []*entity.Field{
			entity.NewField().
				WithName("id").
				WithDataType(entity.FieldTypeInt64).
				WithIsPrimaryKey(true).
				WithIsAutoID(true),
			entity.NewField().
				WithName("content").
				WithDataType(entity.FieldTypeVarChar).
				WithTypeParams("max_length", "65535"),
			entity.NewField().
				WithName("embedding").
				WithDataType(entity.FieldTypeFloatVector).
				WithTypeParams("dim", dimStr),
		},
	}

	if err := m.client.CreateCollection(ctx, schema, 1); err != nil {
		return fmt.Errorf("failed to create collection: %w", err)
	}

	// Create index
	var metricType entity.MetricType
	switch m.cfg.MetricType {
	case "L2":
		metricType = entity.L2
	case "COSINE":
		metricType = entity.COSINE
	default:
		metricType = entity.IP
	}

	idx, err := entity.NewIndexIvfFlat(metricType, 128)
	if err != nil {
		return fmt.Errorf("failed to create index config: %w", err)
	}
	if err := m.client.CreateIndex(ctx, m.cfg.Collection, "embedding", idx, false); err != nil {
		return fmt.Errorf("failed to create index: %w", err)
	}

	return m.client.LoadCollection(ctx, m.cfg.Collection, false)
}

// Search performs ANN search in Milvus.
func (m *MilvusStore) Search(ctx context.Context, queryVector []float32, topK int, filter map[string]string) ([]*Document, error) {
	if err := m.client.LoadCollection(ctx, m.cfg.Collection, false); err != nil {
		return nil, fmt.Errorf("failed to load collection: %w", err)
	}

	sp, _ := entity.NewIndexAUTOINDEXSearchParam(1)
	vec := entity.FloatVector(queryVector)

	var metricType entity.MetricType
	switch m.cfg.MetricType {
	case "L2":
		metricType = entity.L2
	case "COSINE":
		metricType = entity.COSINE
	default:
		metricType = entity.IP
	}

	results, err := m.client.Search(
		ctx, m.cfg.Collection,
		nil,                 // partitions
		"",                  // filter expression
		[]string{"content"}, // output fields
		[]entity.Vector{vec},
		"embedding",
		metricType,
		topK,
		sp,
	)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	if len(results) == 0 {
		return nil, nil
	}

	docs := make([]*Document, 0)
	for _, result := range results {
		if result.Err != nil {
			continue
		}
		// Extract IDs
		idCol := result.IDs
		// Extract content
		contentCol := result.Fields.GetColumn("content")

		for i := 0; i < result.ResultCount; i++ {
			id, _ := idCol.GetAsInt64(i)
			content := ""
			if contentCol != nil {
				if cv, ok := contentCol.(*entity.ColumnVarChar); ok {
					content, _ = cv.ValueByIdx(i)
				}
			}
			score := float32(0)
			if i < len(result.Scores) {
				score = result.Scores[i]
			}
			docs = append(docs, &Document{
				ID:      fmt.Sprintf("%d", id),
				Content: content,
				Metadata: map[string]string{
					"score": fmt.Sprintf("%.4f", score),
				},
			})
		}
	}

	return docs, nil
}

// Insert indexes documents into Milvus.
func (m *MilvusStore) Insert(ctx context.Context, docs []*Document) error {
	if len(docs) == 0 {
		return nil
	}

	contents := make([]string, len(docs))
	vectors := make([][]float32, len(docs))
	for i, doc := range docs {
		contents[i] = doc.Content
		vectors[i] = doc.Vector
	}

	colContent := entity.NewColumnVarChar("content", contents)
	colEmbedding := entity.NewColumnFloatVector("embedding", m.cfg.Dimension, vectors)

	_, err := m.client.Insert(ctx, m.cfg.Collection, "", colContent, colEmbedding)
	if err != nil {
		return fmt.Errorf("insert failed: %w", err)
	}

	return m.client.Flush(ctx, m.cfg.Collection, false)
}

// Delete removes documents from Milvus by ID using an expression.
func (m *MilvusStore) Delete(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	expr := "id in ["
	for i, id := range ids {
		if i > 0 {
			expr += ", "
		}
		expr += id
	}
	expr += "]"
	return m.client.Delete(ctx, m.cfg.Collection, "", expr)
}

// Close releases the Milvus connection.
func (m *MilvusStore) Close() error {
	return m.client.Close()
}
