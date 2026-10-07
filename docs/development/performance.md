# Performance baseline

The repository contains a reproducible transparent-dispatch benchmark:

```sh
GOCACHE=/tmp/torii-go-cache go test ./internal/api -run '^$' -bench BenchmarkTransparentHandlerDispatch -benchmem -count=5
```

The benchmark measures gateway request inspection, provider lookup, model
rewrite dispatch, and response handoff with an in-memory provider. It excludes
upstream network latency by design. Provider discovery, classifier latency,
SSE first-byte latency, and concurrent session behavior must be measured with
the documented fake-server integration harness before publishing release
numbers; no machine-specific numbers are treated as product guarantees.
