package vector

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
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
		// Verify schema compatibility: if the ID field is still the old Int64 type, drop and recreate
		desc, err := m.client.DescribeCollection(ctx, m.cfg.Collection)
		if err != nil {
			return fmt.Errorf("failed to describe collection: %w", err)
		}
		for _, f := range desc.Schema.Fields {
			if f.Name == "id" && f.DataType != entity.FieldTypeVarChar {
				if err := m.client.DropCollection(ctx, m.cfg.Collection); err != nil {
					return fmt.Errorf("collection schema changed (id: Int64→VarChar), failed to drop old: %w", err)
				}
				break
			}
		}
		if has, _ := m.client.HasCollection(ctx, m.cfg.Collection); has {
			return nil // compatible schema
		}
	}

	dimStr := fmt.Sprintf("%d", m.cfg.Dimension)
	schema := &entity.Schema{
		CollectionName: m.cfg.Collection,
		Fields: []*entity.Field{
			entity.NewField().
				WithName("id").
				WithDataType(entity.FieldTypeVarChar).
				WithTypeParams("max_length", "128").
				WithIsPrimaryKey(true).
				WithIsAutoID(false),
			entity.NewField().
				WithName("source_file").
				WithDataType(entity.FieldTypeVarChar).
				WithTypeParams("max_length", "512"),
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
func (m *MilvusStore) Search(ctx context.Context, queryVector []float32, topK int, filter map[string]string) ([]*schema.Document, error) {
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

	docs := make([]*schema.Document, 0)
	for _, result := range results {
		if result.Err != nil {
			continue
		}
		idCol := result.IDs
		contentCol := result.Fields.GetColumn("content")

		for i := 0; i < result.ResultCount; i++ {
			id := ""
			if cv, ok := idCol.(*entity.ColumnVarChar); ok {
				id, _ = cv.ValueByIdx(i)
			}
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
			doc := &schema.Document{
				ID:      id,
				Content: content,
			}
			doc.WithScore(float64(score))
			docs = append(docs, doc)
		}
	}

	return docs, nil
}

// Insert indexes documents into Milvus. Each doc must have a dense vector set.
func (m *MilvusStore) Insert(ctx context.Context, docs []*schema.Document) error {
	if len(docs) == 0 {
		return nil
	}

	ids := make([]string, len(docs))
	contents := make([]string, len(docs))
	sourceFiles := make([]string, len(docs))
	vectors := make([][]float32, len(docs))
	for i, doc := range docs {
		ids[i] = doc.ID
		contents[i] = doc.Content
		if vec := doc.DenseVector(); vec != nil {
			vectors[i] = toFloat32Slice(vec)
		}
		if sf, ok := doc.MetaData["source_file"]; ok {
			if sfs, ok := sf.(string); ok {
				sourceFiles[i] = sfs
			}
		}
	}

	colID := entity.NewColumnVarChar("id", ids)
	colSourceFile := entity.NewColumnVarChar("source_file", sourceFiles)
	colContent := entity.NewColumnVarChar("content", contents)
	colEmbedding := entity.NewColumnFloatVector("embedding", m.cfg.Dimension, vectors)

	_, err := m.client.Insert(ctx, m.cfg.Collection, "", colID, colSourceFile, colContent, colEmbedding)
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
		expr += fmt.Sprintf(`"%s"`, id)
	}
	expr += "]"
	return m.client.Delete(ctx, m.cfg.Collection, "", expr)
}

// Retrieve implements retriever.Retriever for Eino integration.
// It converts the float64 embedding (Eino standard) to float32 (Milvus SDK).
func (m *MilvusStore) Retrieve(ctx context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	options := retriever.GetCommonOptions(&retriever.Options{TopK: intPtr(10)}, opts...)
	topK := 10
	if options.TopK != nil {
		topK = *options.TopK
	}

	var queryVec []float32
	if options.Embedding != nil {
		vecs, err := options.Embedding.EmbedStrings(ctx, []string{query})
		if err != nil {
			return nil, fmt.Errorf("embed query failed: %w", err)
		}
		if len(vecs) > 0 {
			queryVec = toFloat32Slice(vecs[0])
		}
	}
	if len(queryVec) == 0 {
		return nil, fmt.Errorf("vector search requires embedding option")
	}

	docs, err := m.Search(ctx, queryVec, topK, nil)
	if err != nil {
		return nil, err
	}

	for _, d := range docs {
		if d.MetaData == nil {
			d.MetaData = make(map[string]any)
		}
		d.MetaData["source"] = "vector"
	}
	return docs, nil
}

// toFloat32Slice converts float64 slice to float32 for Milvus SDK.
func toFloat32Slice(v []float64) []float32 {
	r := make([]float32, len(v))
	for i, val := range v {
		r[i] = float32(val)
	}
	return r
}

func intPtr(n int) *int { return &n }

// Close releases the Milvus connection.
func (m *MilvusStore) Close() error {
	return m.client.Close()
}
