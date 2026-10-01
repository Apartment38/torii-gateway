package classifier

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Apartment38/torii-router/internal/provider"
)

const maxJevResponseSize = 1 << 20

// Jev invokes the typed-decision API and maps its answers to Torii's shared
// classifier contract. Jev is deliberately not exposed as an inference
// provider because its API returns decisions, not chat completions.
type Jev struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

// JevTransport is a provider-backed Jev classifier. It is used for gateways
// such as OpenRouter that expose Jev's System One contract at their own
// origin and credential boundary.
type JevTransport struct {
	provider provider.DecisionProvider
	model    string
}

func NewJev(baseURL, apiKey, model string, client *http.Client) (*Jev, error) {
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("Jev base URL and API key are required")
	}
	if model == "" {
		model = "jev-latest"
	}
	if client == nil {
		client = &http.Client{}
	}
	return &Jev{baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey, model: model, client: client}, nil
}

func (j *Jev) ID() string { return "jev" }

func (j *Jev) Classify(ctx context.Context, request ClassificationRequest) (Classification, error) {
	body, err := jevRequestBody(j.model, request)
	if err != nil {
		return Classification{}, err
	}
	resp, err := j.client.Do(mustRequest(ctx, j.baseURL+"/v1/systemone/", body, j.apiKey))
	if err != nil {
		return Classification{}, fmt.Errorf("Jev request: %w", err)
	}
	return decodeJevResponse(resp, request)
}

func NewJevTransport(p provider.DecisionProvider, model string) (*JevTransport, error) {
	if p == nil {
		return nil, errors.New("Jev provider transport is required")
	}
	if model == "" {
		model = "typesafe/jev-1.13"
	}
	return &JevTransport{provider: p, model: model}, nil
}

func (j *JevTransport) ID() string { return "jev/provider" }

func (j *JevTransport) Classify(ctx context.Context, request ClassificationRequest) (Classification, error) {
	body, err := jevRequestBody(j.model, request)
	if err != nil {
		return Classification{}, err
	}
	resp, err := j.provider.ForwardDecision(ctx, body)
	if err != nil {
		return Classification{}, fmt.Errorf("Jev provider request: %w", err)
	}
	return decodeJevResponse(resp, request)
}

func jevRequestBody(model string, request ClassificationRequest) ([]byte, error) {
	state := request.Text
	if state == "" {
		state = request.Model
	}
	questions := map[string]any{
		"task_type": map[string]any{"type": "choice", "instructions": "Which task type best describes this request?", "criteria": map[string]string{
			"general": "General knowledge or other task", "conversation": "Open-ended conversation", "coding": "Writing or debugging code", "sysadmin": "Operating systems, infrastructure, or deployment", "reasoning": "Multi-step logical reasoning", "math": "Mathematics or quantitative calculation", "research": "Research or source synthesis", "summarization": "Summarizing provided content", "extraction": "Extracting structured facts", "creative": "Creative writing or ideation", "translation": "Translating between languages",
		}},
		"complexity":        map[string]any{"type": "choice", "instructions": "How complex is this request?", "criteria": map[string]string{"low": "Simple and direct", "medium": "Requires several steps or moderate judgment", "high": "Requires deep reasoning, extensive context, or difficult execution"}},
		"tools":             noulQuestion("Does this request require tool or function calling?"),
		"vision":            noulQuestion("Does this request require understanding image content?"),
		"reasoning":         noulQuestion("Does this request require a reasoning-capable model?"),
		"structured_output": noulQuestion("Does this request require structured or schema-constrained output?"),
		"latency_sensitive": noulQuestion("Is low latency especially important for this request?"),
	}
	return json.Marshal(map[string]any{"model": model, "state": state, "questions": questions})
}

func mustRequest(ctx context.Context, endpoint string, body []byte, apiKey string) *http.Request {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func decodeJevResponse(resp *http.Response, request ClassificationRequest) (Classification, error) {
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxJevResponseSize+1))
	if err != nil || len(data) > maxJevResponseSize {
		return Classification{}, errors.New("Jev response is too large")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Classification{}, fmt.Errorf("Jev returned %s", resp.Status)
	}
	var envelope struct {
		Answers map[string]struct {
			Type       string  `json:"type"`
			Choice     string  `json:"choice"`
			Noul       float64 `json:"noul"`
			Confidence float64 `json:"confidence"`
		} `json:"answers"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return Classification{}, fmt.Errorf("decode Jev response: %w", err)
	}
	answer := func(name string) (struct {
		Type       string  `json:"type"`
		Choice     string  `json:"choice"`
		Noul       float64 `json:"noul"`
		Confidence float64 `json:"confidence"`
	}, error) {
		value, ok := envelope.Answers[name]
		if !ok {
			return value, fmt.Errorf("Jev response is missing %q", name)
		}
		return value, nil
	}
	task, err := answer("task_type")
	if err != nil {
		return Classification{}, err
	}
	complexity, err := answer("complexity")
	if err != nil {
		return Classification{}, err
	}
	if task.Choice == "" || complexity.Choice == "" {
		return Classification{}, errors.New("Jev response contains an empty choice")
	}
	result := Classification{TaskType: TaskType(task.Choice), Complexity: Complexity(complexity.Choice), Confidence: task.Confidence}
	if complexity.Confidence < result.Confidence || result.Confidence == 0 {
		result.Confidence = complexity.Confidence
	}
	result.Requirements = FeatureRequirements{Tools: noul(envelope.Answers, "tools"), Vision: noul(envelope.Answers, "vision"), Reasoning: noul(envelope.Answers, "reasoning"), StructuredOutput: noul(envelope.Answers, "structured_output")}
	result.Requirements = FeatureRequirements{Tools: result.Requirements.Tools || request.Requirements.Tools, Vision: result.Requirements.Vision || request.Requirements.Vision, Reasoning: result.Requirements.Reasoning || request.Requirements.Reasoning, StructuredOutput: result.Requirements.StructuredOutput || request.Requirements.StructuredOutput}
	result.Preferences.LatencySensitive = noul(envelope.Answers, "latency_sensitive")
	if err := result.Validate(); err != nil {
		return Classification{}, err
	}
	return result, nil
}

func noulQuestion(instructions string) map[string]any {
	return map[string]any{"type": "noul", "instructions": instructions}
}

func noul(answers map[string]struct {
	Type       string  `json:"type"`
	Choice     string  `json:"choice"`
	Noul       float64 `json:"noul"`
	Confidence float64 `json:"confidence"`
}, name string) bool {
	answer := answers[name]
	return answer.Noul >= 0.5
}

var _ Classifier = (*Jev)(nil)
