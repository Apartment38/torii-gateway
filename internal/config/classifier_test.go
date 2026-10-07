package config

import (
	"math"
	"testing"

	"github.com/Apartment38/torii-gateway/internal/classifier"
	"gopkg.in/yaml.v3"
)

func TestClassifierTuningConfigurationRoundTrip(t *testing.T) {
	router := RouterConfig{Classifier: ClassifierConfig{Type: "jev", Transport: "provider", Provider: "laya", Model: "english", Timeout: "750ms", MinimumConfidence: .4, Questions: map[string]classifier.QuestionOverride{"task_type": {Instructions: "Classify code reviews as coding", Criteria: map[string]string{"coding": "Code review or development"}}}, NoulThresholds: map[string]float64{"tools": 0, "reasoning": 1}}}
	if err := router.Validate(); err != nil {
		t.Fatal(err)
	}
	encoded, err := yaml.Marshal(router)
	if err != nil {
		t.Fatal(err)
	}
	var decoded RouterConfig
	if err := yaml.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatal(err)
	}
	got := decoded.Classifier
	if got.Timeout != "750ms" || got.MinimumConfidence != .4 || got.Questions["task_type"].Criteria["coding"] != "Code review or development" || len(got.NoulThresholds) != 2 || got.NoulThresholds["reasoning"] != 1 {
		t.Fatalf("roundtrip = %+v", got)
	}
	// Disabling inference from this classifier preserves its optional drafts.
	got.Type, got.Transport, got.Provider, got.Model = "disabled", "", "", ""
	if err := (RouterConfig{Classifier: got}).Validate(); err != nil {
		t.Fatalf("disabled tuning drafts rejected: %v", err)
	}
}

func TestClassifierTuningRejectsInvalidControls(t *testing.T) {
	for _, mutate := range []func(*ClassifierConfig){
		func(c *ClassifierConfig) { c.Timeout = "forever" },
		func(c *ClassifierConfig) { c.Timeout = "0s" },
		func(c *ClassifierConfig) { c.Timeout = "1us" },
		func(c *ClassifierConfig) { c.Timeout = "301s" },
		func(c *ClassifierConfig) { c.MinimumConfidence = -.1 },
		func(c *ClassifierConfig) { c.MinimumConfidence = 1.1 },
		func(c *ClassifierConfig) { c.MinimumConfidence = math.NaN() },
		func(c *ClassifierConfig) { c.MinimumConfidence = math.Inf(1) },
		func(c *ClassifierConfig) {
			c.Questions = map[string]classifier.QuestionOverride{"task_type": {Criteria: map[string]string{"other": "New enum"}}}
		},
		func(c *ClassifierConfig) { c.NoulThresholds = map[string]float64{"complexity": .5} },
		func(c *ClassifierConfig) {
			c.Type = "model"
			c.Model = "physical/model"
			c.Questions = map[string]classifier.QuestionOverride{"tools": {Instructions: "Typed only"}}
		},
	} {
		c := ClassifierConfig{Type: "jev", Transport: "provider", Provider: "laya", Model: "english"}
		mutate(&c)
		if (RouterConfig{Classifier: c}).Validate() == nil {
			t.Fatalf("accepted %+v", c)
		}
	}
}
