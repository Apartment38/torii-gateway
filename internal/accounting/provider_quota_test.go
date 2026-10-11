package accounting

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Apartment38/torii-gateway/internal/provider"
	"github.com/Apartment38/torii-gateway/internal/quota"
)

type accountQuotaProvider struct{ calls int }

func (*accountQuotaProvider) ID() string                                       { return "p" }
func (*accountQuotaProvider) Models(context.Context) ([]provider.Model, error) { return nil, nil }
func (*accountQuotaProvider) Forward(context.Context, string, []byte, string) (*http.Response, error) {
	return nil, nil
}
func (*accountQuotaProvider) QuotaKey(string) string { return "p/@account" }
func (p *accountQuotaProvider) Quota(context.Context, string) (quota.Status, error) {
	p.calls++
	return quota.Status{Windows: []quota.Window{{Name: "primary", Unit: "percent", Limit: 100, Used: 25}}}, nil
}

func TestMeteredProviderPreservesAccountQuotaSharing(t *testing.T) {
	p := &accountQuotaProvider{}
	wrapped := Wrap(p, nil, Settings{}, nil)
	c := quota.NewCache(time.Minute, wrapped.(quota.Fetcher))
	for _, model := range []string{"p/model-a", "p/model-b"} {
		s, err := c.Get(context.Background(), model)
		if err != nil || !s.Known || s.FractionRemaining != .75 {
			t.Fatalf("quota: %+v %v", s, err)
		}
	}
	if p.calls != 1 {
		t.Fatalf("account fetched %d times", p.calls)
	}
}
