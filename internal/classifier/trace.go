package classifier

import (
	"bytes"
	"context"
	"github.com/Apartment38/torii-gateway/internal/transport"
	"io"
	"net/http"
	"strings"
	"sync"
)

// DecisionTrace captures bounded protocol bodies only when explicitly enabled.
// HTTP headers, including authorization credentials, never cross this boundary.
type DecisionTrace struct {
	mu                sync.Mutex
	request, response string
	truncated         bool
}
type decisionTraceKey struct{}

func WithDecisionTrace(ctx context.Context) (context.Context, *DecisionTrace) {
	trace := &DecisionTrace{}
	return context.WithValue(ctx, decisionTraceKey{}, trace), trace
}
func (t *DecisionTrace) Snapshot() (string, string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.request, t.response, t.truncated
}
func recordDecisionRequest(ctx context.Context, body []byte) {
	if trace, ok := ctx.Value(decisionTraceKey{}).(*DecisionTrace); ok {
		trace.mu.Lock()
		defer trace.mu.Unlock()
		trace.request, trace.truncated = decisionTraceText(string(body))
	}
}
func decodeObservedJevResponse(ctx context.Context, resp *http.Response, request ClassificationRequest, options DecisionOptions) (Classification, error) {
	if trace, ok := ctx.Value(decisionTraceKey{}).(*DecisionTrace); ok && resp != nil && resp.Body != nil {
		original := resp.Body
		data, err := io.ReadAll(io.LimitReader(original, maxJevResponseSize+1))
		original.Close()
		trace.mu.Lock()
		var truncated bool
		trace.response, truncated = decisionTraceText(string(data))
		trace.truncated = trace.truncated || truncated
		trace.mu.Unlock()
		if err != nil {
			return Classification{}, err
		}
		resp.Body = io.NopCloser(bytes.NewReader(data))
	}
	return decodeJevResponse(resp, request, options)
}

func decisionTraceText(value string) (string, bool) {
	const maxBytes = 64 << 10
	if len(value) <= maxBytes {
		return value, false
	}
	return strings.ToValidUTF8(value[:maxBytes], ""), true
}

func decisionRequestContext(ctx context.Context) context.Context {
	if _, ok := ctx.Value(decisionTraceKey{}).(*DecisionTrace); !ok {
		return ctx
	}
	return transport.WithRequestBodyObserver(ctx, func(body []byte) { recordDecisionRequest(ctx, body) })
}
