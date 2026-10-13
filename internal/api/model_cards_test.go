package api

import "testing"

func TestOfficialModelCard(t *testing.T) {
	for model, want := range map[string]string{
		"openai/gpt-4o":        "https://openai.com/index/gpt-4o-system-card/",
		"gemma3n:e2b":          "https://huggingface.co/google/gemma-3n-E2B-it",
		"google/gemma-3-4b-it": "https://huggingface.co/google/gemma-3-4b-it",
		"Qwen/Qwen3-8B":        "https://huggingface.co/Qwen/Qwen3-8B",
		"gemma4:e2b":           "", "unknown/model": "", "google/../private": "", "google/foo?redirect=evil": "", "deepseek-chat": "",
	} {
		if got := officialModelCard(model); got != want {
			t.Errorf("%s got %s want %s", model, got, want)
		}
	}
}
