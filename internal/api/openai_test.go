package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Apartment38/torii-gateway/internal/api"
	"github.com/Apartment38/torii-gateway/internal/provider"
)

func TestOpenAIHandlerOllamaEndToEnd(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_, _ = io.WriteString(w, `{"data":[{"id":"qwen3","object":"model"}]}`)
		case "/v1/chat/completions":
			body, _ := io.ReadAll(r.Body)
			if strings.Contains(string(body), `local/qwen3`) || !strings.Contains(string(body), `"extension":{"keep":true}`) {
				t.Errorf("unexpected upstream body: %s", body)
			}
			w.Header().Set("X-Request-Id", "upstream-1")
			_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"hello"}}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	ollama, err := provider.NewOllama("local", upstream.URL, upstream.Client())
	if err != nil {
		t.Fatal(err)
	}
	handler, err := api.NewOpenAIHandler([]provider.OpenAIProvider{ollama})
	if err != nil {
		t.Fatal(err)
	}

	models := httptest.NewRecorder()
	handler.ServeHTTP(models, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if models.Code != http.StatusOK || !strings.Contains(models.Body.String(), `"id":"local/qwen3"`) {
		t.Fatalf("models response = %d %s", models.Code, models.Body.String())
	}
	single := httptest.NewRecorder()
	handler.ServeHTTP(single, httptest.NewRequest(http.MethodGet, "/v1/models/local/qwen3", nil))
	if single.Code != http.StatusOK || !strings.Contains(single.Body.String(), `"id":"local/qwen3"`) {
		t.Fatalf("model response = %d %s", single.Code, single.Body.String())
	}
	chat := httptest.NewRecorder()
	body := `{"model":"local/qwen3","messages":[{"role":"user","content":"hi"}],"extension":{"keep":true}}`
	handler.ServeHTTP(chat, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body)))
	if chat.Code != http.StatusOK || !strings.Contains(chat.Body.String(), `"content":"hello"`) {
		t.Fatalf("chat response = %d %s", chat.Code, chat.Body.String())
	}
	if chat.Header().Get("X-Request-Id") != "upstream-1" {
		t.Fatalf("X-Request-Id = %q", chat.Header().Get("X-Request-Id"))
	}
}

type failingProvider struct{}

func TestDecisionProvidersAreAdvertisedButDoNotAcceptChat(t *testing.T) {
	p, err := provider.NewJevProvider(provider.OpenAICompatibleConfig{ID: "laya", BaseURL: "http://localhost:8000", StaticModels: []string{"english"}}, true)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := api.NewOpenAIHandler([]provider.OpenAIProvider{p})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "laya/english") || !strings.Contains(w.Body.String(), `"decision":true`) || !strings.Contains(w.Body.String(), "smart") {
		t.Fatalf("decision model missing from catalog: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"laya/english","messages":[{"role":"user","content":"hello"}]}`)))
	if w.Code == http.StatusOK {
		t.Fatal("classifier accepted a Chat Completions request")
	}
}

func (failingProvider) ID() string { return "broken" }
func (failingProvider) Models(context.Context) ([]provider.Model, error) {
	return nil, context.Canceled
}
func (failingProvider) Forward(context.Context, string, []byte, string) (*http.Response, error) {
	return nil, context.Canceled
}

func TestOpenAIHandlerForwardsTransparentEndpoints(t *testing.T) {
	endpoints := []string{"/v1/chat/completions", "/v1/completions", "/v1/embeddings", "/v1/responses"}
	for _, endpoint := range endpoints {
		t.Run(endpoint, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != endpoint {
					t.Errorf("path = %q, want %q", r.URL.Path, endpoint)
				}
				body, _ := io.ReadAll(r.Body)
				if got, want := string(body), `{"model":"qwen3","extension":{"keep":true}}`; got != want {
					t.Errorf("body = %s, want %s", got, want)
				}
				w.Header().Set("X-Upstream", "preserved")
				w.WriteHeader(http.StatusCreated)
				_, _ = io.WriteString(w, `{"ok":true}`)
			}))
			defer upstream.Close()
			ollama, _ := provider.NewOllama("local", upstream.URL, upstream.Client())
			handler, _ := api.NewOpenAIHandler([]provider.OpenAIProvider{ollama})
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(`{"model":"local/qwen3","extension":{"keep":true}}`)))
			if response.Code != http.StatusCreated || response.Header().Get("X-Upstream") != "preserved" || response.Body.String() != `{"ok":true}` {
				t.Fatalf("response = %d, headers %v, body %s", response.Code, response.Header(), response.Body.String())
			}
		})
	}
}

