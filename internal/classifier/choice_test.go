package classifier

import (
	"context"
	"strings"
	"testing"
)

func TestChooseRejectsUntrustedDecisionResponses(t *testing.T) {
	valid := `{"answers":{"match":{"type":"choice","choice":"candidate_0","confidence":0.95,"probabilities":{"candidate_0":0.95,"none":0.05}}}}`
	criteria := map[string]string{"candidate_0": "gemma-4-e2b", "none": "No match"}
	choice, confidence, err := Choose(context.Background(), decisionFixture(valid), "jev", `{"model":"gemma4:e2b"}`, "Choose exact match", criteria)
	if err != nil || choice != "candidate_0" || confidence != .95 {
		t.Fatalf("%s %f %v", choice, confidence, err)
	}
	for _, body := range []string{
		"not-json", `{"answers":{}}`,
		strings.Replace(valid, `"type":"choice"`, `"type":"noul"`, 1),
		strings.Replace(valid, `"choice":"candidate_0"`, `"choice":"invented"`, 1),
		strings.Replace(valid, `"confidence":0.95`, `"confidence":null`, 1),
		strings.Replace(valid, `"none":0.05`, `"none":0.5`, 1),
		strings.Replace(valid, `"candidate_0":0.95,"none":0.05`, `"candidate_0":0.2,"none":0.8`, 1),
		strings.Repeat("x", maxJevResponseSize+1),
	} {
		if _, _, err := Choose(context.Background(), decisionFixture(body), "jev", "state", "question", criteria); err == nil {
			t.Fatal("accepted malformed decision")
		}
	}
}
