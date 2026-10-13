package api

import (
	"net/url"
	"strings"
)

// Only identified publisher repositories and known aliases produce card links.
// Unknown IDs remain blank rather than linking to a guessed repository.
func officialModelCard(upstream string) string {
	// Closed models publish system cards rather than repository README cards.
	systemCards := map[string]string{
		"gpt-4o":            "https://openai.com/index/gpt-4o-system-card/",
		"gpt-4o-2024-05-13": "https://openai.com/index/gpt-4o-system-card/",
		"gpt-4o-2024-08-06": "https://openai.com/index/gpt-4o-system-card/",
		"gpt-4o-2024-11-20": "https://openai.com/index/gpt-4o-system-card/",
		"o3":                "https://openai.com/index/o3-o4-mini-system-card/",
		"o4-mini":           "https://openai.com/index/o3-o4-mini-system-card/",
	}
	name := strings.TrimPrefix(strings.ToLower(upstream), "openai/")
	if card := systemCards[name]; card != "" {
		return card
	}
	aliases := map[string]string{
		"gemma2:2b": "google/gemma-2-2b-it", "gemma2:9b": "google/gemma-2-9b-it", "gemma2:27b": "google/gemma-2-27b-it",
		"gemma3:1b": "google/gemma-3-1b-it", "gemma3:4b": "google/gemma-3-4b-it", "gemma3:12b": "google/gemma-3-12b-it", "gemma3:27b": "google/gemma-3-27b-it",
		"gemma3n:e2b": "google/gemma-3n-E2B-it", "gemma3n:e4b": "google/gemma-3n-E4B-it",
		"deepseek-r1": "deepseek-ai/DeepSeek-R1",
	}
	if repo := aliases[strings.ToLower(upstream)]; repo != "" {
		return "https://huggingface.co/" + repo
	}
	// Provider catalogs sometimes use a publisher's exact Hugging Face repo ID.
	parts := strings.Split(upstream, "/")
	if len(parts) != 2 || parts[1] == "" || parts[1] == "." || parts[1] == ".." {
		return ""
	}
	publishers := map[string]string{"google": "google", "meta-llama": "meta-llama", "qwen": "Qwen", "deepseek-ai": "deepseek-ai", "mistralai": "mistralai", "microsoft": "microsoft", "nvidia": "nvidia", "nousresearch": "NousResearch", "allenai": "allenai"}
	publisher := publishers[strings.ToLower(parts[0])]
	if publisher == "" || strings.ContainsAny(parts[1], ":?#\\ \t\r\n") {
		return ""
	}
	return "https://huggingface.co/" + publisher + "/" + url.PathEscape(parts[1])
}
