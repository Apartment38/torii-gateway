package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Apartment38/torii-gateway/internal/usage"
)

func TestUsageSharedAcrossReloadAndDiscardedConfig(t *testing.T) {
	r, db := credentialRuntime(t, false)
	original := r.usageStore
	if err := original.Add("p/model", 10, 5, .25); err != nil {
		t.Fatal(err)
	}
	view := httptest.NewRecorder()
	r.getConfig(view, httptest.NewRequest(http.MethodGet, "/gateway/v1/config", nil))
	if view.Code != 200 {
		t.Fatal(view.Code, view.Body.String())
	}
	body := view.Body.String()
	w := httptest.NewRecorder()
	r.putConfig(w, configSaveRequest(t, r, body))
	if w.Code != 200 || r.usageStore != original {
		t.Fatalf("reload replaced usage store: %d %s", w.Code, w.Body.String())
	}
	if err := r.usageStore.Add("p/model", 20, 10, .5); err != nil {
		t.Fatal(err)
	}
	t.Run("discarded candidate", func(t *testing.T) {
		bad := strings.Replace(body, `"type":"openai_compatible"`, `"type":"unknown"`, 1)
		w := httptest.NewRecorder()
		r.putConfig(w, configSaveRequest(t, r, bad))
		if w.Code != 400 || r.usageStore != original {
			t.Fatal("discarded candidate mutated shared usage", w.Code)
		}
	})
	if err := r.usageStore.Add("p/model", 30, 15, .75); err != nil {
		t.Fatal("discarded handler closed shared usage", err)
	}
	if err := r.closeUsage(); err != nil {
		t.Fatal(err)
	}
	pool, err := db.StateConnection()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var requests, input, output string
	var cost float64
	if err := pool.QueryRow("SELECT requests,input_tokens,output_tokens,estimated_cost FROM usage_aggregates WHERE model_id='p/model'").Scan(&requests, &input, &output, &cost); err != nil || requests != "3" || input != "60" || output != "30" || cost != 1.5 {
		t.Fatalf("reload lost/doubled counters: %s %s %s %v %v", requests, input, output, cost, err)
	}
}

func TestHTTPDrainPrecedesFinalUsageFlush(t *testing.T) {
	db := runtimeStateDB(t)
	counters, err := usage.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	r := &handlerRuntime{usageStore: counters}
	defer r.closeUsage()
	started, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	releaseHandler := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseHandler()
	recorded := make(chan error, 1)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		recorded <- counters.Add("final/model", 10, 5, .25)
		w.WriteHeader(http.StatusNoContent)
	})}
	defer server.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- errors.Join(serveHTTP(ctx, server, listener), r.closeUsage()) }()
	response := make(chan error, 1)
	go func() {
		reply, err := http.Get("http://" + listener.Addr().String())
		if err == nil {
			reply.Body.Close()
		}
		response <- err
	}()
	<-started
	cancel()
	select {
	case <-done:
		t.Fatal("closed usage before draining request")
	case <-time.After(20 * time.Millisecond):
	}
	releaseHandler()
	if err := <-recorded; err != nil {
		t.Fatal(err)
	}
	if err := <-response; err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	pool, err := db.StateConnection()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var requests string
	if err := pool.QueryRow("SELECT requests FROM usage_aggregates WHERE model_id='final/model'").Scan(&requests); err != nil || requests != "1" {
		t.Fatal("final request not flushed", err)
	}
}
