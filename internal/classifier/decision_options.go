package classifier

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

// QuestionOverride customizes a rubric without changing the routing output
// contract. Criteria updates merge into the existing named options.
type QuestionOverride struct {
	Instructions string            `yaml:"instructions,omitempty" json:"instructions,omitempty"`
	Criteria     map[string]string `yaml:"criteria,omitempty" json:"criteria,omitempty"`
}

type DecisionOptions struct {
	Questions      map[string]QuestionOverride
	NoulThresholds map[string]float64
}

func (o DecisionOptions) Validate() error {
	defaults := jevQuestions()
	for name, override := range o.Questions {
		question, ok := defaults[name]
		if !ok {
			return fmt.Errorf("unknown classifier question %q", name)
		}
		if override.Instructions != "" && !validRubricText(override.Instructions) {
			return fmt.Errorf("question %q instructions must contain 1 to 4000 characters", name)
		}
		for option, rubric := range override.Criteria {
			_, allowed := question.Criteria[option]
			if question.Type == "noul" {
				allowed = option == "true" || option == "false"
			}
			if !allowed {
				return fmt.Errorf("question %q criteria must use its existing options", name)
			}
			if !validRubricText(rubric) {
				return fmt.Errorf("question %q criterion must contain 1 to 4000 characters", name)
			}
		}
	}
	for name, threshold := range o.NoulThresholds {
		question, ok := defaults[name]
		if !ok || question.Type != "noul" {
			return fmt.Errorf("threshold %q must name a noul question", name)
		}
		if math.IsNaN(threshold) || math.IsInf(threshold, 0) || threshold < 0 || threshold > 1 {
			return fmt.Errorf("threshold %q must be finite and between zero and one", name)
		}
	}
	encoded, err := json.Marshal(o.Questions)
	if err != nil || len(encoded) > 32<<10 {
		return errors.New("classifier question overrides must not exceed 32 KiB")
	}
	return nil
}

func validRubricText(text string) bool {
	return strings.TrimSpace(text) != "" && utf8.ValidString(text) && utf8.RuneCountInString(text) <= 4000
}

func (o DecisionOptions) questions() map[string]jevQuestion {
	questions := jevQuestions()
	for name, override := range o.Questions {
		question := questions[name]
		if override.Instructions != "" {
			question.Instructions = override.Instructions
		}
		for option, rubric := range override.Criteria {
			if question.Criteria == nil {
				question.Criteria = make(map[string]string)
			}
			question.Criteria[option] = rubric
		}
		questions[name] = question
	}
	return questions
}

func (o DecisionOptions) clone() DecisionOptions {
	clone := DecisionOptions{Questions: make(map[string]QuestionOverride, len(o.Questions)), NoulThresholds: make(map[string]float64, len(o.NoulThresholds))}
	for name, override := range o.Questions {
		copied := QuestionOverride{Instructions: override.Instructions, Criteria: make(map[string]string, len(override.Criteria))}
		for option, rubric := range override.Criteria {
			copied.Criteria[option] = rubric
		}
		clone.Questions[name] = copied
	}
	for name, threshold := range o.NoulThresholds {
		clone.NoulThresholds[name] = threshold
	}
	return clone
}
