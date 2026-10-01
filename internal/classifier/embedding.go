package classifier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/Apartment38/torii-gateway/internal/provider"
)

type Embedder interface {
	Embed(context.Context, string) ([]float64, error)
}

type ProviderEmbedder struct {
	Provider provider.OpenAIProvider
	Model    string
}

func (e ProviderEmbedder) Embed(ctx context.Context, text string) ([]float64, error) {
	if e.Provider == nil || e.Model == "" {
		return nil, errors.New("embedding provider and model are required")
	}
	body, _ := json.Marshal(map[string]any{"model": e.Model, "input": text})
	response, err := e.Provider.Forward(ctx, "/v1/embeddings", body, e.Model)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("embedding upstream returned %s", response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil || len(envelope.Data) != 1 {
		return nil, errors.New("embedding response has invalid data")
	}
	return validateVector(envelope.Data[0].Embedding)
}

type PrototypeClassifier struct {
	Embedder   Embedder
	Prototypes map[TaskType][]float64
}

func NewPrototypeClassifier(ctx context.Context, embedder Embedder) (*PrototypeClassifier, error) {
	if embedder == nil {
		return nil, errors.New("embedding model is required")
	}
	labels := map[TaskType]string{TaskGeneral: "general helpful request", TaskCoding: "software programming code", TaskSysadmin: "system administration terminal", TaskReasoning: "deep reasoning logic", TaskMath: "mathematics calculation", TaskResearch: "research investigation sources", TaskSummarization: "summarize text", TaskCreative: "creative writing", TaskTranslation: "translate between languages"}
	prototypes := make(map[TaskType][]float64, len(labels))
	for task, label := range labels {
		vector, err := embedder.Embed(ctx, label)
		if err != nil {
			return nil, fmt.Errorf("embed prototype %s: %w", task, err)
		}
		prototypes[task] = vector
	}
	return &PrototypeClassifier{Embedder: embedder, Prototypes: prototypes}, nil
}

func (c PrototypeClassifier) ID() string { return "embedding" }
func (c PrototypeClassifier) Classify(ctx context.Context, request ClassificationRequest) (Classification, error) {
	if c.Embedder == nil || len(c.Prototypes) == 0 {
		return Classification{}, errors.New("embedding classifier is not configured")
	}
	vector, err := c.Embedder.Embed(ctx, request.Text)
	if err != nil {
		return Classification{}, err
	}
	best := TaskGeneral
	score := -2.0
	for task, prototype := range c.Prototypes {
		prototype, err = validateVector(prototype)
		if err != nil {
			return Classification{}, err
		}
		value, err := cosine(vector, prototype)
		if err != nil {
			return Classification{}, err
		}
		if value > score {
			best, score = task, value
		}
	}
	return Classification{TaskType: best, Complexity: ComplexityMedium, Confidence: math.Max(0, math.Min(1, (score+1)/2))}, nil
}

func validateVector(vector []float64) ([]float64, error) {
	if len(vector) == 0 {
		return nil, errors.New("embedding vector is empty")
	}
	norm := 0.0
	for _, value := range vector {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, errors.New("embedding vector contains a non-finite value")
		}
		norm += value * value
	}
	if norm == 0 {
		return nil, errors.New("embedding vector has zero length")
	}
	return append([]float64(nil), vector...), nil
}
func cosine(a, b []float64) (float64, error) {
	if len(a) != len(b) {
		return 0, errors.New("embedding vector dimensions do not match")
	}
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0, errors.New("embedding vector has zero length")
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb)), nil
}
