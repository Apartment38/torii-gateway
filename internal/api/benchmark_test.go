package api_test

import (
	"context"
	"github.com/Apartment38/torii-gateway/internal/api"
	"github.com/Apartment38/torii-gateway/internal/provider"
	"net/http"
	"strings"
	"testing"
)

type benchmarkProvider struct{}

func (benchmarkProvider) ID() string { return "bench" }
func (benchmarkProvider) Models(context.Context) ([]provider.Model, error) {
	return []provider.Model{{ID: "bench/model", Object: "model"}}, nil
}
func (benchmarkProvider) Forward(context.Context, string, []byte, string) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: http.NoBody}, nil
}

func BenchmarkTransparentHandlerDispatch(b *testing.B) {
	h, err := api.NewOpenAIHandler([]provider.OpenAIProvider{benchmarkProvider{}})
	if err != nil {
		b.Fatal(err)
	}
	request := `{"model":"bench/model","messages":[{"role":"user","content":"hello"}]}`
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		response := newRecorder()
		h.ServeHTTP(response, newRequest(request))
	}
}

// Small wrappers avoid pulling test-only HTTP helpers into production code.
func newRecorder() *responseRecorder { return &responseRecorder{header: make(http.Header)} }
func newRequest(body string) *http.Request {
	request, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	return request
}

type responseRecorder struct {
	header http.Header
	code   int
}

func (r *responseRecorder) Header() http.Header { return r.header }
func (r *responseRecorder) Write(p []byte) (int, error) {
	if r.code == 0 {
		r.code = 200
	}
	return len(p), nil
}
func (r *responseRecorder) WriteHeader(code int) { r.code = code }
