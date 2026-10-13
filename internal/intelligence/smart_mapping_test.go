package intelligence

import (
	"reflect"
	"testing"
)

func TestFuzzyBenchmarkCandidates(t *testing.T) {
	observations := []Observation{{ExternalModelID: "Google/Gemma-4-E2B"}, {ExternalModelID: "gemma-3-27b-it"}, {ExternalModelID: "Google/Gemma-4-E2B"}, {ExternalModelID: "Qwen-3-4b"}}
	want := []string{"Google/Gemma-4-E2B", "gemma-3-27b-it"}
	for _, name := range []string{"gemma4:e2b", "google/gemma-4-e2b-it"} {
		if got := FuzzyBenchmarkCandidates(name, observations); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: got %v want %v", name, got, want)
		}
	}
	for _, name := range []string{"", "latest", "e2b", "llama3:8b"} {
		if got := FuzzyBenchmarkCandidates(name, observations); len(got) != 0 {
			t.Fatalf("unrelated %s: %v", name, got)
		}
	}
}
