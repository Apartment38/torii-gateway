package classifier

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"testing"

	"github.com/Apartment38/torii-gateway/internal/router"
)

type recordingDecision struct{ body []byte }

func (r *recordingDecision) ForwardDecision(_ context.Context, body []byte) (*http.Response, error) {
	r.body = append([]byte(nil), body...)
	return response(layaDecision), nil
}

func TestDecisionOptionsCustomizeWireWithoutChangingContract(t *testing.T) {
	options := DecisionOptions{
		Questions: map[string]QuestionOverride{
			"task_type": {Instructions: "Prefer coding for code review", Criteria: map[string]string{"coding": "Code review or development"}},
			"tools":     {Criteria: map[string]string{"true": "Needs external actions", "false": "Can answer directly"}},
		},
		NoulThresholds: map[string]float64{"tools": .7, "latency_sensitive": 1, "reasoning": 0},
	}
	transport := &recordingDecision{}
	c, err := NewJevTransportWithOptions(transport, "english", options)
	if err != nil {
		t.Fatal(err)
	}
	// Constructor copies configuration; caller mutations cannot race live requests.
	options.Questions["task_type"].Criteria["coding"] = "mutated after construction"
	options.NoulThresholds["tools"] = 0
	got, err := c.Classify(context.Background(), ClassificationRequest{Text: "Debug the code", Requirements: router.RequestRequirements{Tools: true}})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Requirements.Tools || !got.Requirements.Reasoning || got.Preferences.LatencySensitive {
		t.Fatalf("threshold/objective classification = %+v", got)
	}
	var body struct{ Questions map[string]jevQuestion }
	if err := json.Unmarshal(transport.body, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Questions) != 7 || body.Questions["task_type"].Type != "choice" || len(body.Questions["task_type"].Criteria) != 11 || body.Questions["task_type"].Instructions != "Prefer coding for code review" || body.Questions["task_type"].Criteria["coding"] != "Code review or development" || body.Questions["task_type"].Criteria["general"] == "" || body.Questions["tools"].Criteria["true"] != "Needs external actions" {
		t.Fatalf("questions = %+v", body.Questions)
	}
	got, err = c.Classify(context.Background(), ClassificationRequest{Text: "Debug the code"})
	if err != nil || got.Requirements.Tools {
		t.Fatalf("custom tools threshold = %+v, %v", got, err)
	}
}

func TestDecisionOptionsRejectUnsupportedOrUnboundedRubrics(t *testing.T) {
	tests := []DecisionOptions{
		{Questions: map[string]QuestionOverride{"new_question": {Instructions: "Unknown"}}},
		{Questions: map[string]QuestionOverride{"task_type": {Criteria: map[string]string{"new_task": "Unsupported"}}}},
		{Questions: map[string]QuestionOverride{"tools": {Criteria: map[string]string{"yes": "Wrong key"}}}},
		{Questions: map[string]QuestionOverride{"complexity": {Instructions: " "}}},
		{Questions: map[string]QuestionOverride{"complexity": {Instructions: strings.Repeat("a", 4001)}}},
		{Questions: map[string]QuestionOverride{"task_type": {Criteria: map[string]string{"coding": " "}}}},
		{NoulThresholds: map[string]float64{"task_type": .5}},
		{NoulThresholds: map[string]float64{"unknown": .5}},
		{NoulThresholds: map[string]float64{"tools": -.1}},
		{NoulThresholds: map[string]float64{"vision": 1.1}},
		{NoulThresholds: map[string]float64{"reasoning": math.NaN()}},
		{NoulThresholds: map[string]float64{"structured_output": math.Inf(1)}},
	}
	for _, options := range tests {
		if options.Validate() == nil {
			t.Fatalf("accepted invalid options: %+v", options)
		}
		if _, err := NewJevTransportWithOptions(&recordingDecision{}, "english", options); err == nil {
			t.Fatal("transport constructor accepted invalid options")
		}
	}
	oversized := DecisionOptions{Questions: map[string]QuestionOverride{"task_type": {Criteria: map[string]string{}}}}
	for _, name := range []string{"general", "conversation", "coding", "sysadmin", "reasoning", "math", "research", "summarization", "extraction"} {
		oversized.Questions["task_type"].Criteria[name] = strings.Repeat("a", 4000)
	}
	if oversized.Validate() == nil {
		t.Fatal("accepted >32 KiB of overrides")
	}
}

func TestMinimumConfidenceUsesConfiguredFailurePolicy(t *testing.T) {
	c, err := NewJevTransport(decisionFixture(layaDecision), "english")
	if err != nil {
		t.Fatal(err)
	}
	request := ClassificationRequest{Requirements: router.RequestRequirements{Vision: true, Tools: true, StructuredOutput: true, Reasoning: true}}
	runner := Runner{Classifier: c, MinimumConfidence: .21, OnFailure: FailureFail}
	if _, err := runner.Classify(context.Background(), request); err == nil {
		t.Fatal("low confidence accepted")
	}
	runner.OnFailure = FailureCapabilityFallback
	got, err := runner.Classify(context.Background(), request)
	if err != nil || got.TaskType != TaskGeneral || got.Complexity != ComplexityMedium || !got.Requirements.Vision || !got.Requirements.Tools || !got.Requirements.Reasoning || !got.Requirements.StructuredOutput || got.Decision == nil {
		t.Fatalf("low-confidence fallback = %+v, %v", got, err)
	}
	runner.MinimumConfidence = .2
	got, err = runner.Classify(context.Background(), request)
	if err != nil || got.TaskType != TaskCoding {
		t.Fatalf("threshold equality = %+v, %v", got, err)
	}
}
