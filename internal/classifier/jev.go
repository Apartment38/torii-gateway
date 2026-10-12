package classifier

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"

	"github.com/Apartment38/torii-gateway/internal/provider"
)

const maxJevResponseSize = 1 << 20

// Jev invokes the typed-decision API and maps its answers to Torii's shared
// classifier contract. Jev is deliberately not exposed as an inference
// provider because its API returns decisions, not chat completions.
type Jev struct {
	endpoint string
	apiKey   string
	model    string
	client   *http.Client
	options  DecisionOptions
}

// JevTransport is a provider-backed Jev classifier. It is used for gateways
// such as OpenRouter that expose Jev's System One contract at their own
// origin and credential boundary.
type JevTransport struct {
	provider provider.DecisionProvider
	model    string
	options  DecisionOptions
}

func NewJev(baseURL, apiKey, model string, client *http.Client) (*Jev, error) {
	return NewJevWithOptions(baseURL, apiKey, model, client, DecisionOptions{})
}

func NewJevWithOptions(baseURL, apiKey, model string, client *http.Client, options DecisionOptions) (*Jev, error) {
	if err := options.Validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(apiKey) == "" || strings.ContainsAny(apiKey, "\r\n") {
		return nil, errors.New("Jev API key is required and must not contain newlines")
	}
	u, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("Jev base URL must be an HTTP(S) URL without credentials, query, or fragment")
	}
	// Accept a service root or an API root, retaining any deployment prefix.
	u.Path = strings.TrimSuffix(strings.TrimRight(u.Path, "/"), "/v1") + "/v1/systemone"
	u.RawPath = ""
	if model == "" {
		model = "jev-latest"
	}
	if client == nil {
		client = &http.Client{}
	}
	cloned := *client
	cloned.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Jev{endpoint: u.String(), apiKey: apiKey, model: model, client: &cloned, options: options.clone()}, nil
}

func (j *Jev) ChoosesModel() bool { return true }

func (j *Jev) ID() string { return "jev" }

