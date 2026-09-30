// Package catalog builds immutable, deterministic model catalog snapshots.
package catalog

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/Apartment38/torii-router/internal/provider"
)

// TriState represents verified support, verified non-support, or unknown.
type TriState string

const (
	Unknown     TriState = "unknown"
	Supported   TriState = "supported"
	Unsupported TriState = "unsupported"
)

// EndpointCapabilities records which public endpoint families a model accepts.
type EndpointCapabilities struct {
	ChatCompletions TriState `json:"chat_completions"`
	Completions     TriState `json:"completions"`
	Responses       TriState `json:"responses"`
	Embeddings      TriState `json:"embeddings"`
}

// ModelCapabilities records model features independently of endpoints.
type ModelCapabilities struct {
	Streaming        TriState `json:"streaming"`
	Tools            TriState `json:"tools"`
	Vision           TriState `json:"vision"`
	StructuredOutput TriState `json:"structured_output"`
	Reasoning        TriState `json:"reasoning"`
}

// Modalities records verified model input and output media support.
type Modalities struct {
	TextInput   TriState `json:"text_input"`
	TextOutput  TriState `json:"text_output"`
	ImageInput  TriState `json:"image_input"`
	AudioInput  TriState `json:"audio_input"`
	AudioOutput TriState `json:"audio_output"`
}

type Pricing struct {
	InputPerMillion  *float64 `json:"input_per_million,omitempty"`
	OutputPerMillion *float64 `json:"output_per_million,omitempty"`
}

type Quality struct {
	General   *float64 `json:"general,omitempty"`
	Tools     *float64 `json:"tools,omitempty"`
	Reasoning *float64 `json:"reasoning,omitempty"`
}

type Availability struct {
	Available bool      `json:"available"`
	Stale     bool      `json:"stale"`
	CheckedAt time.Time `json:"checked_at"`
}

type Provenance struct {
	CatalogSource    string `json:"catalog_source"`
	CapabilitySource string `json:"capability_source,omitempty"`
}

// Descriptor is the normalized metadata for one physical model. Snapshots
// contain values only, so callers cannot mutate catalog-owned state.
type Descriptor struct {
	PublicID      string               `json:"id"`
	ProviderID    string               `json:"provider_id"`
	UpstreamID    string               `json:"upstream_id"`
	Created       int64                `json:"created,omitempty"`
	OwnedBy       string               `json:"owned_by,omitempty"`
	Endpoints     EndpointCapabilities `json:"endpoints"`
	Capabilities  ModelCapabilities    `json:"capabilities"`
	Modalities    Modalities           `json:"modalities"`
	ContextWindow *int64               `json:"context_window,omitempty"`
	Pricing       Pricing              `json:"pricing"`
	Quality       Quality              `json:"quality"`
	Local         *bool                `json:"local,omitempty"`
	Availability  Availability         `json:"availability"`
	Metadata      Provenance           `json:"metadata"`
}

//go:embed capabilities.json
var bundledJSON []byte

func Bundled() ([]Descriptor, error) {
	var records []Descriptor
	if err := json.Unmarshal(bundledJSON, &records); err != nil {
		return nil, fmt.Errorf("decode bundled capabilities: %w", err)
	}
	return append([]Descriptor(nil), records...), nil
}

type providerState struct {
	descriptors []Descriptor
}

// Catalog refreshes providers independently and atomically publishes snapshots.
type Catalog struct {
	mu        sync.RWMutex
	providers []provider.Provider
	bundled   []Descriptor
	overrides []Descriptor
	state     map[string]providerState
	snapshot  []Descriptor
	now       func() time.Time
}

func New(providers []provider.Provider, bundled, overrides []Descriptor) (*Catalog, error) {
	seen := make(map[string]struct{}, len(providers))
	for _, p := range providers {
		if p == nil {
			return nil, errors.New("catalog: nil provider")
		}
		if _, ok := seen[p.ID()]; ok {
			return nil, fmt.Errorf("catalog: duplicate provider %q", p.ID())
		}
		seen[p.ID()] = struct{}{}
	}
	if bundled == nil {
		var err error
		bundled, err = Bundled()
		if err != nil {
			return nil, err
		}
	}
	return &Catalog{providers: append([]provider.Provider(nil), providers...), bundled: append([]Descriptor(nil), bundled...), overrides: append([]Descriptor(nil), overrides...), state: make(map[string]providerState), now: time.Now}, nil
}