func TestOpenAIHandlerReturnsPartialDeterministicCatalog(t *testing.T) {
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"data":[{"id":"z"},{"id":"a"}]}`)
	}))
	defer first.Close()
	working, _ := provider.NewOllama("working", first.URL, first.Client())
	handler, err := api.NewOpenAIHandler([]provider.OpenAIProvider{failingProvider{}, working})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	var envelope struct {
		Data []provider.Model `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Data) != 3 || envelope.Data[0].ID != "smart" || envelope.Data[1].ID != "working/a" || envelope.Data[2].ID != "working/z" {
		t.Fatalf("models = %#v", envelope.Data)
	}
}

func TestSmartRemainsVisibleInClientFirstFiftyModels(t *testing.T) {
	ids := make([]string, 60)
	for i := range ids {
		ids[i] = fmt.Sprintf("model-%02d", i)
	}
	upstream, err := provider.NewOpenAICompatibleProvider(provider.OpenAICompatibleConfig{
		ID: "ziz", BaseURL: "http://unused.example", StaticModels: ids,
	})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := api.NewOpenAIHandler([]provider.OpenAIProvider{upstream})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	var envelope struct {
		Data []provider.Model `json:"data"`
	}
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &envelope) != nil {
		t.Fatal("invalid model list", response.Code, response.Body.String())
	}
	if len(envelope.Data) != 61 || envelope.Data[0].ID != "smart" || envelope.Data[0].OwnedBy != "torii" {
		t.Fatal("routing model is missing from first page", envelope.Data)
	}
	for i, id := range ids {
		if envelope.Data[i+1].ID != "ziz/"+id {
			t.Fatal("physical names or ordering changed", envelope.Data[i+1])
		}
	}
}

func TestOpenAIHandlerSmartRoutesAndSessionDeletion(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			_, _ = io.WriteString(w, `{"data":[{"id":"model"}]}`)
			return
		}
		if r.URL.Path == "/api/show" {
			_, _ = io.WriteString(w, `{"capabilities":["completion"],"model_info":{"general.architecture":"llama","llama.context_length":8192}}`)
			return
		}
		if r.URL.Path == "/v1/chat/completions" {
			_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"smart"}}]}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer upstream.Close()
	physical, _ := provider.NewOllama("local", upstream.URL, upstream.Client())
	handler, err := api.NewOpenAIHandler([]provider.OpenAIProvider{physical})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"smart","user":"agent","messages":[{"role":"user","content":"hi"}]}`))
	request.Header.Set("X-Session-ID", "session-1")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"smart"`) {
		t.Fatalf("smart response = %d %s", response.Code, response.Body.String())
	}
	deleted := httptest.NewRecorder()
	handler.ServeHTTP(deleted, httptest.NewRequest(http.MethodDelete, "/gateway/v1/sessions/session-1", nil))
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", deleted.Code)
	}
}

