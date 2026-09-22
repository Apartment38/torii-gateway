package provider

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const providerIDPattern = `[a-z0-9][a-z0-9._-]*`

var providerIDRegexp = regexp.MustCompile(`^` + providerIDPattern + `$`)

// ValidateID checks whether id is a valid provider namespace.
func ValidateID(id string) error {
	if !providerIDRegexp.MatchString(id) {
		return fmt.Errorf("provider ID must match %s", providerIDPattern)
	}
	return nil
}

// ParseModelID splits a public model ID into its provider namespace and
// upstream model ID. Only the first slash is a separator because upstream
// model IDs, such as OpenRouter model IDs, may contain slashes themselves.
func ParseModelID(publicID string) (providerID, modelID string, err error) {
	providerID, modelID, found := strings.Cut(publicID, "/")
	if !found {
		return "", "", errors.New("model ID must have the form <provider>/<model>")
	}
	if err := ValidateID(providerID); err != nil {
		return "", "", err
	}
	if modelID == "" {
		return "", "", errors.New("upstream model ID must not be empty")
	}
	return providerID, modelID, nil
}
