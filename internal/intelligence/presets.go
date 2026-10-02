package intelligence

import (
	"embed"
	"encoding/json"
	"strings"
)

// Built-in presets are shared with the generated console, avoiding separately
// maintained browser schemas. Explicit operator datasets and mappings still win.
//
//go:embed presets.json
var presetFiles embed.FS

type SourcePreset struct {
	Name           string         `json:"name"`
	RequiresAPIKey bool           `json:"requires_api_key"`
	Note           string         `json:"note"`
	Settings       SourceSettings `json:"settings"`
}

func SourcePresets() []SourcePreset {
	data, _ := presetFiles.ReadFile("presets.json")
	var presets []SourcePreset
	if err := json.Unmarshal(data, &presets); err != nil {
		panic("invalid built-in benchmark presets")
	}
	return presets
}

func WithSourceDefaults(cfg SourceSettings) SourceSettings {
	for _, preset := range SourcePresets() {
		if preset.Settings.Type != cfg.Type {
			continue
		}
		if cfg.URL == "" || (cfg.Type == "artificial_analysis" && cfg.URL == "https://artificialanalysis.ai/api/v2/language/models/free") {
			cfg.URL = preset.Settings.URL
		}
		if len(cfg.Datasets) == 0 || (cfg.Type == "huggingface" && len(cfg.Datasets) == 1 && (strings.TrimSpace(cfg.Datasets[0].ID) == "" || cfg.Datasets[0].ID == "org/your-leaderboard")) {
			cfg.Datasets = preset.Settings.Datasets
		}
		return cfg
	}
	return cfg
}
