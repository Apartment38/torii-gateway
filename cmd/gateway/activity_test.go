package main

import (
	"bufio"
	"context"
	"github.com/Apartment38/torii-gateway/internal/activity"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestActivityAuthenticatedHistoryAndSSEReplay(t *testing.T) {
	r, _ := credentialRuntime(t)
	server := httptest.NewServer(r)
	defer server.Close()
	req, _ := http.NewRequest("GET", server.URL+"/gateway/v1/activity", nil)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatal("public activity")
	}
	ch, unsubscribe := r.activityStore.Subscribe()
	defer unsubscribe()
	r.activityStore.Emit(activity.Event{Kind: "request_start", RequestID: "test-request", KeyName: "Client", RequestedModel: "smart"})
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("event not published")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ = http.NewRequestWithContext(ctx, "GET", server.URL+"/gateway/v1/activity/events", nil)
	req.SetBasicAuth("chosen-admin", "administrator-password")
	response, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 || response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatal("SSE rejected", response.StatusCode)
	}
	scanner := bufio.NewScanner(response.Body)
	seenID := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "id: ") {
			seenID = true
		}
		if strings.Contains(line, "test-request") {
			if !seenID {
				t.Fatal("missing replay ID")
			}
			break
		}
	}
	cancel()
}
