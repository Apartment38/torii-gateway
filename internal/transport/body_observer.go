package transport

import "context"

type requestBodyObserverKey struct{}

// WithRequestBodyObserver observes the final rewritten protocol body for a
// single upstream call. HTTP headers and credentials are never supplied.
func WithRequestBodyObserver(ctx context.Context, observe func([]byte)) context.Context {
	return context.WithValue(ctx, requestBodyObserverKey{}, observe)
}
