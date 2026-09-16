package config

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadMissingConfigReturnsDocumentedDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := Config{
		Version:      1,
		MaxResults:   100,
		StaleDays:    30,
		TrackShellCD: true,
		Scan: ScanConfig{
			MaxDepth:       4,
			IncludeHidden:  false,
			FollowSymlinks: false,
			Ignore:         []string{".git", "node_modules", "vendor", "target", "dist", ".cache"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load() = %#v; want %#v", got, want)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.toml")
	want := Default()
	want.MaxResults = 42
	want.Scan.Ignore = append(want.Scan.Ignore, "build")
	if err := Save(path, want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip = %#v; want %#v", got, want)
	}
}

func TestPathsHonorEnvironmentOverrides(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	dataDir := filepath.Join(t.TempDir(), "data")
	t.Setenv("JD_CONFIG_DIR", configDir)
	t.Setenv("JD_DATA_DIR", dataDir)

	got, err := ResolvePaths()
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	if got.ConfigFile != filepath.Join(configDir, "config.toml") {
		t.Fatalf("ConfigFile = %q", got.ConfigFile)
	}
	if got.DatabaseFile != filepath.Join(dataDir, "state.db") {
		t.Fatalf("DatabaseFile = %q", got.DatabaseFile)
	}
}

func TestValidateRejectsInvalidRanges(t *testing.T) {
	cfg := Default()
	cfg.Scan.MaxDepth = -1
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted negative scan depth")
	}
}

func TestSetUpdatesSupportedKeysWithValidation(t *testing.T) {
	cfg := Default()
	if err := Set(&cfg, "max_results", "25"); err != nil {
		t.Fatalf("Set(max_results) error = %v", err)
	}
	if err := Set(&cfg, "scan.include_hidden", "true"); err != nil {
		t.Fatalf("Set(scan.include_hidden) error = %v", err)
	}
	if cfg.MaxResults != 25 || !cfg.Scan.IncludeHidden {
		t.Fatalf("Set() produced %#v", cfg)
	}
	if err := Set(&cfg, "scan.max_depth", "-1"); err == nil {
		t.Fatal("Set() accepted invalid scan depth")
	}
	if err := Set(&cfg, "unknown", "value"); err == nil {
		t.Fatal("Set() accepted unknown key")
	}
}
