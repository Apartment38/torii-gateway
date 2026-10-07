package api

import (
	"context"
	"github.com/Apartment38/torii-gateway/internal/classifier"
	"log/slog"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Apartment38/torii-gateway/internal/requestid"
	"github.com/Apartment38/torii-gateway/internal/router"
)

type smartSelection struct {
	Affinity      bool                 `json:"affinity"`
	SelectedModel string               `json:"selected_model,omitempty"`
	Reasons       []router.ModelReason `json:"reasons"`
	Truncated     bool                 `json:"truncated,omitempty"`
}
type smartAttempt struct {
	Model     string  `json:"model"`
	Status    int     `json:"status,omitempty"`
	Failed    bool    `json:"failed"`
	ElapsedMS float64 `json:"elapsed_ms"`
}

func (routing *smartRouting) recordSelection(decision router.Decision, affinity bool) {
	selection := smartSelection{Affinity: affinity, SelectedModel: safeEvidenceToken(decision.Model.PublicID)}
	selection.Truncated = len(decision.Reasons) > 64
	for _, reason := range decision.Reasons[:min(64, len(decision.Reasons))] {
		reason.Model = safeEvidenceToken(reason.Model)
		selection.Reasons = append(selection.Reasons, reason)
	}
	routing.selections = append(routing.selections, selection)
	if decision.Model.PublicID != "" {
		routing.estimatedCost = decision.EstimatedCost
	}
}
func (routing *smartRouting) recordAttempt(model string, response *http.Response, err error, elapsed time.Duration) {
	if routing == nil {
		return
	}
	attempt := smartAttempt{Model: safeEvidenceToken(model), Failed: err != nil, ElapsedMS: milliseconds(elapsed)}
	if response != nil {
		attempt.Status = response.StatusCode
		attempt.Failed = attempt.Failed || response.StatusCode < 200 || response.StatusCode >= 300
	}
	routing.attempts = append(routing.attempts, attempt)
}

// Header fields summarize routing only. Raw request/response bytes and prompt,
// state, identifiers, or provider credentials never enter routing evidence.
func (routing *smartRouting) writeHeaders(headers http.Header, model string) {
	if routing == nil {
		return
	}
	// Strip any upstream attempt to supply gateway routing evidence, including
	// optional fields absent from this request, before writing our own values.
	for key := range headers {
		if strings.HasPrefix(strings.ToLower(key), "x-torii-") {
			delete(headers, key)
		}
	}
	if model = safeEvidenceToken(model); model != "" {
		headers.Set("X-Torii-Model", model)
	}
	if task := safeEvidenceToken(string(routing.classification.TaskType)); task != "" {
		headers.Set("X-Torii-Task", task)
	}
	if complexity := safeEvidenceToken(string(routing.classification.Complexity)); complexity != "" {
		headers.Set("X-Torii-Complexity", complexity)
	}
	headers.Set("X-Torii-Routing-Ms", strconv.FormatFloat(milliseconds(routing.elapsed), 'f', 3, 64))
	headers.Set("X-Torii-Classifier-Ms", strconv.FormatFloat(milliseconds(routing.classifierElapsed), 'f', 3, 64))
	headers.Set("X-Torii-Classifier-Source", routing.classifierSource)
	headers.Set("X-Torii-Estimated-Inference-Cost-Usd", costEvidence(routing.estimatedCost))
	headers.Set("X-Torii-Classifier-Cost-Usd", "unknown")
	if decision := routing.classification.Decision; decision != nil {
		if version := safeEvidenceToken(decision.Model); version != "" {
			headers.Set("X-Torii-Classifier-Model", version)
		}
		if routing.classifierSource == "called" && decision.Usage != nil {
			headers.Set("X-Torii-Classifier-Input-Tokens", strconv.FormatInt(decision.Usage.InputTokens, 10))
			headers.Set("X-Torii-Classifier-Output-Tokens", strconv.FormatInt(decision.Usage.OutputTokens, 10))
		}
	}
}

func (routing *smartRouting) logEvidence(ctx context.Context) {
	if routing == nil {
		return
	}
	attrs := []any{"routing_ms", milliseconds(routing.elapsed), "classifier_ms", milliseconds(routing.classifierElapsed), "classifier_source", routing.classifierSource,
		"task_type", routing.classification.TaskType, "complexity", routing.classification.Complexity, "confidence", routing.classification.Confidence,
		"estimated_inference_cost_usd", costEvidence(routing.estimatedCost), "inference_cost_basis", "estimated_request_tokens", "classifier_cost_usd", "unknown",
		"selections", routing.selections, "attempts", routing.attempts}
	if id, ok := requestid.FromContext(ctx); ok {
		attrs = append(attrs, "request_id", safeEvidenceToken(id))
	}
	if routing.errorKind != "" {
		attrs = append(attrs, "error_kind", routing.errorKind)
	}
	if decision := routing.classification.Decision; decision != nil {
		attrs = append(attrs, "classifier_model", safeEvidenceToken(decision.Model))
		if routing.classifierSource == "called" && decision.Usage != nil {
			attrs = append(attrs, "classifier_actual_usage", decision.Usage)
		}
		// Only decoder-validated task/complexity choice records are retained;
		// no unbounded arbitrary provider metadata or prompt text is logged.
		if task, ok := decision.Choices["task_type"]; ok {
			attrs = append(attrs, "classifier_task_choice", boundedChoiceEvidence(task))
		}
		if complexity, ok := decision.Choices["complexity"]; ok {
			attrs = append(attrs, "classifier_complexity_choice", boundedChoiceEvidence(complexity))
		}
	}
	slog.InfoContext(ctx, "smart_routing_decision", attrs...)
}
func safeEvidenceToken(value string) string {
	if len(value) > 256 {
		return ""
	}
	for _, r := range value {
		if r < 32 || r > 126 {
			return ""
		}
	}
	return value
}
func milliseconds(elapsed time.Duration) float64 { return float64(elapsed) / float64(time.Millisecond) }
func costEvidence(cost *float64) string {
	if cost == nil || math.IsNaN(*cost) || math.IsInf(*cost, 0) || *cost < 0 {
		return "unknown"
	}
	return strconv.FormatFloat(*cost, 'g', -1, 64)
}

func boundedChoiceEvidence(choice classifier.ChoiceDecision) classifier.ChoiceDecision {
	result := classifier.ChoiceDecision{Choice: safeEvidenceToken(choice.Choice), Confidence: choice.Confidence, Probabilities: make(map[string]float64)}
	if math.IsNaN(result.Confidence) || math.IsInf(result.Confidence, 0) {
		result.Confidence = 0
	}
	keys := make([]string, 0, len(choice.Probabilities))
	for key := range choice.Probabilities {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys[:min(16, len(keys))] {
		value := choice.Probabilities[key]
		if key != "" && safeEvidenceToken(key) != "" && !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1 {
			result.Probabilities[key] = value
		}
	}
	return result
}
