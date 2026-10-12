package activity

import (
	"bytes"
	"fmt"
	"github.com/Apartment38/torii-gateway/internal/state"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func activityDB(t *testing.T) *state.Database {
	t.Helper()
	d, err := state.OpenDatabase(filepath.Join(t.TempDir(), "torii.db"), bytes.Repeat([]byte{5}, 32))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}
func TestActivityRetentionCapReplayAndSlowSubscriber(t *testing.T) {
	d := activityDB(t)
	s, err := Open(d, Options{RetentionDays: 7, MaxBytes: 8192})
	if err != nil {
		t.Fatal(err)
	}
	ch, unsubscribe := s.Subscribe()
	defer unsubscribe()
	for i := range 100 {
		s.Emit(Event{Kind: "request_complete", RequestID: "req", Status: i})
	}
	s.Close()
	events, gap, err := s.History(1, 100)
	if err == nil {
		t.Fatal("closed storage unexpectedly available", events, gap)
	}
	restored, err := Open(d, Options{RetentionDays: 7, MaxBytes: 8192})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	events, gap, err = restored.History(1, 100)
	if err != nil || !gap || len(events) > 30 {
		t.Fatal("cap/replay", len(events), gap, err)
	}
	var size int64
	if err = restored.db.QueryRow("SELECT bytes FROM activity_storage").Scan(&size); err != nil || size > 8192 {
		t.Fatal("storage cap exceeded", size, err)
	}
	for range ch {
	} // A stalled viewer was disconnected without blocking the writer.
	if _, err = restored.db.Exec("UPDATE activity_events SET at=0"); err != nil {
		t.Fatal(err)
	}
	if err = restored.prune(); err != nil {
		t.Fatal(err)
	}
	events, _, err = restored.History(0, 100)
	if err != nil || len(events) != 0 {
		t.Fatal("retention failed", err)
	}
}
func TestActivitySafeStructuredLogsAndBoundedQueues(t *testing.T) {
	d := activityDB(t)
	s, err := Open(d, Options{})
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	defer slog.SetDefault(previous)
	s.Emit(Event{Kind: "request_complete", KeyName: "Client", RequestedModel: "smart", SelectedModel: "p/model", Status: 200})
	s.Close()
	if !bytes.Contains(logs.Bytes(), []byte(`"kind":"request_complete"`)) || bytes.Contains(logs.Bytes(), []byte("prompt")) {
		t.Fatal("unsafe or absent metadata log", logs.String())
	}
	if cap(s.queue) != 1024 {
		t.Fatal("unbounded recording queue")
	}
}
func TestRotatingLogFileBound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "activity.log")
	writer, err := NewRotatingWriter(path, 100, 2)
	if err != nil {
		t.Fatal(err)
	}
	for range 10 {
		if _, err = writer.Write(bytes.Repeat([]byte{'x'}, 60)); err != nil {
			t.Fatal(err)
		}
	}
	writer.Close()
	for _, suffix := range []string{"", ".1", ".2"} {
		info, err := os.Stat(path + suffix)
		if err != nil || info.Size() > 100 || info.Mode().Perm() != 0600 {
			t.Fatal("rotation bound", suffix, err)
		}
	}
	if _, err = os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Fatal("too many rotated files")
	}
}

func TestDebugCaptureOptInBoundsAndLogRedaction(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			db := activityDB(t)
			s, err := Open(db, Options{PromptLogging: enabled, JevDecisionLogging: enabled})
			if err != nil {
				t.Fatal(err)
			}
			var logs bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
			defer slog.SetDefault(previous)
			ch, cancel := s.Subscribe()
			defer cancel()
			s.Emit(Event{Kind: "request_complete", Prompt: "private prompt" + strings.Repeat("x", 70<<10), JevRequest: "private decision", JevResponse: "private response"})
			e := <-ch
			if enabled {
				if len(e.Prompt) != MaxDebugBytes || !e.DebugTruncated || e.JevRequest != "private decision" || e.JevResponse != "private response" {
					t.Fatal("capture missing or unbounded")
				}
			} else if e.Prompt != "" || e.JevRequest != "" || e.JevResponse != "" {
				t.Fatal("debug capture defaulted on")
			}
			s.Close()
			if strings.Contains(logs.String(), "private") {
				t.Fatal("debug contents escaped into general logs")
			}
			restored, err := Open(db, Options{PromptLogging: enabled, JevDecisionLogging: enabled})
			if err != nil {
				t.Fatal(err)
			}
			defer restored.Close()
			rows, _, err := restored.History(0, 100)
			if err != nil || len(rows) != 1 || rows[0].Prompt != e.Prompt {
				t.Fatal("debug capture not persistent", err)
			}
		})
	}
}
func TestConfigureImmediatelyPrunesAndNotifiesViewers(t *testing.T) {
	s, err := Open(activityDB(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ch, cancel := s.Subscribe()
	defer cancel()
	s.Emit(Event{Kind: "request_complete", Prompt: "private prompt"})
	<-ch
	if _, err = s.db.Exec("UPDATE activity_events SET at=?", time.Now().Add(-48*time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	s.Configure(Options{RetentionDays: 1})
	rows, _, err := s.History(0, 100)
	if err != nil || len(rows) != 0 {
		t.Fatal("retention not applied immediately", err)
	}
	select {
	case e := <-ch:
		if e.Kind != "history_reset" {
			t.Fatal("missing cleanup notification", e)
		}
	case <-time.After(time.Second):
		t.Fatal("cleanup not sent")
	}
	s.Configure(Options{PromptLogging: true})
	s.Emit(Event{Kind: "request_complete", Prompt: strings.Repeat("x", 40<<10)})
	<-ch
	s.Configure(Options{MaxBytes: 8192})
	rows, _, err = s.History(0, 100)
	if err != nil || len(rows) != 0 {
		t.Fatal("size cap not applied immediately", err)
	}
}
