package api

import (
	"net/http"
	"strings"
)

var hopByHopResponseHeaders = map[string]struct{}{
	"connection":          {},
	"keep-alive":          {},
	"proxy-authenticate":  {},
	"proxy-authorization": {},
	"proxy-connection":    {},
	"te":                  {},
	"trailer":             {},
	"transfer-encoding":   {},
	"upgrade":             {},
}

// CopyResponseHeaders copies end-to-end upstream headers to a downstream
// response. Headers named by Connection are hop-by-hop as well as the standard
// and de facto static hop-by-hop names.
func CopyResponseHeaders(dst, src http.Header) {
	excluded := make(map[string]struct{}, len(hopByHopResponseHeaders))
	for name := range hopByHopResponseHeaders {
		excluded[name] = struct{}{}
	}
	for _, connection := range src.Values("Connection") {
		for name := range strings.SplitSeq(connection, ",") {
			excluded[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
		}
	}
	for name, values := range src {
		if _, skip := excluded[strings.ToLower(name)]; skip {
			continue
		}
		for _, value := range values {
			dst.Add(name, value)
		}
	}
}
