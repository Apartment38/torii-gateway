package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Apartment38/torii-gateway/internal/accounting"
	"github.com/Apartment38/torii-gateway/internal/classifier"
	"github.com/Apartment38/torii-gateway/internal/fault"
	"github.com/Apartment38/torii-gateway/internal/provider"
	"github.com/Apartment38/torii-gateway/internal/router"
	"github.com/Apartment38/torii-gateway/internal/session"
	"github.com/Apartment38/torii-gateway/internal/transport"
)

// smartRouting freezes semantic requirements once per request. Every physical
// attempt is selected under those requirements and the same policy snapshot.
type smartRouting struct {
	identity          session.Identity
	classification    classifier.Classification
	body              []byte
	endpoint          string
	requirements      router.RequestRequirements
	policy            router.SelectionPolicy
	pinnedModel       string
	toolContinuation  bool
	elapsed           time.Duration
	classifierElapsed time.Duration
	classifierSource  string
	estimatedCost     *float64
	selections        []smartSelection
	attempts          []smartAttempt
	errorKind         fault.Kind
}

func (h *OpenAIHandler) prepareSmartRouting(r *http.Request, body []byte) (*smartRouting, error) {
	started := time.Now()
	routing := &smartRouting{body: body, endpoint: r.URL.Path, identity: smartIdentityInfo(r, body, r.URL.Path), policy: h.policy, toolContinuation: smartToolContinuation(body), classifierSource: "not_configured"}
	defer func() { routing.elapsed += time.Since(started) }()
	if err := h.ensureCatalog(r.Context()); err != nil {
		return routing, fault.Wrap(fault.ProviderUnavailable, "discover smart models", err)
	}
	endpoint := router.EndpointChat
	switch r.URL.Path {
	case "/v1/completions":
		endpoint = router.EndpointCompletions
	case "/v1/responses":
		endpoint = router.EndpointResponses
	case "/v1/embeddings":
		endpoint = router.EndpointEmbeddings
	}
	requirements, err := router.ExtractRequirements(endpoint, body)
	if err != nil {
		return routing, fault.Wrap(fault.InvalidRequest, "inspect smart requirements", err)
	}
	pinned, hasPin := h.sessions.GetWithSignature(routing.identity.ID, routing.identity.Signature)
	// Tool outputs contain provider/model-specific opaque call IDs. Once
	// affinity expires or the conversation signature changes, choosing any
	// new model would send those IDs outside their verified original route.
	if routing.toolContinuation && !hasPin {
		return routing, fault.New(fault.ModelUnavailable, "smart")
	}
	if hasPin {
		routing.pinnedModel = pinned.Model
	}
	classification := classifier.Fallback(requirements)
	// The semantic task remains the user's task while the model consumes tool
	// results. Reusing its classification preserves tool-loop continuity and
	// avoids classifying a tool result as a new, easier user request.
	cached, hasCached := pinned.Classification.(classifier.Classification)
	if routing.toolContinuation && hasPin && hasCached {
		classification = classifier.ApplyObjective(cached, requirements)
		routing.classifierSource = "cached_tool_task"
	} else if h.classifier.Classifier != nil && endpoint != router.EndpointEmbeddings {
		routing.classifierSource = "called"
		classifyStarted := time.Now()
		classification, err = h.classifier.Classify(r.Context(), classifier.ClassificationRequest{
			Model: h.classifier.Classifier.ID(), Text: smartText(body), Requirements: requirements,
		})
		routing.classifierElapsed = time.Since(classifyStarted)
		routing.classification = classification
		if err != nil {
			return routing, fault.Wrap(fault.ClassifierFailure, "classify smart request", err)
		}
	}
	classification = classifier.ApplyObjective(classification, requirements)
	requirements.TaskType = string(classification.TaskType)
	requirements.Tools = classification.Requirements.Tools
	requirements.Vision = classification.Requirements.Vision
	requirements.Reasoning = classification.Requirements.Reasoning
	requirements.StructuredOutput = classification.Requirements.StructuredOutput
	requirements.LatencySensitive = classification.Preferences.LatencySensitive
	routing.classification, routing.requirements = classification, requirements
	return routing, nil
}

