package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunStartsCLIWithIsolatedState(t *testing.T) {
	root := t.TempDir()
	t.Setenv("JD_CONFIG_DIR", filepath.Join(root, "config"))
	t.Setenv("JD_DATA_DIR", filepath.Join(root, "data"))
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	code := run([]string{"version"}, bytes.NewBuffer(nil), stdout, stderr)
	if code != 0 || stdout.String() != "jd dev\n" || stderr.Len() != 0 {
		t.Fatalf("run() code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestDoctorReportsCorruptDatabasePathWithoutDeletingIt(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	dataDir := filepath.Join(root, "data")
	t.Setenv("JD_CONFIG_DIR", configDir)
	t.Setenv("JD_DATA_DIR", dataDir)
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	databasePath := filepath.Join(dataDir, "state.db")
	if err := os.WriteFile(databasePath, []byte("not a sqlite database"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	code := run([]string{"doctor"}, bytes.NewBuffer(nil), stdout, stderr)
	if code != 1 {
		t.Fatalf("run(doctor) code=%d; want 1", code)
	}
	if !strings.Contains(stderr.String(), databasePath) || !strings.Contains(stderr.String(), "back up or move") {
		t.Fatalf("doctor error lacks recovery context: %q", stderr.String())
	}
	data, err := os.ReadFile(databasePath)
	if err != nil || string(data) != "not a sqlite database" {
		t.Fatalf("doctor changed corrupt database: %q, %v", data, err)
	}
}