func TestOpenAIHandlerRejectsStatefulResponsesFields(t *testing.T) {
	for _, parameter := range []string{"previous_response_id", "conversation"} {
		t.Run(parameter, func(t *testing.T) {
			handler, _ := api.NewOpenAIHandler([]provider.OpenAIProvider{failingProvider{}})
			response := httptest.NewRecorder()
			body := fmt.Sprintf(`{"model":"broken/model",%q:null}`, parameter)
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body)))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d: %s", response.Code, response.Body.String())
			}
			var envelope struct {
				Error struct {
					Code  string  `json:"code"`
					Param *string `json:"param"`
				} `json:"error"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || envelope.Error.Code != "unsupported_parameter" || envelope.Error.Param == nil || *envelope.Error.Param != parameter {
				t.Fatalf("response = %s, decode error = %v", response.Body.String(), err)
			}
		})
	}
}

func TestOpenAIHandlerRejectsNonBooleanStream(t *testing.T) {
	handler, _ := api.NewOpenAIHandler([]provider.OpenAIProvider{failingProvider{}})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"broken/model","stream":"yes"}`)))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid_stream") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestOpenAIHandlerFlushesStreamingEvents(t *testing.T) {
	secondEvent := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("Accept = %q", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: first\n\n")
		w.(http.Flusher).Flush()
		<-secondEvent
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer upstream.Close()
	ollama, _ := provider.NewOllama("local", upstream.URL, upstream.Client())
	handler, _ := api.NewOpenAIHandler([]provider.OpenAIProvider{ollama})
	gateway := httptest.NewServer(handler)
	defer gateway.Close()

	response, err := http.Post(gateway.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{"model":"local/a","stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	first := make([]byte, len("data: first\n\n"))
	if _, err := io.ReadFull(response.Body, first); err != nil {
		t.Fatal(err)
	}
	if string(first) != "data: first\n\n" {
		t.Fatalf("first event = %q", first)
	}
	close(secondEvent)
	rest, err := io.ReadAll(response.Body)
	if err != nil || string(rest) != "data: [DONE]\n\n" {
		t.Fatalf("remaining stream = %q, error = %v", rest, err)
	}
}

func TestOpenAIHandlerCancelsUpstreamWhenClientDisconnects(t *testing.T) {
	canceled := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: first\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(canceled)
	}))
	defer upstream.Close()
	ollama, _ := provider.NewOllama("local", upstream.URL, upstream.Client())
	handler, _ := api.NewOpenAIHandler([]provider.OpenAIProvider{ollama})
	gateway := httptest.NewServer(handler)
	defer gateway.Close()

	response, err := http.Post(gateway.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{"model":"local/a","stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	first := make([]byte, len("data: first\n\n"))
	if _, err := io.ReadFull(response.Body, first); err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("upstream context was not canceled after downstream disconnect")
	}
}

func TestOpenAIHandlerMatchesRoutingFieldsCaseSensitively(t *testing.T) {
	localCalled := false
	localServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		localCalled = true
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		const expected = `{"model":"a","Model":"cloud/b","Stream":true}`
		if string(body) != expected {
			t.Errorf("upstream body = %s, want %s", body, expected)
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	defer localServer.Close()
	cloudServer := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("case-variant Model field routed request to cloud provider")
	}))
	defer cloudServer.Close()

	local, err := provider.NewOllama("local", localServer.URL, localServer.Client())
	if err != nil {
		t.Fatal(err)
	}
	cloud, err := provider.NewOllama("cloud", cloudServer.URL, cloudServer.Client())
	if err != nil {
		t.Fatal(err)
	}
	handler, err := api.NewOpenAIHandler([]provider.OpenAIProvider{local, cloud})
	if err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	requestBody := `{"model":"local/a","Model":"cloud/b","Stream":true}`
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(requestBody)))
	if response.Code != http.StatusOK {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if !localCalled {
		t.Fatal("local provider was not called")
	}
}

func TestOpenAIHandlerRejectsOversizedRequest(t *testing.T) {
	handler, _ := api.NewOpenAIHandler(nil)
	response := httptest.NewRecorder()
	body := io.MultiReader(strings.NewReader(`{"model":"local/a","padding":"`), io.LimitReader(zeroReader{}, 32<<20), strings.NewReader(`"}`))
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body))
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusRequestEntityTooLarge, response.Body.String())
	}
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || envelope.Error.Code != "request_too_large" {
		t.Fatalf("response = %s, decode error = %v", response.Body.String(), err)
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	return len(p), nil
}

func TestOpenAIHandlerRemovesConnectionNamedHeaders(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Connection", "X-Upstream-Hop")
		w.Header().Set("X-Upstream-Hop", "private")
		w.Header().Set("Proxy-Connection", "keep-alive")
		w.Header().Set("X-End-To-End", "public")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer upstream.Close()
	ollama, _ := provider.NewOllama("local", upstream.URL, upstream.Client())
	handler, _ := api.NewOpenAIHandler([]provider.OpenAIProvider{ollama})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"local/a"}`)))
	if got := response.Header().Get("X-Upstream-Hop"); got != "" {
		t.Errorf("X-Upstream-Hop = %q", got)
	}
	if got := response.Header().Get("X-End-To-End"); got != "public" {
		t.Errorf("X-End-To-End = %q", got)
	}
	if got := response.Header().Get("Proxy-Connection"); got != "" {
		t.Errorf("Proxy-Connection = %q", got)
	}
}