func (h *OpenAIHandler) selectSmartModel(ctx context.Context, routing *smartRouting, excluded string) (string, error) {
	started := time.Now()
	defer func() { routing.elapsed += time.Since(started) }()
	candidates := make([]router.Candidate, 0)
	var budgetErr error
	for _, model := range h.CatalogModels() {
		if model.PublicID == excluded || !h.enabled(model.PublicID) || len(h.allowedProviders) > 0 && !h.allowedProviders[model.ProviderID] || len(h.allowedModels) > 0 && !h.allowedModels[model.PublicID] {
			continue
		}
		providerID, _, err := provider.ParseModelID(model.PublicID)
		if err != nil || h.byID[providerID] == nil {
			continue
		}
		if subject := accounting.SubjectFrom(ctx); subject != nil && h.accounting != nil && h.budgets.Limited(subject.Key) {
			amount, supported := accounting.Estimate(routing.body, routing.endpoint, model)
			if !supported && (h.budgets.TokenLimited(subject.Key) || h.budgets.CostLimited(subject.Key)) {
				budgetErr = accounting.UnknownCapacity()
				continue
			}
			if err := h.accounting.CanReserve(subject.Key, h.budgets, amount); err != nil {
				budgetErr = err
				continue
			}
		}
		candidate := router.Candidate{Model: model, LatencyEWMA: h.getLatency(model.PublicID), Intelligence: h.modelIntelligence(model.PublicID)}
		if h.quota != nil {
			if status, quotaErr := h.quota.Get(ctx, model.PublicID); quotaErr == nil {
				candidate.QuotaKnown, candidate.QuotaRemaining, candidate.QuotaExhausted = status.Known, status.FractionRemaining, status.Exhausted
			}
		}
		candidates = append(candidates, candidate)
	}
	if len(candidates) == 0 && budgetErr != nil {
		return "", budgetErr
	}
	if excluded == "" && routing.pinnedModel != "" {
		for _, candidate := range candidates {
			if candidate.Model.PublicID == routing.pinnedModel {
				// Affinity is a preference among eligible models, never permission
				// to bypass a current capability, policy, quota, or quality check.
				decision, err := router.Select([]router.Candidate{candidate}, routing.requirements, string(routing.classification.Complexity), routing.policy)
				routing.recordSelection(decision, true)
				if err == nil {
					return routing.pinnedModel, nil
				}
				break
			}
		}
		h.sessions.Delete(routing.identity.ID)
		if routing.toolContinuation {
			return "", fault.New(fault.ModelUnavailable, "smart")
		}
	}
	decision, err := router.Select(candidates, routing.requirements, string(routing.classification.Complexity), routing.policy)
	routing.recordSelection(decision, false)
	if err != nil {
		return "", fault.Wrap(fault.ModelUnavailable, "no eligible smart model", err)
	}
	return decision.Model.PublicID, nil
}

func retryableResponse(response *http.Response, err error) bool {
	if err != nil {
		return !errors.Is(err, transport.ErrInvalidRequest)
	}
	return response == nil || response.Body == nil || response.StatusCode >= 500 || response.StatusCode == http.StatusTooManyRequests
}

func (h *OpenAIHandler) finishSmartRouting(routing *smartRouting, model string, status int, relayErr error) {
	if routing == nil || routing.identity.ID == "" {
		return
	}
	if status < 200 || status >= 300 || relayErr != nil {
		if relayErr != nil {
			routing.errorKind = fault.StreamInterrupted
		}
		h.sessions.Delete(routing.identity.ID)
		return
	}
	_ = h.sessions.Put(session.Record{ID: routing.identity.ID, Model: model, ContextSignature: routing.identity.Signature, Classification: routing.classification})
}

func smartText(body []byte) string {
	var root map[string]any
	if json.Unmarshal(body, &root) != nil {
		return ""
	}
	messages, _ := root["messages"].([]any)
	if messages == nil {
		messages, _ = root["input"].([]any)
	}
	current := contentText(root["input"])
	if messages != nil {
		current = latestUserText(messages)
	} else if current == "" {
		current = contentText(root["prompt"])
	}
	current = smartBoundedText(current, 4000)

	// Keep task instructions distinct from the recent conversation. Only
	// user-visible text is useful here: tool payloads, reasoning, image data,
	// provider IDs and call arguments never become classifier context.
	instructions := contentText(root["instructions"])
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		role, _ := message["role"].(string)
		if role == "system" || role == "developer" {
			if text := smartBoundedText(contentText(message["content"]), 2000); text != "" {
				instructions += "\n" + role + ": " + text
			}
		}
	}
	instructions = smartBoundedText(instructions, 2000)
	context := smartRecentContext(messages)
	if instructions == "" && context == "" {
		return current
	}
	// Preserve simple requests' original state string, while identifying the
	// current task explicitly whenever extra routing context is available.
	state := "Current user request:\n" + current
	if instructions != "" {
		state += "\n\nTask instructions:\n" + instructions
	}
	if context != "" {
		state += "\n\nRecent conversation (oldest first):\n" + context
	}
	return state
}

