package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// EvaluationMetric represents a named evaluation dimension.
type EvaluationMetric string

const (
	MetricFaithfulness EvaluationMetric = "faithfulness"
	MetricRelevance    EvaluationMetric = "relevance"
	MetricCompleteness EvaluationMetric = "completeness"
)

// EvalResult holds the results of a single RAG evaluation run.
type EvalResult struct {
	Query          string             `json:"query"`
	RetrievedDocs  []string           `json:"retrieved_docs"`
	Faithfulness   float64            `json:"faithfulness"`
	Relevance      float64            `json:"relevance"`
	Completeness   float64            `json:"completeness"`
	OverallScore   float64            `json:"overall_score"`
	PerDocScores   []DocEvalScore     `json:"per_doc_scores,omitempty"`
	LLMExplanation string             `json:"llm_explanation,omitempty"`
}

// DocEvalScore holds per-document evaluation scores.
type DocEvalScore struct {
	Content      string  `json:"content"`
	Relevance    float64 `json:"relevance"`
	Faithfulness float64 `json:"faithfulness"`
}

// TopKExperiment holds results for different TopK values.
type TopKExperiment struct {
	Results map[int][]EvalResult `json:"results"`
	BestK   int                   `json:"best_k"`
	Summary string                `json:"summary"`
}

// RAGEvaluator evaluates RAG retrieval quality using LLM-based metrics.
type RAGEvaluator struct {
	chatModel einomodel.ToolCallingChatModel
}

// NewRAGEvaluator creates a new RAG evaluator.
func NewRAGEvaluator(chatModel einomodel.ToolCallingChatModel) *RAGEvaluator {
	return &RAGEvaluator{chatModel: chatModel}
}

// Evaluate performs a full three-dimensional evaluation of RAG results.
func (e *RAGEvaluator) Evaluate(ctx context.Context, query string, retrievedDocs []string, expectedAnswer string) (*EvalResult, error) {
	if len(retrievedDocs) == 0 {
		return &EvalResult{
			Query:         query,
			RetrievedDocs: retrievedDocs,
			OverallScore:  0,
		}, nil
	}

	prompt := buildEvalPrompt(query, retrievedDocs, expectedAnswer)

	resp, err := e.chatModel.Generate(ctx, []*schema.Message{
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return nil, fmt.Errorf("RAG evaluation LLM call failed: %w", err)
	}

	result, err := parseEvalResponse(resp.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse RAG eval response: %w", err)
	}

	result.Query = query
	result.RetrievedDocs = retrievedDocs
	result.LLMExplanation = resp.Content
	result.OverallScore = (result.Faithfulness + result.Relevance + result.Completeness) / 3.0

	return result, nil
}

// RunTopKExperiment evaluates RAG quality across different TopK values.
// searcher is called with each k value to retrieve documents.
func (e *RAGEvaluator) RunTopKExperiment(ctx context.Context, kValues []int,
	searchFunc func(ctx context.Context, k int) ([]string, error),
	queries []string, expectedAnswers []string) (*TopKExperiment, error) {

	exp := &TopKExperiment{
		Results: make(map[int][]EvalResult),
	}

	bestAvgScore := 0.0
	bestK := kValues[0]

	for _, k := range kValues {
		var results []EvalResult

		for i, query := range queries {
			docs, err := searchFunc(ctx, k)
			if err != nil {
				continue
			}

			expected := ""
			if i < len(expectedAnswers) {
				expected = expectedAnswers[i]
			}

			result, err := e.Evaluate(ctx, query, docs, expected)
			if err != nil {
				continue
			}
			results = append(results, *result)
		}

		exp.Results[k] = results

		// Calculate average for this k
		if len(results) > 0 {
			var sum float64
			for _, r := range results {
				sum += r.OverallScore
			}
			avg := sum / float64(len(results))
			if avg > bestAvgScore {
				bestAvgScore = avg
				bestK = k
			}
		}
	}

	exp.BestK = bestK
	exp.Summary = fmt.Sprintf("Best TopK=%d with average score %.2f across %d test queries",
		bestK, bestAvgScore, len(queries))

	return exp, nil
}

func buildEvalPrompt(query string, docs []string, expectedAnswer string) string {
	var docText strings.Builder
	for i, doc := range docs {
		content := doc
		if len(content) > 500 {
			content = content[:500] + "..."
		}
		docText.WriteString(fmt.Sprintf("[Doc %d] %s\n\n", i+1, content))
	}

	prompt := fmt.Sprintf(`You are a RAG (Retrieval-Augmented Generation) quality evaluator. Assess the retrieved documents for the given query.

Query: %s

Retrieved Documents:
%s

%s

Evaluate on three dimensions (score each 0.0-1.0):

1. **Faithfulness** (忠实度): How factually consistent would an answer be if based only on these documents? (1.0 = fully grounded, 0.0 = hallucinated)
2. **Relevance** (相关性): How relevant are the retrieved documents to the query? (1.0 = directly on-topic, 0.0 = completely unrelated)
3. **Completeness** (完整性): To what extent do these documents cover all aspects needed to answer the query? (1.0 = fully covers, 0.0 = missing critical information)

Output ONLY a JSON object:
{
  "faithfulness": <0.0-1.0>,
  "relevance": <0.0-1.0>,
  "completeness": <0.0-1.0>,
  "explanation": "<one sentence per dimension>"
}`, query, docText.String(), optionalExpectedSection(expectedAnswer))

	return prompt
}

func optionalExpectedSection(expected string) string {
	if expected != "" {
		return fmt.Sprintf("Expected Answer (for reference): %s", expected)
	}
	return ""
}

func parseEvalResponse(response string) (*EvalResult, error) {
	// Trim markdown
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	// Find JSON object
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}")
	if start == -1 || end == -1 || end <= start {
		return nil, fmt.Errorf("no JSON object found in response")
	}

	var result EvalResult
	if err := json.Unmarshal([]byte(response[start:end+1]), &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal eval: %w", err)
	}

	return &result, nil
}

// OfflineEvalConfig configures an offline RAG evaluation run.
type OfflineEvalConfig struct {
	TopKValues []int
	TestQueries []TestQuery
	OutputPath string
}

// TestQuery is a single test case for RAG evaluation.
type TestQuery struct {
	Query          string `json:"query"`
	ExpectedAnswer string `json:"expected_answer"`
	Category       string `json:"category"`
}