// Refresh discovers each provider. Failed providers retain their last-known-good
// entries as stale; one provider failure never hides successful providers.
func (c *Catalog) Refresh(ctx context.Context) error {
	now := c.now().UTC()
	c.mu.Lock()
	defer c.mu.Unlock()
	nextState := make(map[string]providerState, len(c.state))
	for id, state := range c.state {
		nextState[id] = providerState{descriptors: cloneDescriptors(state.descriptors)}
	}
	usable := 0
	for _, p := range c.providers {
		models, err := p.Models(ctx)
		if err != nil {
			if previous, ok := nextState[p.ID()]; ok {
				for i := range previous.descriptors {
					previous.descriptors[i].Availability.Stale = true
				}
				nextState[p.ID()] = previous
				usable++
			} else if fallback := bundledForProvider(c.bundled, p.ID(), now); len(fallback) != 0 {
				for i := range fallback {
					fallback[i] = mergeMatching(fallback[i], c.overrides, "config")
				}
				nextState[p.ID()] = providerState{descriptors: fallback}
				usable++
			}
			continue
		}
		descriptors := make([]Descriptor, 0, len(models))
		for _, model := range models {
			modelProvider, upstream, err := provider.ParseModelID(model.ID)
			if err != nil || modelProvider != p.ID() {
				return fmt.Errorf("catalog: provider %q returned invalid model ID %q", p.ID(), model.ID)
			}
			d := unknownDescriptor(model.ID, p.ID(), upstream)
			d.Created, d.OwnedBy = model.Created, model.OwnedBy
			d.Availability = Availability{Available: true, CheckedAt: now}
			d.Metadata.CatalogSource = "live"
			d = mergeMatching(d, c.bundled, "bundled")
			d = mergeMatching(d, c.overrides, "config")
			descriptors = append(descriptors, d)
		}
		nextState[p.ID()] = providerState{descriptors: descriptors}
		usable++
	}
	if usable == 0 {
		return errors.New("catalog: no usable providers")
	}
	var combined []Descriptor
	ids := make(map[string]string)
	for providerID, state := range nextState {
		for _, d := range state.descriptors {
			if owner, exists := ids[d.PublicID]; exists {
				return fmt.Errorf("catalog: model ID collision %q between %q and %q", d.PublicID, owner, providerID)
			}
			ids[d.PublicID] = providerID
			combined = append(combined, d)
		}
	}
	sort.Slice(combined, func(i, j int) bool { return combined[i].PublicID < combined[j].PublicID })
	c.state = nextState
	c.snapshot = combined
	return nil
}

func (c *Catalog) Models() []Descriptor {
	c.mu.RLock()
	defer c.mu.RUnlock()
	models := make([]Descriptor, len(c.snapshot))
	for i := range c.snapshot {
		models[i] = clone(c.snapshot[i])
	}
	return models
}

func (c *Catalog) Model(id string) (Descriptor, bool) {
	models := c.Models()
	i := sort.Search(len(models), func(i int) bool { return models[i].PublicID >= id })
	if i == len(models) || models[i].PublicID != id {
		return Descriptor{}, false
	}
	return models[i], true
}

func unknownDescriptor(publicID, providerID, upstreamID string) Descriptor {
	return Descriptor{PublicID: publicID, ProviderID: providerID, UpstreamID: upstreamID,
		Endpoints:    EndpointCapabilities{Unknown, Unknown, Unknown, Unknown},
		Capabilities: ModelCapabilities{Unknown, Unknown, Unknown, Unknown, Unknown},
		Modalities:   Modalities{Unknown, Unknown, Unknown, Unknown, Unknown}}
}

func bundledForProvider(records []Descriptor, providerID string, now time.Time) []Descriptor {
	var result []Descriptor
	for _, record := range records {
		if record.ProviderID != providerID {
			continue
		}
		record = clone(record)
		record.Metadata.CatalogSource = "bundled"
		record.Availability = Availability{Available: true, Stale: true, CheckedAt: now}
		result = append(result, record)
	}
	return result
}

