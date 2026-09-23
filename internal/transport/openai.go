package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// OpenAICompatible forwards requests to an upstream which implements the
// OpenAI HTTP API. A transport is intended to be shared for the lifetime of a
// provider so its HTTP client's connection pool is reused.
type OpenAICompatible struct {
	baseURL *url.URL
	client  *http.Client
}

// NewOpenAICompatible constructs a transparent OpenAI-compatible transport.
func NewOpenAICompatible(baseURL string, client *http.Client) (*OpenAICompatible, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse upstream base URL: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("upstream base URL must be an HTTP(S) URL without credentials, query, or fragment")
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &OpenAICompatible{baseURL: u, client: client}, nil
}

// Do sends a non-streaming JSON request after replacing only the top-level
// model field. Unknown fields remain raw JSON values and are passed through.
// The caller owns the returned response body.
func (t *OpenAICompatible) Do(ctx context.Context, endpoint string, body []byte, model string) (*http.Response, error) {
	if t == nil || t.baseURL == nil || t.client == nil {
		return nil, errors.New("OpenAI-compatible transport is not initialized")
	}
	if !strings.HasPrefix(endpoint, "/") || strings.Contains(endpoint, "?") {
		return nil, errors.New("upstream endpoint must be an absolute path without a query")
	}
	rewritten, err := rewriteRequest(body, model)
	if err != nil {
		return nil, err
	}
	u := *t.baseURL
	u.Path = strings.TrimRight(u.Path, "/") + endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(rewritten))
	if err != nil {
		return nil, fmt.Errorf("create upstream request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	response, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send upstream request: %w", err)
	}
	return response, nil
}

func rewriteRequest(body []byte, model string) ([]byte, error) {
	if model == "" {
		return nil, errors.New("upstream model ID must not be empty")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("decode OpenAI request: %w", err)
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return nil, errors.New("OpenAI request must be a JSON object")
	}

	var modelStart, modelEnd int64 = -1, -1
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("decode OpenAI request: %w", err)
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, errors.New("OpenAI request contains a non-string field name")
		}
		beforeValue := decoder.InputOffset()
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, fmt.Errorf("decode OpenAI request field %q: %w", key, err)
		}
		afterValue := decoder.InputOffset()
		switch key {
		case "model":
			if modelStart >= 0 {
				return nil, errors.New("OpenAI request contains duplicate model fields")
			}
			relativeStart := bytes.Index(body[beforeValue:afterValue], raw)
			if relativeStart < 0 {
				return nil, errors.New("locate model field in OpenAI request")
			}
			modelStart = beforeValue + int64(relativeStart)
			modelEnd = modelStart + int64(len(raw))
		case "stream":
			var stream bool
			if err := json.Unmarshal(raw, &stream); err != nil {
				return nil, errors.New("OpenAI request field stream must be a boolean")
			}
			if stream {
				return nil, errors.New("streaming is not supported yet")
			}
		}
	}
	if _, err := decoder.Token(); err != nil {
		return nil, fmt.Errorf("decode OpenAI request: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, err
	}
	if modelStart < 0 {
		return nil, errors.New("OpenAI request field model is required")
	}

	encodedModel, _ := json.Marshal(model)
	rewritten := make([]byte, 0, len(body)-int(modelEnd-modelStart)+len(encodedModel))
	rewritten = append(rewritten, body[:modelStart]...)
	rewritten = append(rewritten, encodedModel...)
	rewritten = append(rewritten, body[modelEnd:]...)
	return rewritten, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return fmt.Errorf("decode OpenAI request: %w", err)
		}
		return errors.New("OpenAI request must contain one JSON value")
	}
	return nil
}
