package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// ErrInvalidRequest identifies malformed or unsupported client request data.
// Callers may use it to distinguish validation failures from upstream errors.
var ErrInvalidRequest = errors.New("invalid OpenAI request")

// ErrInvalidStream identifies a top-level stream value that is not a boolean.
var ErrInvalidStream = errors.New("invalid stream field")

// RequestFields contains the top-level fields needed before routing a request.
type RequestFields struct {
	Model  string
	Stream bool
}

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
	appendURLPath(&u, endpoint)
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
		return nil, fmt.Errorf("%w: upstream model ID must not be empty", ErrInvalidRequest)
	}
	fields, modelStart, modelEnd, err := inspectRequest(body)
	if err != nil {
		return nil, err
	}
	if fields.Stream {
		return nil, invalid("streaming is not supported yet")
	}
	if modelStart < 0 {
		return nil, fmt.Errorf("%w: model is required", ErrInvalidRequest)
	}

	encodedModel, _ := json.Marshal(model)
	rewritten := make([]byte, 0, len(body)-(modelEnd-modelStart)+len(encodedModel))
	rewritten = append(rewritten, body[:modelStart]...)
	rewritten = append(rewritten, encodedModel...)
	rewritten = append(rewritten, body[modelEnd:]...)
	return rewritten, nil
}

// InspectRequest validates a JSON object and extracts routing fields without
// copying unrelated values such as messages or image payloads.
func InspectRequest(body []byte) (RequestFields, error) {
	fields, _, _, err := inspectRequest(body)
	return fields, err
}

func inspectRequest(body []byte) (RequestFields, int, int, error) {
	i := skipSpace(body, 0)
	if i >= len(body) || body[i] != '{' {
		return RequestFields{}, -1, -1, invalid("request must be a JSON object")
	}
	i++
	fields, modelStart, modelEnd := RequestFields{}, -1, -1
	expectField := false
	for {
		i = skipSpace(body, i)
		if i < len(body) && body[i] == '}' {
			if expectField {
				return fields, -1, -1, invalid("expected field after comma")
			}
			i++
			break
		}
		expectField = false
		keyStart := i
		next, err := scanString(body, i)
		if err != nil {
			return fields, -1, -1, err
		}
		var key string
		if err := json.Unmarshal(body[keyStart:next], &key); err != nil {
			return fields, -1, -1, invalid("invalid field name")
		}
		i = skipSpace(body, next)
		if i >= len(body) || body[i] != ':' {
			return fields, -1, -1, invalid("expected colon after field name")
		}
		i = skipSpace(body, i+1)
		valueStart := i
		i, err = scanValue(body, i, 0)
		if err != nil {
			return fields, -1, -1, err
		}
		switch key {
		case "model":
			if modelStart >= 0 {
				return fields, -1, -1, invalid("duplicate model fields")
			}
			if err := json.Unmarshal(body[valueStart:i], &fields.Model); err != nil || fields.Model == "" {
				return fields, -1, -1, invalid("model must be a non-empty string")
			}
			modelStart, modelEnd = valueStart, i
		case "stream":
			if err := json.Unmarshal(body[valueStart:i], &fields.Stream); err != nil {
				return fields, -1, -1, fmt.Errorf("%w: %w", ErrInvalidRequest, ErrInvalidStream)
			}
		}
		i = skipSpace(body, i)
		if i >= len(body) {
			return fields, -1, -1, invalid("unterminated object")
		}
		if body[i] == ',' {
			i++
			expectField = true
			continue
		}
		if body[i] == '}' {
			i++
			break
		}
		return fields, -1, -1, invalid("expected comma or end of object")
	}
	if skipSpace(body, i) != len(body) {
		return fields, -1, -1, invalid("request must contain one JSON value")
	}
	return fields, modelStart, modelEnd, nil
}

func scanValue(b []byte, i, depth int) (int, error) {
	if i >= len(b) {
		return i, invalid("missing field value")
	}
	if depth > 10000 {
		return i, invalid("maximum JSON depth exceeded")
	}
	switch b[i] {
	case '"':
		return scanString(b, i)
	case '{':
		return scanComposite(b, i, '}', true, depth)
	case '[':
		return scanComposite(b, i, ']', false, depth)
	default:
		start := i
		for i < len(b) && !strings.ContainsRune(" \t\r\n,}]", rune(b[i])) {
			i++
		}
		if !json.Valid(b[start:i]) {
			return i, invalid("invalid JSON value")
		}
		return i, nil
	}
}

func scanComposite(b []byte, i int, closing byte, object bool, depth int) (int, error) {
	i++
	i = skipSpace(b, i)
	if i < len(b) && b[i] == closing {
		return i + 1, nil
	}
	for {
		if object {
			var err error
			i, err = scanString(b, i)
			if err != nil {
				return i, err
			}
			i = skipSpace(b, i)
			if i >= len(b) || b[i] != ':' {
				return i, invalid("expected colon after field name")
			}
			i = skipSpace(b, i+1)
		}
		var err error
		i, err = scanValue(b, i, depth+1)
		if err != nil {
			return i, err
		}
		i = skipSpace(b, i)
		if i >= len(b) {
			return i, invalid("unterminated JSON value")
		}
		if b[i] == closing {
			return i + 1, nil
		}
		if b[i] != ',' {
			return i, invalid("expected comma or end of value")
		}
		i = skipSpace(b, i+1)
	}
}

func scanString(b []byte, i int) (int, error) {
	start := i
	if i >= len(b) || b[i] != '"' {
		return i, invalid("expected string field name")
	}
	for i++; i < len(b); i++ {
		if b[i] == '"' {
			if !json.Valid(b[start : i+1]) {
				return i, invalid("invalid JSON string")
			}
			return i + 1, nil
		}
		if b[i] == '\\' {
			i++
			if i >= len(b) {
				break
			}
		}
		if b[i] < 0x20 {
			return i, invalid("invalid control character in string")
		}
	}
	return i, invalid("unterminated string")
}

func skipSpace(b []byte, i int) int {
	for i < len(b) && strings.ContainsRune(" \t\r\n", rune(b[i])) {
		i++
	}
	return i
}
func invalid(message string) error { return fmt.Errorf("%w: %s", ErrInvalidRequest, message) }

func appendURLPath(u *url.URL, endpoint string) {
	escapedPath := strings.TrimRight(u.EscapedPath(), "/") + endpoint
	path, err := url.PathUnescape(escapedPath)
	if err != nil {
		panic("url.URL returned an invalid escaped path: " + err.Error())
	}
	u.Path = path
	u.RawPath = escapedPath
}
