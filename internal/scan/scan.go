package scan

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Options struct {
	MaxDepth       int
	IncludeHidden  bool
	FollowSymlinks bool
	Ignore         []string
}

func Discover(ctx context.Context, root string, options Options) ([]string, error) {
	root, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("inspect scan root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("scan root is not a directory: %s", root)
	}
	ignored := make(map[string]struct{}, len(options.Ignore))
	for _, name := range options.Ignore {
		ignored[strings.ToLower(name)] = struct{}{}
	}
	seen := make(map[string]struct{})
	var result []string
	var visit func(string, int, bool) error
	visit = func(path string, depth int, isRoot bool) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		base := filepath.Base(path)
		if !isRoot {
			if _, skip := ignored[strings.ToLower(base)]; skip {
				return nil
			}
			if !options.IncludeHidden && strings.HasPrefix(base, ".") {
				return nil
			}
		}
		lstat, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if lstat.Mode()&os.ModeSymlink != 0 {
			if !options.FollowSymlinks && !isRoot {
				return nil
			}
			lstat, err = os.Stat(path)
			if err != nil || !lstat.IsDir() {
				return err
			}
		}
		if !lstat.IsDir() {
			return nil
		}
		realPath, err := filepath.EvalSymlinks(path)
		if err != nil {
			return err
		}
		realPath, err = filepath.Abs(realPath)
		if err != nil {
			return err
		}
		if _, visited := seen[realPath]; visited {
			return nil
		}
		seen[realPath] = struct{}{}
		result = append(result, path)
		if depth >= options.MaxDepth {
			return nil
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := visit(filepath.Join(path, entry.Name()), depth+1, false); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(root, 0, true); err != nil {
		return nil, err
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i]) < strings.ToLower(result[j])
	})
	return result, nil
}
