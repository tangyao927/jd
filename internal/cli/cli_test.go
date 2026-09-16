package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"jd/internal/config"
	"jd/internal/store"
)

func TestDirectPathPrintsAbsoluteTarget(t *testing.T) {
	runtime, _, stdout, _ := testRuntime(t)
	target := filepath.Join(runtime.Cwd(), "space dir")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}

	code := Execute(context.Background(), []string{"space dir"}, runtime)
	if code != ExitOK || stdout.String() != target+"\n" {
		t.Fatalf("Execute() code=%d stdout=%q; want 0 and %q", code, stdout.String(), target+"\n")
	}
}

func TestAmbiguousNonInteractiveQueryDoesNotChoose(t *testing.T) {
	runtime, db, stdout, _ := testRuntime(t)
	now := runtime.Now()
	for _, path := range []string{filepath.Join(runtime.Cwd(), "a", "api"), filepath.Join(runtime.Cwd(), "b", "api")} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := db.RecordVisit(context.Background(), path, now); err != nil {
			t.Fatal(err)
		}
	}

	code := Execute(context.Background(), []string{"api"}, runtime)
	if code != ExitAmbiguous {
		t.Fatalf("Execute() code=%d; want %d", code, ExitAmbiguous)
	}
	if stdout.Len() != 0 {
		t.Fatalf("ambiguous query printed target %q", stdout.String())
	}
}

