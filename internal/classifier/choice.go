package classifier

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/Apartment38/torii-gateway/internal/provider"
)

// Choose asks a configured System One model a single bounded choice question.
// The response must cover exactly the supplied options with valid probabilities.
func Choose(ctx context.Context, transport provider.DecisionProvider, model, state, instructions string, criteria map[string]string) (string, float64, error) {
	if transport == nil || len(criteria) < 2 || len(criteria) > 250 {
		return "", 0, errors.New("invalid decision options")
	}
	body, err := json.Marshal(map[string]any{"model": model, "state": state, "questions": map[string]jevQuestion{
		"match": {Type: "choice", Instructions: instructions, Criteria: criteria},
	}})
	if err != nil {
		return "", 0, err
	}
	resp, err := transport.ForwardDecision(decisionRequestContext(ctx), body)
	if err != nil {
		return "", 0, errors.New("decision request failed (network or timeout)")
	}
	if resp == nil || resp.Body == nil {
		return "", 0, errors.New("decision response is empty")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", 0, errors.New("decision provider rejected the request")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxJevResponseSize+1))
	if err != nil || len(data) > maxJevResponseSize {
		return "", 0, errors.New("decision response is too large or unreadable")
	}
	var envelope struct {
		Answers map[string]jevAnswer `json:"answers"`
	}
	if json.Unmarshal(data, &envelope) != nil {
		return "", 0, errors.New("invalid decision response JSON")
	}
	answer, ok := envelope.Answers["match"]
	if !ok || answer.Type != "choice" {
		return "", 0, errors.New("decision response is missing the match choice")
	}
	if err := validateJevChoice(answer, criteria); err != nil {
		return "", 0, err
	}
	return answer.Choice, *answer.Confidence, nil
}
