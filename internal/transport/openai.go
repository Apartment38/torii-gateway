package transport

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// RequestModel reads the model from a top-level OpenAI request object while
// strictly validating its JSON syntax. Unknown fields are inspected but not
// normalized, so callers can continue forwarding the original request bytes.
func RequestModel(body []byte) (string, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	token, err := decoder.Token()
	if err != nil {
		return "", fmt.Errorf("decode request: %w", err)
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return "", errors.New("decode request: top-level value must be an object")
	}

	var model string
	found := false
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return "", fmt.Errorf("decode request field: %w", err)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return "", fmt.Errorf("decode request field %q: %w", key, err)
		}
		if key == "model" {
			if err := json.Unmarshal(value, &model); err != nil {
				return "", errors.New("decode request: model must be a string")
			}
			found = true
		}
	}
	if _, err := decoder.Token(); err != nil {
		// Decoder.More reports false both before a valid closing brace and when a
		// comma is followed by one. Token performs the required strict check.
		return "", fmt.Errorf("decode request object: %w", err)
	}
	if token, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err != nil {
			return "", fmt.Errorf("decode trailing request data: %w", err)
		}
		return "", fmt.Errorf("decode request: unexpected trailing value %v", token)
	}
	if !found {
		return "", errors.New("decode request: model is required")
	}
	return model, nil
}
