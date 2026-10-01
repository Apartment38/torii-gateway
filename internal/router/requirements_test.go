package router_test

import (
	"bytes"
	"github.com/Apartment38/torii-gateway/internal/router"
	"testing"
)

func TestExtractRequirementsObjectiveFeatures(t *testing.T) {
	body := []byte(`{"model":"smart","messages":[{"role":"user","content":[{"type":"text","text":"hello"},{"type":"image_url","image_url":{"url":"data:image/png;base64,AA"}}]}],"tools":[{"type":"function"}],"response_format":{"type":"json_schema"},"reasoning_effort":"high","max_completion_tokens":500,"stream":true,"unknown":{"preserved":true}}`)
	before := append([]byte(nil), body...)
	got, err := router.ExtractRequirements(router.EndpointChat, body)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Tools || !got.Vision || !got.StructuredOutput || !got.Reasoning || !got.Streaming || got.RequestedOutputTokens != 500 || got.EstimatedInputTokens <= 0 {
		t.Fatalf("requirements=%#v", got)
	}
	if !bytes.Equal(body, before) {
		t.Fatal("extractor changed raw request")
	}
}
func TestExtractRequirementsEmbeddings(t *testing.T) {
	got, err := router.ExtractRequirements(router.EndpointEmbeddings, []byte(`{"model":"m","input":["one","two"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got.Endpoint != router.EndpointEmbeddings || got.EstimatedInputTokens == 0 {
		t.Fatalf("requirements=%#v", got)
	}
}
func TestExtractRequirementsRejectsMalformedDocuments(t *testing.T) {
	for _, body := range []string{`[]`, `{}`, `{} {}`, `{"stream":`} {
		_, err := router.ExtractRequirements(router.EndpointChat, []byte(body))
		if body == `{}` {
			if err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err == nil {
			t.Fatalf("accepted %q", body)
		}
	}
}
