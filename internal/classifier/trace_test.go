package classifier

import (
	"context"
	"github.com/Apartment38/torii-gateway/internal/provider"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecisionTraceCapturesProtocolWithoutChangingClassification(t *testing.T) {
	p := &recordingDecision{}
	j, err := NewJevTransport(p, "english")
	if err != nil {
		t.Fatal(err)
	}
	ctx, trace := WithDecisionTrace(context.Background())
	got, err := j.Classify(ctx, ClassificationRequest{Text: "private prompt"})
	if err != nil {
		t.Fatal(err)
	}
	request, response, truncated := trace.Snapshot()
	if request != string(p.body) || response != layaDecision || truncated {
		t.Fatal("protocol bodies changed")
	}
	plain, err := j.Classify(context.Background(), ClassificationRequest{Text: "private prompt"})
	if err != nil || got.TaskType != plain.TaskType || got.Complexity != plain.Complexity {
		t.Fatal("capture changed decision", err)
	}
}
func TestDecisionTraceBoundsAndCapturesInvalidResponses(t *testing.T) {
	ctx, trace := WithDecisionTrace(context.Background())
	recordDecisionRequest(ctx, []byte(strings.Repeat("x", 70<<10)))
	_, err := decodeObservedJevResponse(ctx, response(`{"invalid":"private response"}`), ClassificationRequest{}, DecisionOptions{})
	if err == nil {
		t.Fatal("accepted invalid response")
	}
	request, body, truncated := trace.Snapshot()
	if len(request) != 64<<10 || !truncated || body != `{"invalid":"private response"}` {
		t.Fatal("invalid capture", len(request), body, truncated)
	}
}

func TestDecisionTraceUsesFinalProviderModelRewrite(t *testing.T) {
	var wire string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		wire = string(data)
		w.Write([]byte(layaDecision))
	}))
	defer server.Close()
	p, err := provider.NewJevProvider(provider.OpenAICompatibleConfig{ID: "jev", BaseURL: server.URL, StaticModels: []string{"alias"}, ModelRewrites: map[string]string{"alias": "actual"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	j, err := NewJevTransport(p, "alias")
	if err != nil {
		t.Fatal(err)
	}
	ctx, trace := WithDecisionTrace(context.Background())
	if _, err = j.Classify(ctx, ClassificationRequest{Text: "hi"}); err != nil {
		t.Fatal(err)
	}
	request, _, _ := trace.Snapshot()
	if request != wire || !strings.Contains(request, `"model":"actual"`) {
		t.Fatal("capture did not match final provider request", request, wire)
	}
}
