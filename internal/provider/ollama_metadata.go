package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Facts are read only from Ollama's native show response. A successful names
// listing is insufficient to claim model or endpoint support.
func (o *Ollama) enrichModel(ctx context.Context, model *Model) {
	u := *o.baseURL
	escaped := strings.TrimRight(u.EscapedPath(), "/") + "/api/show"
	path, err := url.PathUnescape(escaped)
	if err != nil {
		return
	}
	u.Path, u.RawPath = path, escaped
	name := strings.TrimPrefix(model.ID, o.id+"/")
	body, _ := json.Marshal(map[string]string{"model": name})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := o.client.Do(req)
	if err != nil {
		return
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return
	}
	var show struct {
		Capabilities []string                   `json:"capabilities"`
		RemoteHost   string                     `json:"remote_host"`
		RemoteModel  string                     `json:"remote_model"`
		ModelInfo    map[string]json.RawMessage `json:"model_info"`
	}
	limited := &io.LimitedReader{R: response.Body, N: maxModelCatalogSize + 1}
	decoder := json.NewDecoder(limited)
	if decoder.Decode(&show) != nil || limited.N <= 0 || show.Capabilities == nil {
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF || limited.N <= 0 {
		return
	}
	capabilities := make(map[string]bool, len(show.Capabilities))
	for _, capability := range show.Capabilities {
		capabilities[capability] = true
	}
	local := show.RemoteHost == "" && show.RemoteModel == ""
	// A missing location on an incomplete/old/cloud response is unknown. A
	// native architecture record with no remote alias verifies local execution.
	var locality *bool
	if !local || len(show.ModelInfo["general.architecture"]) > 0 {
		locality = &local
	}
	model.Discovery = &ModelDiscovery{
		Source: "ollama/api/show", Local: locality,
		Endpoints:    map[string]bool{"chat_completions": capabilities["completion"], "completions": capabilities["completion"], "embeddings": capabilities["embedding"]},
		Capabilities: map[string]bool{"tools": capabilities["tools"], "vision": capabilities["vision"], "reasoning": capabilities["thinking"]},
	}
	if capabilities["completion"] {
		// Streaming is implemented by Ollama's compatible chat/completion
		// transport. Local completion models also support schema-constrained
		// output through response_format. Require native architecture evidence;
		// incomplete discovery and cloud aliases retain unknown support.
		model.Discovery.Capabilities["streaming"] = true
		if locality != nil && *locality {
			model.Discovery.Capabilities["structured_output"] = true
		}
	}
	var architecture string
	if json.Unmarshal(show.ModelInfo["general.architecture"], &architecture) == nil && architecture != "" {
		var contextLength int64
		if json.Unmarshal(show.ModelInfo[architecture+".context_length"], &contextLength) == nil && contextLength > 0 {
			model.ContextWindow = &contextLength
		}
	}
}
