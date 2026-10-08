package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Apartment38/torii-gateway/internal/activity"
	"net/http"
	"strconv"
	"time"
)

func (r *handlerRuntime) activityAPI(w http.ResponseWriter, req *http.Request) {
	if r.activityStore == nil {
		writeConfigError(w, 503, errors.New("activity unavailable"))
		return
	}
	limit := 100
	if raw := req.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 1000 {
			writeConfigError(w, 400, errors.New("limit must be 1-1000"))
			return
		}
		limit = n
	}
	cursor := req.URL.Query().Get("after_id")
	if req.URL.Path == "/gateway/v1/activity/events" && req.Header.Get("Last-Event-ID") != "" {
		cursor = req.Header.Get("Last-Event-ID")
	}
	after := int64(0)
	if cursor != "" {
		n, err := strconv.ParseInt(cursor, 10, 64)
		if err != nil || n < 0 {
			writeConfigError(w, 400, errors.New("invalid event ID"))
			return
		}
		after = n
	}
	if req.URL.Path != "/gateway/v1/activity/events" {
		events, gap, err := r.activityStore.History(after, limit)
		if err != nil {
			writeConfigError(w, 503, errors.New("activity history unavailable"))
			return
		}
		writeJSON(w, 200, map[string]any{"events": events, "gap": gap})
		return
	}
	subscriber, unsubscribe := r.activityStore.Subscribe()
	defer unsubscribe()
	events, gap, err := r.activityStore.History(after, 1000)
	if err != nil {
		writeConfigError(w, 503, errors.New("activity history unavailable"))
		return
	}
	controller := http.NewResponseController(w)
	defer controller.SetWriteDeadline(time.Time{})
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	send := func(event activity.Event) error {
		controller.SetWriteDeadline(time.Now().Add(5 * time.Second))
		data, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if event.ID > 0 {
			if _, err = fmt.Fprintf(w, "id: %d\n", event.ID); err != nil {
				return err
			}
		}
		kind := "activity"
		if event.Kind == "gap" {
			kind = "gap"
		}
		if _, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, data); err != nil {
			return err
		}
		return controller.Flush()
	}
	if gap {
		after = 0
		if send(activity.Event{Kind: "gap", GapCount: 1}) != nil {
			return
		}
	}
	for _, event := range events {
		if event.ID > after {
			if send(event) != nil {
				return
			}
			after = event.ID
		}
	}
	if controller.Flush() != nil {
		return
	}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	var stopped <-chan struct{}
	if r.background != nil {
		stopped = r.background.Done()
	}
	for {
		select {
		case <-req.Context().Done():
			return
		case <-stopped:
			return
		case event, ok := <-subscriber:
			if !ok {
				send(activity.Event{Kind: "gap", GapCount: 1})
				return
			}
			if event.ID == 0 || event.ID > after {
				if send(event) != nil {
					return
				}
				after = max(after, event.ID)
			}
		case <-ticker.C:
			if cookie, err := req.Cookie(adminCookie); err == nil {
				if _, err = r.credentials.AdminSession(cookie.Value); err != nil {
					return
				}
			}
			controller.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if _, err = fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			if controller.Flush() != nil {
				return
			}
		}
	}
}
