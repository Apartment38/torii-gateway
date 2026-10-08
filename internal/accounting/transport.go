package accounting

import (
	"bytes"
	"encoding/json"
	"github.com/Apartment38/torii-gateway/internal/catalog"
	"io"
	"net/http"
)

// Transport instruments the legacy direct typed classifier's HTTP call. Other
// providers are observed after adapter translation through the provider wrapper.
type Transport struct {
	Base     http.RoundTripper
	Store    *Store
	Settings Settings
	Describe func(string) catalog.Descriptor
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if SubjectFrom(req.Context()) == nil {
		return t.Base.RoundTrip(req)
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	req.Body.Close()
	clone := req.Clone(req.Context())
	clone.Body = io.NopCloser(bytes.NewReader(body))
	var fields struct {
		Model string `json:"model"`
	}
	json.Unmarshal(body, &fields)
	m := &Metered{id: "classifier", Store: t.Store, Settings: t.Settings, Describe: t.Describe}
	return m.forward(req.Context(), "/v1/systemone", body, fields.Model, func() (*http.Response, error) { return t.Base.RoundTrip(clone) })
}
