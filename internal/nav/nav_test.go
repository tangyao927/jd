package nav

import (
	"path/filepath"
	"testing"
	"time"
)

func TestResolveLiteralAcceptsExistingDirectory(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "space dir")
	if err := mkdirAll(target); err != nil {
		t.Fatal(err)
	}

	got, ok, err := ResolveLiteral([]string{"space dir"}, root)
	if err != nil {
		t.Fatalf("ResolveLiteral returned error: %v", err)
	}
	if !ok || got != target {
		t.Fatalf("ResolveLiteral() = %q, %v; want %q, true", got, ok, target)
	}
}

func TestResolveLiteralRejectsFiles(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "notes.txt")
	if err := writeFile(file, []byte("notes")); err != nil {
		t.Fatal(err)
	}

	_, ok, err := ResolveLiteral([]string{"notes.txt"}, root)
	if err != nil {
		t.Fatalf("ResolveLiteral returned error: %v", err)
	}
	if ok {
		t.Fatal("ResolveLiteral accepted a regular file")
	}
}

func TestRankRequiresQueryTermsInPathOrder(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	candidates := []Candidate{
		{Path: "/work/api/service/src", VisitCount: 2, LastVisited: now},
		{Path: "/work/src/service/api", VisitCount: 20, LastVisited: now},
	}

	got := Rank([]string{"api", "src"}, "/work", candidates, now)
	if len(got) != 1 || got[0].Path != "/work/api/service/src" {
		t.Fatalf("Rank() = %#v; want only ordered api/src path", got)
	}
}

func TestRankOrdersByMatchKindThenPinThenFrecency(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	candidates := []Candidate{
		{Path: "/work/services/my-api", VisitCount: 100, LastVisited: now},
		{Path: "/work/api-client", Pin: "client", VisitCount: 1, LastVisited: now.Add(-8 * 24 * time.Hour)},
		{Path: "/work/archive/api", VisitCount: 2, LastVisited: now.Add(-2 * time.Hour)},
		{Path: "/work/current/api", VisitCount: 5, LastVisited: now.Add(-30 * time.Minute)},
	}

	got := Rank([]string{"api"}, "/work", candidates, now)
	want := []string{
		"/work/current/api",
		"/work/archive/api",
		"/work/api-client",
		"/work/services/my-api",
	}
	if len(got) != len(want) {
		t.Fatalf("Rank() returned %d entries; want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Path != want[i] {
			t.Fatalf("Rank()[%d] = %q; want %q", i, got[i].Path, want[i])
		}
	}
}

func TestExactPinFindIsCaseInsensitive(t *testing.T) {
	candidates := []Candidate{{Path: "/work/project", Pin: "Proj"}}
	got, ok := ExactPin("proj", candidates)
	if !ok || got.Path != "/work/project" {
		t.Fatalf("ExactPin() = %#v, %v", got, ok)
	}
}

func TestRankSupportsCharacterSubsequenceAcrossPathSegments(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	got := Rank([]string{"wapi"}, "/", []Candidate{{Path: "/work/api", VisitCount: 1, LastVisited: now}}, now)
	if len(got) != 1 || got[0].Path != "/work/api" || got[0].Kind != MatchSubsequence {
		t.Fatalf("Rank(wapi) = %#v; want /work/api subsequence match", got)
	}
}
