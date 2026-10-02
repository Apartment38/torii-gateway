package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/Apartment38/torii-gateway/internal/buildinfo"
	"github.com/Apartment38/torii-gateway/internal/state"
)

func printVersion(args []string, output io.Writer) error {
	if len(args) != 0 {
		return errors.New("usage: gateway version")
	}
	_, err := fmt.Fprintf(output, "Torii Gateway %s\nCommit: %s\nBuilt: %s\nDatabase schema: %d (supported baseline: %d)\nEncryption key version: 1\n", buildinfo.Version, buildinfo.Commit, buildinfo.BuildDate, state.SchemaVersion, state.BaselineSchemaVersion)
	return err
}
