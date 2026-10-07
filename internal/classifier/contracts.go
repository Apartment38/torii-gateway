// Package classifier defines semantic classification independently of model selection.
package classifier

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/Apartment38/torii-gateway/internal/router"
)

type TaskType string

const (
	TaskGeneral       TaskType = "general"
	TaskConversation  TaskType = "conversation"
	TaskCoding        TaskType = "coding"
	TaskSysadmin      TaskType = "sysadmin"
	TaskReasoning     TaskType = "reasoning"
	TaskMath          TaskType = "math"
	TaskResearch      TaskType = "research"
	TaskSummarization TaskType = "summarization"
	TaskExtraction    TaskType = "extraction"
	TaskCreative      TaskType = "creative"
	TaskTranslation   TaskType = "translation"
)

type Complexity string

const (
	ComplexityLow    Complexity = "low"
	ComplexityMedium Complexity = "medium"
	ComplexityHigh   Complexity = "high"
)

type FailurePolicy string

const (
	FailureCapabilityFallback FailurePolicy = "capability_fallback"
	FailureFail               FailurePolicy = "fail"
)

type FeatureRequirements struct {
	Tools            bool `json:"tools"`
	Vision           bool `json:"vision"`
	Reasoning        bool `json:"reasoning"`
	StructuredOutput bool `json:"structured_output"`
}
type Preferences struct {
	LatencySensitive bool `json:"latency_sensitive"`
}
type Classification struct {
	TaskType     TaskType            `json:"task_type"`
	Complexity   Complexity          `json:"complexity"`
	Confidence   float64             `json:"confidence"`
	Requirements FeatureRequirements `json:"requirements"`
	Preferences  Preferences         `json:"preferences"`
	Decision     *DecisionMetadata   `json:"-"`
}

// DecisionMetadata retains protocol evidence for routing observability. It
// never contains the request state or instructions sent to a classifier.
type DecisionMetadata struct {
	Model   string
	Usage   *DecisionUsage
	Choices map[string]ChoiceDecision
}

type DecisionUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

type ChoiceDecision struct {
	Choice        string
	Confidence    float64
	Probabilities map[string]float64
}
type ClassificationRequest struct {
	Model        string
	Text         string
	Requirements router.RequestRequirements
}
type Classifier interface {
	ID() string
	Classify(context.Context, ClassificationRequest) (Classification, error)
}

func (c Classification) Validate() error {
	if !validTask(c.TaskType) {
		return fmt.Errorf("invalid task type %q", c.TaskType)
	}
	if c.Complexity != ComplexityLow && c.Complexity != ComplexityMedium && c.Complexity != ComplexityHigh {
		return fmt.Errorf("invalid complexity %q", c.Complexity)
	}
	if math.IsNaN(c.Confidence) || math.IsInf(c.Confidence, 0) || c.Confidence < 0 || c.Confidence > 1 {
		return errors.New("confidence must be finite and between zero and one")
	}
	return nil
}
func ApplyObjective(c Classification, r router.RequestRequirements) Classification {
	c.Requirements.Tools = c.Requirements.Tools || r.Tools
	c.Requirements.Vision = c.Requirements.Vision || r.Vision
	c.Requirements.Reasoning = c.Requirements.Reasoning || r.Reasoning
	c.Requirements.StructuredOutput = c.Requirements.StructuredOutput || r.StructuredOutput
	return c
}
func Fallback(r router.RequestRequirements) Classification {
	return ApplyObjective(Classification{TaskType: TaskGeneral, Complexity: ComplexityMedium}, r)
}

type Runner struct {
	Classifier        Classifier
	Timeout           time.Duration
	OnFailure         FailurePolicy
	MinimumConfidence float64
}

func (r Runner) Classify(ctx context.Context, request ClassificationRequest) (Classification, error) {
	if r.Classifier == nil {
		return r.failed(request.Requirements, errors.New("classifier is required"))
	}
	if request.Model == "smart" {
		return r.failed(request.Requirements, errors.New("classifier recursion is forbidden"))
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, err := r.Classifier.Classify(callCtx, request)
	if err == nil {
		err = result.Validate()
	}
	if err != nil {
		return r.failed(request.Requirements, err)
	}
	if result.Confidence < r.MinimumConfidence {
		fallback, err := r.failed(request.Requirements, errors.New("classifier confidence is below the configured minimum"))
		fallback.Decision = result.Decision
		return fallback, err
	}
	return ApplyObjective(result, request.Requirements), nil
}
func (r Runner) failed(req router.RequestRequirements, cause error) (Classification, error) {
	if r.OnFailure == FailureCapabilityFallback {
		return Fallback(req), nil
	}
	return Classification{}, fmt.Errorf("classification failed: %w", cause)
}
func validTask(t TaskType) bool {
	switch t {
	case TaskGeneral, TaskConversation, TaskCoding, TaskSysadmin, TaskReasoning, TaskMath, TaskResearch, TaskSummarization, TaskExtraction, TaskCreative, TaskTranslation:
		return true
	}
	return false
}
