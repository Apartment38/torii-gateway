// Package web serves the embedded Torii Gateway configuration console.
package web

import (
	"embed"
	"mime"
	"net/http"
	"strings"

	"github.com/Apartment38/torii-gateway/assets"
)

//go:embed static/*
var staticFiles embed.FS

// Handler returns the unauthenticated web console handler. Authentication is
// deliberately represented at the request boundary so a future Basic Auth or
// Authentik middleware can wrap this handler without changing the UI.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/web/torii-gateway-") {
			name := strings.TrimPrefix(r.URL.Path, "/web/")
			data, err := assets.Files.ReadFile(name)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(data)
			return
		}
		if r.URL.Path == "/" {
			r.URL.Path = "/index.html"
		}
		if strings.HasPrefix(r.URL.Path, "/web/") {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/web")
		}
		name := "static" + r.URL.Path
		data, err := staticFiles.ReadFile(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if contentType := mime.TypeByExtension(r.URL.Path[strings.LastIndex(r.URL.Path, "."):]); contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		_, _ = w.Write(data)
	})
}
