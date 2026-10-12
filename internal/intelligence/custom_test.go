package intelligence

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestCustomCSVValidation(t *testing.T) {
	now := time.Now().UTC()
	rows, err := ParseCSV(strings.NewReader(CSVTemplate), now)
	if err != nil || len(rows) != 1 || rows[0].Normalized == nil || *rows[0].Normalized != .85 {
		t.Fatalf("template: %+v %v", rows, err)
	}
	for _, input := range []string{
		strings.Replace(CSVTemplate, ",0.85,", ",1.85,", 1),
		strings.Replace(CSVTemplate, ",85,", ",NaN,", 1),
		strings.Replace(CSVTemplate, ",coding,", ",made-up,", 1),
		strings.Replace(CSVTemplate, "2026-10-01T00:00:00Z", "yesterday", 1),
		strings.Split(CSVTemplate, "\n")[0] + "\n",
		CSVTemplate + strings.Split(CSVTemplate, "\n")[1] + "\n",
	} {
		if _, err := ParseCSV(strings.NewReader(input), now); err == nil {
			t.Fatalf("invalid upload accepted: %s", input)
		}
	}
	rows, err = ParseCSV(strings.NewReader(strings.Replace(CSVTemplate, ",0.85,", ",,", 1)), now)
	if err != nil || rows[0].Normalized != nil {
		t.Fatalf("unknown score changed: %+v %v", rows, err)
	}
}
func TestCustomImportEditAndReload(t *testing.T) {
	m, store := manager(t, nil, options())
	ctx := context.Background()
	at := time.Now().UTC()
	o := observation(CustomSource, "custom-code", Coding, "local/model-a", .8, at)
	if err := m.Import(ctx, []Observation{o}); err != nil {
		t.Fatal(err)
	}
	snapshot := m.Snapshot()
	source := snapshot.Sources[CustomSource]
	if len(snapshot.Models) == 0 || !source.Enabled {
		t.Fatal("custom data did not affect matching")
	}
	revision := DatasetRevision(source.Observations)
	o.Score = .95
	o.Normalized = &o.Score
	if err := m.Edit(ctx, CustomSource, 0, revision, o); err != nil {
		t.Fatal(err)
	}
	if err := m.Edit(ctx, CustomSource, 0, revision, o); err != ErrEditConflict {
		t.Fatalf("stale edit accepted: %v", err)
	}
	loaded, err := NewManager(store, nil, modelCatalog(), options())
	if err != nil {
		t.Fatal(err)
	}
	got := loaded.Snapshot()
	if !got.Sources[CustomSource].Enabled || got.Sources[CustomSource].Observations[0].Score != .95 {
		t.Fatal("custom edit lost on restart")
	}
	loaded.ReloadCache()
	if !loaded.Snapshot().Sources[CustomSource].Enabled {
		t.Fatal("custom source disabled on configuration reload")
	}
	if err := loaded.Import(ctx, []Observation{o}); err != nil {
		t.Fatal(err)
	}
	if len(loaded.Snapshot().Sources[CustomSource].Observations) != 1 {
		t.Fatal("repeat upload duplicated row")
	}
	if err := loaded.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if len(loaded.Snapshot().Sources[CustomSource].Observations) != 1 {
		t.Fatal("refresh lost custom data")
	}
}