func clone(d Descriptor) Descriptor {
	d.ContextWindow = copyInt(d.ContextWindow)
	d.Pricing.InputPerMillion = copyFloat(d.Pricing.InputPerMillion)
	d.Pricing.OutputPerMillion = copyFloat(d.Pricing.OutputPerMillion)
	d.Quality.General = copyFloat(d.Quality.General)
	d.Quality.Tools = copyFloat(d.Quality.Tools)
	d.Quality.Reasoning = copyFloat(d.Quality.Reasoning)
	if d.Local != nil {
		value := *d.Local
		d.Local = &value
	}
	return d
}

func cloneDescriptors(input []Descriptor) []Descriptor {
	result := make([]Descriptor, len(input))
	for i := range input {
		result[i] = clone(input[i])
	}
	return result
}

func mergeMatching(base Descriptor, records []Descriptor, source string) Descriptor {
	for _, overlay := range records {
		if overlay.PublicID != base.PublicID {
			continue
		}
		base.Endpoints = mergeEndpoints(base.Endpoints, overlay.Endpoints)
		base.Capabilities = mergeCapabilities(base.Capabilities, overlay.Capabilities)
		base.Modalities = mergeModalities(base.Modalities, overlay.Modalities)
		if overlay.ContextWindow != nil {
			base.ContextWindow = copyInt(overlay.ContextWindow)
		}
		if overlay.Pricing.InputPerMillion != nil {
			base.Pricing.InputPerMillion = copyFloat(overlay.Pricing.InputPerMillion)
		}
		if overlay.Pricing.OutputPerMillion != nil {
			base.Pricing.OutputPerMillion = copyFloat(overlay.Pricing.OutputPerMillion)
		}
		if overlay.Quality.General != nil {
			base.Quality.General = copyFloat(overlay.Quality.General)
		}
		if overlay.Quality.Tools != nil {
			base.Quality.Tools = copyFloat(overlay.Quality.Tools)
		}
		if overlay.Quality.Reasoning != nil {
			base.Quality.Reasoning = copyFloat(overlay.Quality.Reasoning)
		}
		if overlay.Local != nil {
			v := *overlay.Local
			base.Local = &v
		}
		base.Metadata.CapabilitySource = source
	}
	return base
}

func mergeEndpoints(a, b EndpointCapabilities) EndpointCapabilities {
	if b.ChatCompletions != "" && b.ChatCompletions != Unknown {
		a.ChatCompletions = b.ChatCompletions
	}
	if b.Completions != "" && b.Completions != Unknown {
		a.Completions = b.Completions
	}
	if b.Responses != "" && b.Responses != Unknown {
		a.Responses = b.Responses
	}
	if b.Embeddings != "" && b.Embeddings != Unknown {
		a.Embeddings = b.Embeddings
	}
	return a
}
func mergeCapabilities(a, b ModelCapabilities) ModelCapabilities {
	if b.Streaming != "" && b.Streaming != Unknown {
		a.Streaming = b.Streaming
	}
	if b.Tools != "" && b.Tools != Unknown {
		a.Tools = b.Tools
	}
	if b.Vision != "" && b.Vision != Unknown {
		a.Vision = b.Vision
	}
	if b.StructuredOutput != "" && b.StructuredOutput != Unknown {
		a.StructuredOutput = b.StructuredOutput
	}
	if b.Reasoning != "" && b.Reasoning != Unknown {
		a.Reasoning = b.Reasoning
	}
	return a
}
func mergeModalities(a, b Modalities) Modalities {
	if b.TextInput != "" && b.TextInput != Unknown {
		a.TextInput = b.TextInput
	}
	if b.TextOutput != "" && b.TextOutput != Unknown {
		a.TextOutput = b.TextOutput
	}
	if b.ImageInput != "" && b.ImageInput != Unknown {
		a.ImageInput = b.ImageInput
	}
	if b.AudioInput != "" && b.AudioInput != Unknown {
		a.AudioInput = b.AudioInput
	}
	if b.AudioOutput != "" && b.AudioOutput != Unknown {
		a.AudioOutput = b.AudioOutput
	}
	return a
}
func copyInt(v *int64) *int64 {
	if v == nil {
		return nil
	}
	n := *v
	return &n
}
func copyFloat(v *float64) *float64 {
	if v == nil {
		return nil
	}
	n := *v
	return &n
}
