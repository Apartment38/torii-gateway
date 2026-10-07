package intelligence

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	parquet "github.com/parquet-go/parquet-go"
)

type benchmarkRoundTripper func(*http.Request) (*http.Response, error)

func (f benchmarkRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type judgmentFixture struct {
	Model     string  `parquet:"model"`
	Question  string  `parquet:"question_id"`
	Category  string  `parquet:"category"`
	Task      string  `parquet:"task"`
	Score     float64 `parquet:"score"`
	Timestamp float64 `parquet:"tstamp"`
}

func judgmentFile(t *testing.T, model string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := parquet.NewGenericWriter[judgmentFixture](&b)
	if _, err := w.Write([]judgmentFixture{{Model: model, Question: "q1", Category: "coding", Task: "code", Score: .8, Timestamp: 1750000000}}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

const fixtureRevision = "0123456789abcdef0123456789abcdef01234567"

func fixtureManifest(files ...string) []byte {
	siblings := []map[string]string{}
	for _, filename := range files {
		siblings = append(siblings, map[string]string{"rfilename": filename})
	}
	data, _ := json.Marshal(map[string]any{"sha": fixtureRevision, "siblings": siblings, "cardData": map[string]any{"configs": []any{map[string]any{"config_name": "default", "data_files": []any{map[string]any{"split": "leaderboard", "path": "data/leaderboard-*"}}}}}})
	return data
}

func fixtureResponse(req *http.Request, status int, data []byte) *http.Response {
	return &http.Response{Request: req, StatusCode: status, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(data)), ContentLength: int64(len(data))}
}

func parquetSource(t *testing.T, transport benchmarkRoundTripper, key string) *huggingFace {
	t.Helper()
	source, err := newLiveBench(SourceSettings{ID: "livebench", Type: "livebench"}, key, FetchOptions{Client: &http.Client{Transport: transport}, RequestInterval: time.Nanosecond})
	if err != nil {
		t.Fatal(err)
	}
	return source.(*huggingFace)
}

func TestParquetTransportSelectionPreservesCustomRows(t *testing.T) {
	for _, item := range []struct {
		mode, url, transport string
		want                 bool
	}{
		{"livebench", "https://datasets-server.huggingface.co/rows", "", true},
		{"lmarena", "https://datasets-server.huggingface.co/rows", "auto", true},
		{"livebench", "https://custom.example/rows", "", false},
		{"lmarena", "https://datasets-server.huggingface.co/rows", "rows", false},
		{"huggingface", "https://datasets-server.huggingface.co/rows", "", false},
		{"huggingface", "https://datasets-server.huggingface.co/rows", "parquet", true},
	} {
		h := &huggingFace{remote: &remote{cfg: SourceSettings{URL: item.url, Transport: item.transport}}, mode: item.mode}
		if h.useParquet() != item.want {
			t.Fatalf("%+v", item)
		}
	}
}

func TestParquetDownloadsEverySelectedShardPinsRevisionAndMatchesRows(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	first, second := judgmentFile(t, "model-a"), judgmentFile(t, "model-b")
	var calls []string
	h := parquetSource(t, func(req *http.Request) (*http.Response, error) {
		calls = append(calls, req.URL.Path)
		if req.Header.Get("Authorization") != "Bearer optional-token" {
			t.Fatal("Hub token missing")
		}
		if strings.HasPrefix(req.URL.Path, "/api/") {
			return fixtureResponse(req, 200, fixtureManifest("data/leaderboard-00001.parquet", "data/other.parquet", "data/leaderboard-00000.parquet")), nil
		}
		if !strings.Contains(req.URL.Path, "/resolve/"+fixtureRevision+"/") {
			t.Fatal("unpinned download")
		}
		data := first
		if strings.HasSuffix(req.URL.Path, "00001.parquet") {
			data = second
		}
		return fixtureResponse(req, 200, data), nil
	}, "optional-token")
	var phases []string
	ctx := context.WithValue(context.Background(), refreshActivityKey{}, func(phase, message string) {
		phases = append(phases, phase)
		if strings.Contains(message, "optional-token") {
			t.Fatal("token exposed")
		}
	})
	result, err := h.Fetch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 3 || len(result.Observations) != 2 {
		t.Fatalf("calls=%v result=%+v", calls, result)
	}
	for i, model := range []string{"model-a", "model-b"} {
		row := map[string]any{"model": model, "question_id": "q1", "category": "coding", "task": "code", "score": .8, "tstamp": float64(1750000000)}
		expected, err := h.parseRow(h.cfg.Datasets[0], row)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(result.Observations[i], expected[0]) {
			t.Fatalf("Parquet/rows mismatch: %+v vs %+v", result.Observations[i], expected[0])
		}
	}
	if len(phases) != 5 || phases[0] != "discovering" || phases[1] != "downloading" || phases[2] != "parsing" {
		t.Fatal(phases)
	}
	files, _ := os.ReadDir(os.Getenv("TMPDIR"))
	if len(files) != 0 {
		t.Fatal("temporary downloads leaked")
	}
}

func TestParquetRedirectsStripTokenAndRejectUnsafeDestinations(t *testing.T) {
	data := judgmentFile(t, "model-a")
	cdnCalls := 0
	h := parquetSource(t, func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "us.aws.cdn.hf.co" {
			cdnCalls++
			if req.Header.Get("Authorization") != "" {
				t.Fatal("token leaked to CDN")
			}
			return fixtureResponse(req, 200, data), nil
		}
		if strings.HasPrefix(req.URL.Path, "/api/") {
			return fixtureResponse(req, 200, fixtureManifest("data/leaderboard-0.parquet")), nil
		}
		resp := fixtureResponse(req, 302, nil)
		resp.Header.Set("Location", "https://us.aws.cdn.hf.co/file?signature=private-download-url")
		return resp, nil
	}, "optional-token")
	if _, err := h.Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if cdnCalls != 1 {
		t.Fatal("CDN not downloaded")
	}
	original, _ := http.NewRequest("GET", "https://huggingface.co/file", nil)
	for _, destination := range []string{"http://us.aws.cdn.hf.co/file", "https://evil.example/file", "https://us.aws.cdn.hf.co:444/file", "https://user:pass@huggingface.co/file"} {
		req, _ := http.NewRequest("GET", destination, nil)
		if benchmarkDownloadRedirect(req, []*http.Request{original}) == nil {
			t.Fatal("accepted", destination)
		}
	}
	req, _ := http.NewRequest("GET", "https://huggingface.co/file", nil)
	if benchmarkDownloadRedirect(req, make([]*http.Request, 6)) == nil {
		t.Fatal("redirect limit ignored")
	}
}

func TestParquetFailureRetainsCacheAndDoesNotFallbackToRows(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	fail := false
	data := judgmentFile(t, "model-a")
	h := parquetSource(t, func(req *http.Request) (*http.Response, error) {
		if strings.HasPrefix(req.URL.Path, "/api/") {
			return fixtureResponse(req, 200, fixtureManifest("data/leaderboard-0.parquet", "data/leaderboard-1.parquet")), nil
		}
		if fail && strings.HasSuffix(req.URL.Path, "1.parquet") {
			return fixtureResponse(req, 404, []byte("secret-sentinel")), nil
		}
		if req.URL.Host != "huggingface.co" {
			t.Fatal("rows fallback")
		}
		return fixtureResponse(req, 200, data), nil
	}, "")
	m, _ := manager(t, []Source{h}, options())
	if err := m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := m.Snapshot().Sources[h.ID()]
	fail = true
	if err := m.Refresh(context.Background()); err == nil {
		t.Fatal("partial import succeeded")
	}
	after := m.Snapshot().Sources[h.ID()]
	if !reflect.DeepEqual(before.Observations, after.Observations) || !before.LastSuccess.Equal(after.LastSuccess) || strings.Contains(after.Error, "secret-sentinel") {
		t.Fatal("cache or redaction changed")
	}
	files, _ := os.ReadDir(os.Getenv("TMPDIR"))
	if len(files) != 0 {
		t.Fatal("temporary downloads leaked after failure")
	}
}

func TestParquetLimitsMalformedFilesAndCancellation(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	data := judgmentFile(t, "model-a")
	h := parquetSource(t, func(req *http.Request) (*http.Response, error) { return fixtureResponse(req, 200, data), nil }, "")
	if _, _, _, err := h.fetchParquetFile(context.Background(), h.client, h.cfg.Datasets[0], fixtureRevision, "file.parquet", 10, 10); err == nil {
		t.Fatal("byte limit ignored")
	}
	for _, item := range []struct {
		data  []byte
		limit int64
	}{{data, 0}, {[]byte("not parquet"), 10}} {
		if _, _, err := h.decodeParquet(context.Background(), bytes.NewReader(item.data), int64(len(item.data)), item.limit, h.cfg.Datasets[0]); err == nil {
			t.Fatal("bad dataset accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := h.decodeParquet(ctx, bytes.NewReader(data), int64(len(data)), 10, h.cfg.Datasets[0]); err == nil {
		t.Fatal("cancellation ignored")
	}
	h.client.Transport = benchmarkRoundTripper(func(req *http.Request) (*http.Response, error) {
		resp := fixtureResponse(req, 200, data)
		resp.ContentLength = -1
		return resp, nil
	})
	if _, _, _, err := h.fetchParquetFile(context.Background(), h.client, h.cfg.Datasets[0], fixtureRevision, "file.parquet", 10, 10); err == nil {
		t.Fatal("unknown-length byte limit ignored")
	}
	files, _ := os.ReadDir(os.Getenv("TMPDIR"))
	if len(files) != 0 {
		t.Fatal("temporary downloads leaked")
	}
}

func TestParquetArenaPreservesPreferenceConfidenceAndDates(t *testing.T) {
	type arenaRow struct {
		Model    string  `parquet:"model_name"`
		Rating   float64 `parquet:"rating"`
		Lower    float64 `parquet:"rating_lower"`
		Date     string  `parquet:"leaderboard_publish_date"`
		Category string  `parquet:"category"`
	}
	var b bytes.Buffer
	w := parquet.NewGenericWriter[arenaRow](&b)
	w.Write([]arenaRow{{"model-a", 1200, 1190, "2026-10-01", "coding"}})
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	source, err := newArena(SourceSettings{ID: "arena", Type: "lmarena"}, "", FetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	h := source.(*huggingFace)
	rows, _, err := h.decodeParquet(context.Background(), bytes.NewReader(b.Bytes()), int64(b.Len()), 10, h.cfg.Datasets[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Category != Preference || rows[0].EvaluatedAt.Format("2006-01-02") != "2026-10-01" || object(rows[0].Metadata["row"])["rating_lower"] != float64(1190) {
		t.Fatal(rows)
	}
}

func TestLivePublishedParquetBenchmarks(t *testing.T) {
	if os.Getenv("TORII_LIVE_BENCHMARK_SMOKE") != "1" {
		t.Skip("opt-in public network smoke test")
	}
	var mu sync.Mutex
	requests, rowsCalls := 0, 0
	client := &http.Client{Transport: benchmarkRoundTripper(func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		requests++
		if req.URL.Path == "/rows" {
			rowsCalls++
		}
		mu.Unlock()
		return http.DefaultTransport.RoundTrip(req)
	})}
	var sources []Source
	opts := options()
	opts.SourceTimeout = 2 * time.Minute
	for _, item := range []struct {
		kind    string
		factory Factory
	}{{"livebench", newLiveBench}, {"lmarena", newArena}} {
		cfg := SourceSettings{ID: item.kind, Type: item.kind, Enabled: true}
		source, err := item.factory(cfg, "", FetchOptions{Timeout: 2 * time.Minute, Client: client})
		if err != nil {
			t.Fatal(err)
		}
		sources = append(sources, source)
		opts.Sources = append(opts.Sources, cfg)
	}
	m, _ := manager(t, sources, opts)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if err := m.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	for id, source := range m.Snapshot().Sources {
		if len(source.Observations) == 0 {
			t.Fatal("empty publication", id)
		}
		t.Logf("%s: %d observations; %v", id, len(source.Observations), source.Metadata)
	}
	mu.Lock()
	defer mu.Unlock()
	if rowsCalls != 0 {
		t.Fatal("live file transport called /rows")
	}
	t.Logf("Combined refresh: %d total HTTP requests, zero /rows calls", requests)
}
