package classifier_test

import (
	"context"
	"errors"
	"github.com/Apartment38/torii-router/internal/classifier"
	"github.com/Apartment38/torii-router/internal/router"
	"testing"
	"time"
)

type fakeClassifier struct {
	result classifier.Classification
	err    error
	wait   bool
}

func (f fakeClassifier) ID() string { return "fake" }
func (f fakeClassifier) Classify(ctx context.Context, _ classifier.ClassificationRequest) (classifier.Classification, error) {
	if f.wait {
		<-ctx.Done()
		return classifier.Classification{}, ctx.Err()
	}
	return f.result, f.err
}
func TestRunnerAppliesObjectiveRequirements(t *testing.T) {
	runner := classifier.Runner{Classifier: fakeClassifier{result: classifier.Classification{TaskType: classifier.TaskCoding, Complexity: classifier.ComplexityHigh, Confidence: .9}}, Timeout: time.Second, OnFailure: classifier.FailureFail}
	got, err := runner.Classify(context.Background(), classifier.ClassificationRequest{Model: "physical/model", Requirements: router.RequestRequirements{Tools: true, Vision: true}})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Requirements.Tools || !got.Requirements.Vision {
		t.Fatalf("classification=%#v", got)
	}
}
func TestRunnerFailurePoliciesAndRecursion(t *testing.T) {
	req := classifier.ClassificationRequest{Model: "physical/model", Requirements: router.RequestRequirements{Reasoning: true}}
	fallback := classifier.Runner{Classifier: fakeClassifier{err: errors.New("bad")}, OnFailure: classifier.FailureCapabilityFallback}
	got, err := fallback.Classify(context.Background(), req)
	if err != nil || got.TaskType != classifier.TaskGeneral || got.Complexity != classifier.ComplexityMedium || !got.Requirements.Reasoning {
		t.Fatalf("fallback=%#v,%v", got, err)
	}
	fail := classifier.Runner{Classifier: fakeClassifier{}, OnFailure: classifier.FailureFail}
	req.Model = "smart"
	if _, err := fail.Classify(context.Background(), req); err == nil {
		t.Fatal("smart recursion accepted")
	}
}
func TestRunnerBoundsClassifierCall(t *testing.T) {
	runner := classifier.Runner{Classifier: fakeClassifier{wait: true}, Timeout: time.Millisecond, OnFailure: classifier.FailureFail}
	if _, err := runner.Classify(context.Background(), classifier.ClassificationRequest{Model: "physical/model"}); err == nil {
		t.Fatal("timeout returned no error")
	}
}
func TestClassificationRejectsInvalidSchema(t *testing.T) {
	tests := []classifier.Classification{{TaskType: "new", Complexity: classifier.ComplexityLow}, {TaskType: classifier.TaskGeneral, Complexity: "extreme"}, {TaskType: classifier.TaskGeneral, Complexity: classifier.ComplexityLow, Confidence: 2}}
	for _, value := range tests {
		if value.Validate() == nil {
			t.Fatalf("accepted %#v", value)
		}
	}
}
