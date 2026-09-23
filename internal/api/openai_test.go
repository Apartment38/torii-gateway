package api

import (
	"net/http"
	"testing"
)

func TestCopyResponseHeadersStripsProxyConnection(t *testing.T) {
	src := make(http.Header)
	src.Set("Proxy-Connection", "keep-alive")
	src.Set("X-Request-ID", "request-1")
	dst := make(http.Header)
	CopyResponseHeaders(dst, src)
	if got := dst.Get("Proxy-Connection"); got != "" {
		t.Fatalf("Proxy-Connection = %q, want empty", got)
	}
	if got := dst.Get("X-Request-ID"); got != "request-1" {
		t.Fatalf("X-Request-ID = %q, want request-1", got)
	}
}