func smartRecentContext(messages []any) string {
	current := latestUserIndex(messages)
	if current < 0 {
		return ""
	}
	var parts []string
	userTurns := 0
	for i := current - 1; i >= 0; i-- {
		message, _ := messages[i].(map[string]any)
		role, _ := message["role"].(string)
		kind, _ := message["type"].(string)
		if role == "" && (kind == "" || kind == "message" || kind == "input_text" || kind == "text") {
			role = "user"
		}
		if role != "user" && role != "assistant" {
			continue
		}
		if role == "user" {
			userTurns++
			if userTurns > 4 {
				break
			}
		}
		if text := smartBoundedText(contentText(message["content"]), 4000); text != "" {
			parts = append(parts, role+": "+text)
		}
	}
	// Select from the tail first so old conversation cannot crowd out the
	// proposal or task that a short follow-up refers to.
	var kept []string
	remaining := 10000
	for _, part := range parts {
		part = smartBoundedText(part, remaining)
		if part == "" {
			break
		}
		encoded, _ := json.Marshal(part)
		remaining -= len(encoded) + 2
		kept = append(kept, part)
		if remaining < 100 {
			break
		}
	}
	for i, j := 0, len(kept)-1; i < j; i, j = i+1, j-1 {
		kept[i], kept[j] = kept[j], kept[i]
	}
	return strings.Join(kept, "\n")
}

// Budgets count serialized bytes, including JSON escaping, rather than only
// characters. Preserve both ends of a long task: its goal and final constraints.
func smartBoundedText(text string, limit int) string {
	text = strings.TrimSpace(text)
	const marker = "\n[… truncated …]\n"
	if limit <= len(marker)+2 {
		return ""
	}
	budget := limit - 2
	for {
		candidate := text
		if len(candidate) > budget {
			head := (budget - len(marker)) / 2
			tail := len(text) - (budget - len(marker) - head)
			for head > 0 && !utf8.RuneStart(text[head]) {
				head--
			}
			for tail < len(text) && !utf8.RuneStart(text[tail]) {
				tail++
			}
			candidate = text[:head] + marker + text[tail:]
		}
		encoded, _ := json.Marshal(candidate)
		if len(encoded) <= limit {
			return candidate
		}
		budget = budget * (limit - 2) / len(encoded)
		if budget <= len(marker) {
			return ""
		}
	}
}

func latestUserText(messages []any) string {
	i := latestUserIndex(messages)
	if i < 0 {
		return ""
	}
	message := messages[i].(map[string]any)
	if content, ok := message["content"]; ok {
		return contentText(content)
	}
	return contentText(message)
}

func latestUserIndex(messages []any) int {
	for i := len(messages) - 1; i >= 0; i-- {
		message, ok := messages[i].(map[string]any)
		if !ok {
			continue
		}
		role, _ := message["role"].(string)
		kind, _ := message["type"].(string)
		if role == "user" || (role == "" && (kind == "" || kind == "message" || kind == "input_text" || kind == "text")) {
			return i
		}
	}
	return -1
}

func contentText(v any) string {
	switch value := v.(type) {
	case string:
		return value
	case []any:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			if text := contentText(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		kind, _ := value["type"].(string)
		if kind == "text" || kind == "input_text" || kind == "output_text" {
			text, _ := value["text"].(string)
			return text
		}
	}
	return ""
}

func smartToolContinuation(body []byte) bool {
	var root map[string]any
	if json.Unmarshal(body, &root) != nil {
		return false
	}
	for _, key := range []string{"messages", "input"} {
		messages, ok := root[key].([]any)
		if !ok || len(messages) == 0 {
			continue
		}
		last, _ := messages[len(messages)-1].(map[string]any)
		role, _ := last["role"].(string)
		kind, _ := last["type"].(string)
		if role == "tool" || role == "function" || kind == "function_call_output" {
			return true
		}
	}
	return false
}

func smartIdentity(r *http.Request, body []byte, endpoint string) string {
	return smartIdentityInfo(r, body, endpoint).ID
}

func smartIdentityInfo(r *http.Request, body []byte, endpoint string) session.Identity {
	var root map[string]any
	if json.Unmarshal(body, &root) != nil {
		return session.Identity{}
	}
	metadata, _ := root["metadata"].(map[string]any)
	user, _ := root["user"].(string)
	var system, first string
	messages, _ := root["messages"].([]any)
	if messages == nil {
		messages, _ = root["input"].([]any)
	}
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		role, _ := message["role"].(string)
		content := contentText(message["content"])
		if role == "system" || role == "developer" {
			system += "\n" + content
		}
		if (role == "user" || role == "") && first == "" {
			first = content
		}
	}
	if instructions, ok := root["instructions"].(string); ok {
		system += "\n" + instructions
	}
	if first == "" {
		first = contentText(root["input"])
	}
	if first == "" {
		first = contentText(root["prompt"])
	}
	headers := map[string]string{"X-Session-ID": r.Header.Get("X-Session-ID"), "X-Conversation-ID": r.Header.Get("X-Conversation-ID")}
	return session.Identify(headers, metadata, user, endpoint, system, first)
}