func TestFirstChoosesHighestRankedCandidate(t *testing.T) {
	runtime, db, stdout, _ := testRuntime(t)
	now := runtime.Now()
	old := filepath.Join(runtime.Cwd(), "old", "api")
	recent := filepath.Join(runtime.Cwd(), "recent", "api")
	for _, path := range []string{old, recent} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.RecordVisit(context.Background(), old, now.Add(-10*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordVisit(context.Background(), recent, now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}

	code := Execute(context.Background(), []string{"--first", "api"}, runtime)
	if code != ExitOK || stdout.String() != recent+"\n" {
		t.Fatalf("Execute() code=%d stdout=%q; want %q", code, stdout.String(), recent+"\n")
	}
}

func TestQueryJSONHasStableVersionedShape(t *testing.T) {
	runtime, db, stdout, _ := testRuntime(t)
	target := filepath.Join(runtime.Cwd(), "api")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordVisit(context.Background(), target, runtime.Now()); err != nil {
		t.Fatal(err)
	}

	code := Execute(context.Background(), []string{"query", "api", "--format", "json"}, runtime)
	if code != ExitOK {
		t.Fatalf("Execute() code=%d", code)
	}
	var got struct {
		SchemaVersion int      `json:"schema_version"`
		Query         []string `json:"query"`
		Candidates    []struct {
			Rank    int      `json:"rank"`
			Path    string   `json:"path"`
			Match   string   `json:"match"`
			Sources []string `json:"sources"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON %q: %v", stdout.String(), err)
	}
	if got.SchemaVersion != 1 || len(got.Query) != 1 || got.Query[0] != "api" {
		t.Fatalf("query envelope = %#v", got)
	}
	if len(got.Candidates) != 1 || got.Candidates[0].Rank != 1 || got.Candidates[0].Path != target || got.Candidates[0].Match != "exact_name" {
		t.Fatalf("candidate = %#v", got.Candidates)
	}
	if len(got.Candidates[0].Sources) != 1 || got.Candidates[0].Sources[0] != "history" {
		t.Fatalf("sources = %#v", got.Candidates[0].Sources)
	}
}

func TestPinDefaultsToCurrentDirectoryAndCanBeResolved(t *testing.T) {
	runtime, _, stdout, _ := testRuntime(t)
	if code := Execute(context.Background(), []string{"pin", "home"}, runtime); code != ExitOK {
		t.Fatalf("pin code=%d", code)
	}
	stdout.Reset()
	if code := Execute(context.Background(), []string{"home"}, runtime); code != ExitOK {
		t.Fatalf("resolve pin code=%d", code)
	}
	if stdout.String() != runtime.Cwd()+"\n" {
		t.Fatalf("pin target = %q; want %q", stdout.String(), runtime.Cwd()+"\n")
	}
}

func TestExactPinClearsMissingMarkerWhenDirectoryReturns(t *testing.T) {
	runtime, db, stdout, _ := testRuntime(t)
	target := filepath.Join(runtime.Cwd(), "external")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := db.SetPin(context.Background(), "drive", target, runtime.Now()); err != nil {
		t.Fatal(err)
	}
	if err := db.MarkMissing(context.Background(), target, runtime.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if code := Execute(context.Background(), []string{"drive"}, runtime); code != ExitOK {
		t.Fatalf("resolve returned pin code=%d", code)
	}
	if stdout.String() != target+"\n" {
		t.Fatalf("target=%q; want %q", stdout.String(), target+"\n")
	}
	entries, err := db.ListDirectories(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !entries[0].MissingSince.IsZero() {
		t.Fatalf("missing marker was not cleared: %#v", entries)
	}
}

func TestRootAddScansDirectoriesForQueries(t *testing.T) {
	runtime, _, stdout, _ := testRuntime(t)
	project := filepath.Join(runtime.Cwd(), "projects", "api")
	ignored := filepath.Join(runtime.Cwd(), "projects", "node_modules", "pkg")
	for _, path := range []string{project, ignored} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	root := filepath.Join(runtime.Cwd(), "projects")
	if code := Execute(context.Background(), []string{"root", "add", root}, runtime); code != ExitOK {
		t.Fatalf("root add code=%d", code)
	}
	stdout.Reset()
	if code := Execute(context.Background(), []string{"api"}, runtime); code != ExitOK {
		t.Fatalf("resolve scanned directory code=%d", code)
	}
	if stdout.String() != project+"\n" {
		t.Fatalf("scanned target = %q; want %q", stdout.String(), project+"\n")
	}
	stdout.Reset()
	if code := Execute(context.Background(), []string{"pkg"}, runtime); code != ExitNoMatch {
		t.Fatalf("ignored directory resolve code=%d; want %d", code, ExitNoMatch)
	}
}

func TestRecordAndForgetHistory(t *testing.T) {
	runtime, db, stdout, _ := testRuntime(t)
	target := filepath.Join(runtime.Cwd(), "visited")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if code := Execute(context.Background(), []string{"_record", target}, runtime); code != ExitOK {
		t.Fatalf("_record code=%d", code)
	}
	entries, err := db.ListDirectories(context.Background(), true)
	if err != nil || len(entries) != 1 || entries[0].VisitCount != 1 {
		t.Fatalf("recorded entries = %#v, %v", entries, err)
	}
	stdout.Reset()
	if code := Execute(context.Background(), []string{"history", "forget", target}, runtime); code != ExitOK {
		t.Fatalf("history forget code=%d", code)
	}
	entries, err = db.ListDirectories(context.Background(), true)
	if err != nil || len(entries) != 0 {
		t.Fatalf("entries after forget = %#v, %v", entries, err)
	}
}

func TestConfigSetPersistsValidatedValue(t *testing.T) {
	runtime, _, stdout, _ := testRuntime(t)
	if code := Execute(context.Background(), []string{"config", "set", "max_results", "25"}, runtime); code != ExitOK {
		t.Fatalf("config set code=%d", code)
	}
	got, err := config.Load(runtime.Paths.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	if got.MaxResults != 25 {
		t.Fatalf("persisted max_results=%d; want 25", got.MaxResults)
	}
	stdout.Reset()
	if code := Execute(context.Background(), []string{"config", "get", "max_results"}, runtime); code != ExitOK {
		t.Fatalf("config get code=%d", code)
	}
	if stdout.String() != "25\n" {
		t.Fatalf("config get output=%q", stdout.String())
	}
}

func TestInitAndCompletionCommandsProduceShellIntegration(t *testing.T) {
	runtime, _, stdout, _ := testRuntime(t)
	runtime.BinaryPath = filepath.Join(runtime.Cwd(), "bin", "jd")
	if code := Execute(context.Background(), []string{"init", "zsh", "--bind", "j"}, runtime); code != ExitOK {
		t.Fatalf("init code=%d", code)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("j()")) {
		t.Fatalf("init output missing custom binding: %q", stdout.String())
	}
	stdout.Reset()
	if code := Execute(context.Background(), []string{"completion", "zsh", "--bind", "j"}, runtime); code != ExitOK {
		t.Fatalf("completion code=%d", code)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("#compdef j")) {
		t.Fatalf("completion output missing custom binding: %q", stdout.String())
	}
}

func TestDoctorReportsResolvedPaths(t *testing.T) {
	runtime, _, stdout, _ := testRuntime(t)
	if code := Execute(context.Background(), []string{"doctor"}, runtime); code != ExitOK {
		t.Fatalf("doctor code=%d", code)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(runtime.Paths.DatabaseFile)) || !bytes.Contains(stdout.Bytes(), []byte("database: ok")) {
		t.Fatalf("doctor output=%q", stdout.String())
	}
}

func testRuntime(t *testing.T) (*Runtime, *store.Store, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "data", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	runtime := &Runtime{
		Store:  db,
		Config: config.Default(),
		Paths: config.Paths{
			ConfigFile:   filepath.Join(root, "config", "config.toml"),
			DatabaseFile: db.Path(),
		},
		Stdout:      stdout,
		Stderr:      stderr,
		Stdin:       bytes.NewBuffer(nil),
		Getwd:       func() (string, error) { return root, nil },
		Now:         func() time.Time { return time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC) },
		Interactive: false,
	}
	return runtime, db, stdout, stderr
}
