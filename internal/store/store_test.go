package store

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestRecordVisitAccumulatesWithoutLostUpdates(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	const writers = 20
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := db.RecordVisit(ctx, "/work/api", time.Unix(1_000, 0)); err != nil {
				t.Errorf("RecordVisit() error = %v", err)
			}
		}()
	}
	wg.Wait()

	entries, err := db.ListDirectories(ctx, true)
	if err != nil {
		t.Fatalf("ListDirectories() error = %v", err)
	}
	if len(entries) != 1 || entries[0].VisitCount != writers {
		t.Fatalf("entries = %#v; want one entry with %d visits", entries, writers)
	}
}

func TestPinLifecycleIsCaseInsensitive(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := db.SetPin(ctx, "Proj", "/work/project", time.Unix(2_000, 0)); err != nil {
		t.Fatalf("SetPin() error = %v", err)
	}
	pins, err := db.ListPins(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pins) != 1 || pins[0].Name != "Proj" || pins[0].Path != "/work/project" {
		t.Fatalf("pins = %#v", pins)
	}
	if err := db.DeletePin(ctx, "proj"); err != nil {
		t.Fatalf("DeletePin() error = %v", err)
	}
	pins, err = db.ListPins(ctx)
	if err != nil || len(pins) != 0 {
		t.Fatalf("pins after delete = %#v, %v", pins, err)
	}
}

func TestCleanStaleKeepsPinnedDirectory(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)
	if err := db.RecordVisit(ctx, "/gone/unpinned", now.Add(-60*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordVisit(ctx, "/gone/pinned", now.Add(-60*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := db.SetPin(ctx, "keep", "/gone/pinned", now); err != nil {
		t.Fatal(err)
	}
	if err := db.MarkMissing(ctx, "/gone/unpinned", now.Add(-31*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := db.MarkMissing(ctx, "/gone/pinned", now.Add(-31*24*time.Hour)); err != nil {
		t.Fatal(err)
	}

	removed, err := db.CleanStale(ctx, now, 30)
	if err != nil {
		t.Fatalf("CleanStale() error = %v", err)
	}
	if removed != 1 {
		t.Fatalf("CleanStale() removed %d; want 1", removed)
	}
	entries, err := db.ListDirectories(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Path != "/gone/pinned" {
		t.Fatalf("remaining entries = %#v", entries)
	}
}

func TestReplacingAndRemovingRootKeepsVisitedDirectory(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	now := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)
	root := Root{Path: "/work", MaxDepth: 4}
	if err := db.AddRoot(ctx, root); err != nil {
		t.Fatalf("AddRoot() error = %v", err)
	}
	if err := db.ReplaceRootEntries(ctx, root.Path, []string{"/work/api", "/work/web"}, now); err != nil {
		t.Fatalf("ReplaceRootEntries() error = %v", err)
	}
	if err := db.RecordVisit(ctx, "/work/api", now); err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceRootEntries(ctx, root.Path, []string{"/work/web"}, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	entries, err := db.ListDirectories(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	var api Directory
	for _, entry := range entries {
		if entry.Path == "/work/api" {
			api = entry
		}
	}
	if !api.Visited || api.Scanned {
		t.Fatalf("visited removed scan entry = %#v; want visited=true scanned=false", api)
	}

	if err := db.RemoveRoot(ctx, root.Path); err != nil {
		t.Fatalf("RemoveRoot() error = %v", err)
	}
	entries, err = db.ListDirectories(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Path != "/work/api" {
		t.Fatalf("entries after root removal = %#v; want only visited api", entries)
	}
}

func TestListRootsReturnsSavedScanOptions(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	want := Root{Path: "/work", MaxDepth: 7, IncludeHidden: true, FollowSymlinks: false}
	if err := db.AddRoot(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err := db.ListRoots(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Path != want.Path || got[0].MaxDepth != 7 || !got[0].IncludeHidden {
		t.Fatalf("ListRoots() = %#v; want %#v", got, want)
	}
}
