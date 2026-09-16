package scan

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestDiscoverHonorsDepthHiddenAndIgnoreRules(t *testing.T) {
	root := t.TempDir()
	for _, relative := range []string{
		"api/src",
		"api/src/deeper",
		"node_modules/pkg",
		".hidden/project",
	} {
		if err := os.MkdirAll(filepath.Join(root, relative), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	got, err := Discover(context.Background(), root, Options{
		MaxDepth:       2,
		IncludeHidden:  false,
		FollowSymlinks: false,
		Ignore:         []string{"node_modules"},
	})
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	want := []string{root, filepath.Join(root, "api"), filepath.Join(root, "api", "src")}
	if !slices.Equal(got, want) {
		t.Fatalf("Discover() = %#v; want %#v", got, want)
	}
}

func TestDiscoverDoesNotFollowDirectorySymlinksByDefault(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(outside, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	got, err := Discover(context.Background(), root, Options{MaxDepth: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != root {
		t.Fatalf("Discover() followed or indexed symlink: %#v", got)
	}
}

func TestDiscoverAcceptsExplicitSymlinkRootWithoutFollowingNestedLinks(t *testing.T) {
	actual := t.TempDir()
	if err := os.MkdirAll(filepath.Join(actual, "project"), 0o755); err != nil {
		t.Fatal(err)
	}
	container := t.TempDir()
	root := filepath.Join(container, "work-link")
	if err := os.Symlink(actual, root); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	got, err := Discover(context.Background(), root, Options{MaxDepth: 2, FollowSymlinks: false})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{root, filepath.Join(root, "project")}
	if !slices.Equal(got, want) {
		t.Fatalf("Discover(explicit symlink root) = %#v; want %#v", got, want)
	}
}

func TestDiscoverStopsWhenContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Discover(ctx, t.TempDir(), Options{MaxDepth: 4})
	if err == nil {
		t.Fatal("Discover() ignored cancelled context")
	}
}
