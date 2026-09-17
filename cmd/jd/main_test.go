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

func TestMetadataCommandsDoNotOpenDatabase(t *testing.T) {
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

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "version flag", args: []string{"--version"}, want: "jd dev\n"},
		{name: "version command", args: []string{"version"}, want: "jd dev\n"},
		{name: "help", args: []string{"--help"}, want: "Jump to directories quickly"},
		{name: "completion", args: []string{"completion", "zsh"}, want: "#compdef jd"},
		{name: "shell init", args: []string{"init", "zsh"}, want: "jd()"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			code := run(tt.args, bytes.NewBuffer(nil), stdout, stderr)
			if code != 0 || !strings.Contains(stdout.String(), tt.want) || stderr.Len() != 0 {
				t.Fatalf("run(%q) code=%d stdout=%q stderr=%q", tt.args, code, stdout.String(), stderr.String())
			}
		})
	}

	data, err := os.ReadFile(databasePath)
	if err != nil || string(data) != "not a sqlite database" {
		t.Fatalf("metadata command changed database: %q, %v", data, err)
	}
}

func TestStatelessCommandsIgnoreCorruptConfig(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	t.Setenv("JD_CONFIG_DIR", configDir)
	t.Setenv("JD_DATA_DIR", filepath.Join(root, "data"))
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte("not = [valid"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{{"--version"}, {"version"}, {"--help"}, {"help", "pin"}, {"completion", "zsh"}} {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		if code := run(args, bytes.NewBuffer(nil), stdout, stderr); code != 0 {
			t.Fatalf("run(%q) code=%d stdout=%q stderr=%q", args, code, stdout.String(), stderr.String())
		}
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	if code := run([]string{"init", "zsh"}, bytes.NewBuffer(nil), stdout, stderr); code != 1 {
		t.Fatalf("init with corrupt config code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
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