func (j *Jev) Classify(ctx context.Context, request ClassificationRequest) (Classification, error) {
	body, err := jevRequestBody(j.model, request, j.options)
	if err != nil {
		return Classification{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, j.endpoint, bytes.NewReader(body))
	if err != nil {
		return Classification{}, errors.New("could not create Jev request")
	}
	req.Header.Set("Authorization", "Bearer "+j.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := j.client.Do(req)
	if err != nil {
		return Classification{}, errors.New("Jev request failed (network or timeout)")
	}
	return decodeJevResponse(resp, request, j.options)
}

func NewJevTransport(p provider.DecisionProvider, model string) (*JevTransport, error) {
	return NewJevTransportWithOptions(p, model, DecisionOptions{})
}

func NewJevTransportWithOptions(p provider.DecisionProvider, model string, options DecisionOptions) (*JevTransport, error) {
	if err := options.Validate(); err != nil {
		return nil, err
	}
	if p == nil {
		return nil, errors.New("Jev provider transport is required")
	}
	if model == "" {
		model = "typesafe/jev-1.13"
	}
	return &JevTransport{provider: p, model: model, options: options.clone()}, nil
}

func (j *JevTransport) ChoosesModel() bool { return true }

func (j *JevTransport) ID() string { return "jev/provider" }

func (j *JevTransport) Classify(ctx context.Context, request ClassificationRequest) (Classification, error) {
	body, err := jevRequestBody(j.model, request, j.options)
	if err != nil {
		return Classification{}, err
	}
	resp, err := j.provider.ForwardDecision(ctx, body)
	if err != nil {
		return Classification{}, errors.New("Jev provider request failed (network or timeout)")
	}
	return decodeJevResponse(resp, request, j.options)
}

func jevRequestBody(model string, request ClassificationRequest, options DecisionOptions) ([]byte, error) {
	state := request.Text
	if state == "" {
		state = request.Model
	}
	questions := options.questions()
	if len(request.Candidates) > 0 {
		question, err := modelQuestion(request)
		if err != nil {
			return nil, err
		}
		questions["model"] = question
	}
	return json.Marshal(map[string]any{"model": model, "state": state, "questions": questions})
}

func jevQuestions() map[string]jevQuestion {
	return map[string]jevQuestion{
		"task_type": {Type: "choice", Instructions: "Which task type best describes this request?", Criteria: map[string]string{
			"general": "General knowledge or other task", "conversation": "Open-ended conversation", "coding": "Writing or debugging code", "sysadmin": "Operating systems, infrastructure, or deployment", "reasoning": "Multi-step logical reasoning", "math": "Mathematics or quantitative calculation", "research": "Research or source synthesis", "summarization": "Summarizing provided content", "extraction": "Extracting structured facts", "creative": "Creative writing or ideation", "translation": "Translating between languages",
		}},
		"complexity":        {Type: "choice", Instructions: "How complex is this request?", Criteria: map[string]string{"low": "Simple and direct", "medium": "Requires several steps or moderate judgment", "high": "Requires deep reasoning, extensive context, or difficult execution"}},
		"tools":             noulQuestion("Does this request require tool or function calling?"),
		"vision":            noulQuestion("Does this request require understanding image content?"),
		"reasoning":         noulQuestion("Does this request require a reasoning-capable model?"),
		"structured_output": noulQuestion("Does this request require structured or schema-constrained output?"),
		"latency_sensitive": noulQuestion("Is low latency especially important for this request?"),
	}
}

type jevQuestion struct {
	Type         string            `json:"type"`
	Instructions string            `json:"instructions"`
	Criteria     map[string]string `json:"criteria,omitempty"`
}

type jevAnswer struct {
	Type          string              `json:"type"`
	Choice        string              `json:"choice"`
	Noul          *float64            `json:"noul"`
	Confidence    *float64            `json:"confidence"`
	Probabilities map[string]*float64 `json:"probabilities"`
}

func decodeJevResponse(resp *http.Response, request ClassificationRequest, options ...DecisionOptions) (Classification, error) {
	if resp == nil || resp.Body == nil {
		return Classification{}, errors.New("Jev response is empty")
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxJevResponseSize+1))
	if err != nil || len(data) > maxJevResponseSize {
		return Classification{}, errors.New("Jev response is too large")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Classification{}, fmt.Errorf("Jev returned HTTP %d", resp.StatusCode)
	}
	var envelope struct {
		Model   string               `json:"model"`
		Usage   json.RawMessage      `json:"usage"`
		Answers map[string]jevAnswer `json:"answers"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		// Decoder errors can include upstream-controlled strings. Keep request
		// contents and upstream diagnostics out of gateway errors.
		return Classification{}, errors.New("invalid Jev response JSON")
	}
	questions := jevQuestions()
	if len(request.Candidates) > 0 {
		question, err := modelQuestion(request)
		if err != nil {
			return Classification{}, err
		}
		questions["model"] = question
	}
	for name, question := range questions {
		answer, ok := envelope.Answers[name]
		if !ok || answer.Type != question.Type {
			return Classification{}, fmt.Errorf("Jev response is missing or has the wrong type for %q", name)
		}
		if question.Type == "noul" {
			if !unitProbability(answer.Noul) {
				return Classification{}, fmt.Errorf("Jev response has an invalid noul for %q", name)
			}
		} else if err := validateJevChoice(answer, question.Criteria); err != nil {
			return Classification{}, fmt.Errorf("Jev response has an invalid choice for %q: %w", name, err)
		}
	}
	task, complexity := envelope.Answers["task_type"], envelope.Answers["complexity"]
	result := Classification{TaskType: TaskType(task.Choice), Complexity: Complexity(complexity.Choice), Confidence: math.Min(*task.Confidence, *complexity.Confidence)}
	if len(envelope.Model) > 256 || strings.ContainsAny(envelope.Model, "\r\n\t") {
		return Classification{}, errors.New("Jev response has invalid model metadata")
	}
	result.Decision = &DecisionMetadata{Model: envelope.Model, Usage: decodeJevUsage(envelope.Usage), Choices: make(map[string]ChoiceDecision, 2)}
	names := []string{"task_type", "complexity"}
	if len(request.Candidates) > 0 {
		result.SelectedModel = envelope.Answers["model"].Choice
		result.Confidence = math.Min(result.Confidence, *envelope.Answers["model"].Confidence)
		names = append(names, "model")
	}
	for _, name := range names {
		answer := envelope.Answers[name]
		choice := ChoiceDecision{Choice: answer.Choice, Confidence: *answer.Confidence, Probabilities: make(map[string]float64, len(answer.Probabilities))}
		for option, probability := range answer.Probabilities {
			choice.Probabilities[option] = *probability
		}
		result.Decision.Choices[name] = choice
	}
	thresholds := map[string]float64(nil)
	if len(options) > 0 {
		thresholds = options[0].NoulThresholds
	}
	result.Requirements = FeatureRequirements{Tools: noul(envelope.Answers, "tools", thresholds), Vision: noul(envelope.Answers, "vision", thresholds), Reasoning: noul(envelope.Answers, "reasoning", thresholds), StructuredOutput: noul(envelope.Answers, "structured_output", thresholds)}
	result = ApplyObjective(result, request.Requirements)
	result.Preferences.LatencySensitive = noul(envelope.Answers, "latency_sensitive", thresholds)
	if err := result.Validate(); err != nil {
		return Classification{}, err
	}
	return result, nil
}

// Usage is optional telemetry, not a required decision answer. Publish it only
// when both counts are measured nonnegative integers; missing, null or malformed
// telemetry remains unknown without discarding an otherwise valid decision.
func decodeJevUsage(raw json.RawMessage) *DecisionUsage {
	var usage struct {
		InputTokens  *int64 `json:"input_tokens"`
		OutputTokens *int64 `json:"output_tokens"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &usage) != nil || usage.InputTokens == nil || usage.OutputTokens == nil || *usage.InputTokens < 0 || *usage.OutputTokens < 0 {
		return nil
	}
	return &DecisionUsage{InputTokens: *usage.InputTokens, OutputTokens: *usage.OutputTokens}
}

func unitProbability(value *float64) bool {
	return value != nil && !math.IsNaN(*value) && !math.IsInf(*value, 0) && *value >= 0 && *value <= 1
}

func validateJevChoice(answer jevAnswer, criteria map[string]string) error {
	if _, ok := criteria[answer.Choice]; !ok {
		return errors.New("choice is not a requested option")
	}
	if !unitProbability(answer.Confidence) {
		return errors.New("confidence must be a number between zero and one")
	}
	if len(answer.Probabilities) != len(criteria) {
		return errors.New("probabilities must cover the requested options")
	}
	var total float64
	for option := range criteria {
		probability := answer.Probabilities[option]
		if !unitProbability(probability) {
			return errors.New("probabilities must be numbers between zero and one")
		}
		total += *probability
	}
	// Laya rounds each probability to four decimal places. Permit the
	// maximum aggregate rounding error while rejecting incomplete distributions.
	if math.Abs(total-1) > float64(len(criteria))*0.00005+1e-9 {
		return errors.New("probabilities must sum to one")
	}
	chosen := *answer.Probabilities[answer.Choice]
	for _, probability := range answer.Probabilities {
		if *probability > chosen {
			return errors.New("choice must have the highest probability")
		}
	}
	return nil
}

func noulQuestion(instructions string) jevQuestion {
	return jevQuestion{Type: "noul", Instructions: instructions}
}

func noul(answers map[string]jevAnswer, name string, thresholds map[string]float64) bool {
	threshold := 0.5
	if configured, ok := thresholds[name]; ok {
		threshold = configured
	}
	return *answers[name].Noul >= threshold
}

var _ Classifier = (*Jev)(nil)
