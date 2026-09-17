package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tangyao927/jd/internal/config"
	"github.com/tangyao927/jd/internal/nav"
	"github.com/tangyao927/jd/internal/scan"
	jdshell "github.com/tangyao927/jd/internal/shell"
	"github.com/tangyao927/jd/internal/store"
	"github.com/tangyao927/jd/internal/ui"
)

const (
	ExitOK        = 0
	ExitRuntime   = 1
	ExitUsage     = 2
	ExitNoMatch   = 3
	ExitAmbiguous = 4
	ExitCancelled = 130
)

type Runtime struct {
	Store       store.Database
	Config      config.Config
	Paths       config.Paths
	Stdin       io.Reader
	Stdout      io.Writer
	Stderr      io.Writer
	Getwd       func() (string, error)
	Now         func() time.Time
	Interactive bool
	Selector    func([]nav.Match) (string, error)
	BinaryPath  string
	Version     string
}

func (r *Runtime) Cwd() string {
	cwd, _ := r.Getwd()
	return cwd
}

type exitError struct {
	code int
	msg  string
}

func (e *exitError) Error() string { return e.msg }

func fail(code int, format string, args ...any) error {
	return &exitError{code: code, msg: fmt.Sprintf(format, args...)}
}

func Execute(ctx context.Context, args []string, runtime *Runtime) int {
	root := newRootCommand(runtime)
	root.SetArgs(args)
	root.SetIn(runtime.Stdin)
	root.SetOut(runtime.Stdout)
	root.SetErr(runtime.Stderr)
	root.SilenceErrors = true
	root.SilenceUsage = true
	err := root.ExecuteContext(ctx)
	if err == nil {
		return ExitOK
	}
	var coded *exitError
	if errors.As(err, &coded) {
		if coded.msg != "" {
			fmt.Fprintf(runtime.Stderr, "jd: %s\n", coded.msg)
		}
		return coded.code
	}
	fmt.Fprintf(runtime.Stderr, "jd: %s\n", err)
	return ExitUsage
}

// RequiresConfig reports whether executing args needs the user's configuration.
// Help, version, and completion must remain available when configuration is broken.
func RequiresConfig(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			return true
		}
		if arg == "-h" || arg == "--help" {
			return false
		}
	}
	for _, arg := range args {
		switch arg {
		case "--first":
			continue
		case "--version":
			return false
		case "help", "version", "completion":
			return false
		}
		if strings.HasPrefix(arg, "-") {
			return false
		}
		return true
	}
	return true
}

func newRootCommand(runtime *Runtime) *cobra.Command {
	var first bool
	version := runtime.Version
	if version == "" {
		version = "dev"
	}
	root := &cobra.Command{
		Use:   "jd [query...]",
		Short: "Jump to directories quickly",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return resolve(cmd.Context(), runtime, args, first)
		},
	}
	root.Version = version
	root.SetVersionTemplate("jd {{.Version}}\n")
	root.Flags().BoolVar(&first, "first", false, "choose the highest ranked result without prompting")
	root.AddCommand(
		newJumpCommand(runtime), newQueryCommand(runtime), newPinCommand(runtime), newUnpinCommand(runtime), newPinsCommand(runtime),
		newRootManagementCommand(runtime), newScanCommand(runtime), newHistoryCommand(runtime), newConfigCommand(runtime), newRecordCommand(runtime),
		newInitCommand(runtime), newCompletionCommand(runtime), newDoctorCommand(runtime), newVersionCommand(runtime),
	)
	return root
}

func newJumpCommand(runtime *Runtime) *cobra.Command {
	var first bool
	command := &cobra.Command{
		Use:   "jump [query...]",
		Short: "Treat all arguments as a directory query",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return resolve(cmd.Context(), runtime, args, first)
		},
	}
	command.Flags().BoolVar(&first, "first", false, "choose the highest ranked result without prompting")
	return command
}

func resolve(ctx context.Context, runtime *Runtime, args []string, first bool) error {
	cwd, err := runtime.Getwd()
	if err != nil {
		return fail(ExitRuntime, "get current directory: %v", err)
	}
	if path, ok, err := nav.ResolveLiteral(args, cwd); err != nil {
		return fail(ExitRuntime, "%v", err)
	} else if ok {
		fmt.Fprintln(runtime.Stdout, path)
		return nil
	}
	if len(args) == 1 {
		pins, err := runtime.Store.ListPins(ctx)
		if err != nil {
			return fail(ExitRuntime, "read pins: %v", err)
		}
		for _, pin := range pins {
			if !strings.EqualFold(pin.Name, args[0]) {
				continue
			}
			info, statErr := os.Stat(pin.Path)
			if statErr == nil && info.IsDir() {
				if err := runtime.Store.MarkPresent(ctx, pin.Path, runtime.Now()); err != nil {
					return fail(ExitRuntime, "restore pin state: %v", err)
				}
				fmt.Fprintln(runtime.Stdout, pin.Path)
				return nil
			}
			if os.IsNotExist(statErr) {
				_ = runtime.Store.MarkMissing(ctx, pin.Path, runtime.Now())
			}
		}
	}
	matches, err := findMatches(ctx, runtime, args)
	if err != nil {
		return err
	}
	switch len(matches) {
	case 0:
		return fail(ExitNoMatch, "no matching directory")
	case 1:
		fmt.Fprintln(runtime.Stdout, matches[0].Path)
		return nil
	}
	if first {
		fmt.Fprintln(runtime.Stdout, matches[0].Path)
		return nil
	}
	if !runtime.Interactive {
		return fail(ExitAmbiguous, "%d matching directories; use --first or an interactive terminal", len(matches))
	}
	selector := runtime.Selector
	if selector == nil {
		selector = func(matches []nav.Match) (string, error) {
			return ui.Select(matches, runtime.Stdin, runtime.Stderr)
		}
	}
	target, err := selector(matches)
	if errors.Is(err, ui.ErrCancelled) {
		return fail(ExitCancelled, "")
	}
	if err != nil {
		return fail(ExitRuntime, "select directory: %v", err)
	}
	fmt.Fprintln(runtime.Stdout, target)
	return nil
}

func findMatches(ctx context.Context, runtime *Runtime, args []string) ([]nav.Match, error) {
	entries, err := runtime.Store.ListDirectories(ctx, false)
	if err != nil {
		return nil, fail(ExitRuntime, "read directories: %v", err)
	}
	candidates := make([]nav.Candidate, 0, len(entries))
	for _, entry := range entries {
		info, statErr := os.Stat(entry.Path)
		if os.IsNotExist(statErr) {
			_ = runtime.Store.MarkMissing(ctx, entry.Path, runtime.Now())
			continue
		}
		if statErr != nil || !info.IsDir() {
			continue
		}
		sources := make([]string, 0, 3)
		if entry.Pin != "" {
			sources = append(sources, "pin")
		}
		if entry.Visited {
			sources = append(sources, "history")
		}
		if entry.Scanned {
			sources = append(sources, "scan")
		}
		candidates = append(candidates, nav.Candidate{
			Path: entry.Path, Pin: entry.Pin, Sources: sources,
			VisitCount: entry.VisitCount, LastVisited: entry.LastVisited,
		})
	}
	cwd, err := runtime.Getwd()
	if err != nil {
		return nil, fail(ExitRuntime, "get current directory: %v", err)
	}
	matches := nav.Rank(args, cwd, candidates, runtime.Now())
	if len(matches) > runtime.Config.MaxResults {
		matches = matches[:runtime.Config.MaxResults]
	}
	return matches, nil
}

type queryEnvelope struct {
	SchemaVersion int              `json:"schema_version"`
	Query         []string         `json:"query"`
	Candidates    []queryCandidate `json:"candidates"`
}

type queryCandidate struct {
	Rank    int      `json:"rank"`
	Path    string   `json:"path"`
	Display string   `json:"display"`
	Match   string   `json:"match"`
	Sources []string `json:"sources"`
}

func newQueryCommand(runtime *Runtime) *cobra.Command {
	format := "table"
	command := &cobra.Command{
		Use:   "query [terms...]",
		Short: "Query candidates without changing directory",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			matches, err := findMatches(cmd.Context(), runtime, args)
			if err != nil {
				return err
			}
			switch format {
			case "json":
				envelope := queryEnvelope{SchemaVersion: 1, Query: args, Candidates: make([]queryCandidate, 0, len(matches))}
				for i, match := range matches {
					envelope.Candidates = append(envelope.Candidates, queryCandidate{
						Rank: i + 1, Path: match.Path, Display: displayPath(match.Path),
						Match: string(match.Kind), Sources: match.Sources,
					})
				}
				encoder := json.NewEncoder(runtime.Stdout)
				encoder.SetIndent("", "  ")
				return encoder.Encode(envelope)
			case "table":
				for i, match := range matches {
					fmt.Fprintf(runtime.Stdout, "%d\t%s\t%s\n", i+1, string(match.Kind), displayPath(match.Path))
				}
				return nil
			default:
				return fail(ExitUsage, "unsupported format %q", format)
			}
		},
	}
	command.Flags().StringVar(&format, "format", "table", "output format: table or json")
	return command
}

func newPinCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "pin <name> [path]",
		Short: "Pin a directory under a memorable name",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(args[0]) == "" {
				return fail(ExitUsage, "pin name cannot be empty")
			}
			cwd, err := runtime.Getwd()
			if err != nil {
				return fail(ExitRuntime, "%v", err)
			}
			path := cwd
			if len(args) == 2 {
				path, _, err = nav.ResolveLiteral([]string{args[1]}, cwd)
				if err != nil {
					return fail(ExitRuntime, "%v", err)
				}
				if path == "" {
					return fail(ExitUsage, "pin target is not an existing directory")
				}
			}
			if err := runtime.Store.SetPin(cmd.Context(), args[0], path, runtime.Now()); err != nil {
				return fail(ExitRuntime, "%v", err)
			}
			fmt.Fprintf(runtime.Stdout, "pinned %s -> %s\n", args[0], path)
			return nil
		},
	}
}

func newUnpinCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use: "unpin <name>", Short: "Remove a directory pin", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := runtime.Store.DeletePin(cmd.Context(), args[0]); err != nil {
				return fail(ExitRuntime, "%v", err)
			}
			return nil
		},
	}
}

func newPinsCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use: "pins", Short: "List directory pins", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pins, err := runtime.Store.ListPins(cmd.Context())
			if err != nil {
				return fail(ExitRuntime, "%v", err)
			}
			for _, pin := range pins {
				fmt.Fprintf(runtime.Stdout, "%s\t%s\n", pin.Name, pin.Path)
			}
			return nil
		},
	}
}

func displayPath(path string) string {
	home, err := os.UserHomeDir()
	if err == nil && (path == home || strings.HasPrefix(path, home+string(filepath.Separator))) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

func newRootManagementCommand(runtime *Runtime) *cobra.Command {
	group := &cobra.Command{Use: "root", Short: "Manage directory scan roots"}
	var maxDepth int
	var includeHidden bool
	var followSymlinks bool
	add := &cobra.Command{
		Use: "add <path>", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := runtime.Getwd()
			if err != nil {
				return fail(ExitRuntime, "%v", err)
			}
			path, ok, err := nav.ResolveLiteral(args, cwd)
			if err != nil {
				return fail(ExitRuntime, "%v", err)
			}
			if !ok {
				return fail(ExitUsage, "scan root is not an existing directory")
			}
			root := store.Root{Path: path, MaxDepth: maxDepth, IncludeHidden: includeHidden, FollowSymlinks: followSymlinks}
			paths, err := discoverRoot(cmd.Context(), runtime, root)
			if err != nil {
				return err
			}
			if err := runtime.Store.AddRoot(cmd.Context(), root); err != nil {
				return fail(ExitRuntime, "%v", err)
			}
			if err := runtime.Store.ReplaceRootEntries(cmd.Context(), root.Path, paths, runtime.Now()); err != nil {
				return fail(ExitRuntime, "%v", err)
			}
			fmt.Fprintf(runtime.Stdout, "indexed %d directories under %s\n", len(paths), root.Path)
			return nil
		},
	}
	add.Flags().IntVar(&maxDepth, "max-depth", runtime.Config.Scan.MaxDepth, "maximum directory depth")
	add.Flags().BoolVar(&includeHidden, "include-hidden", runtime.Config.Scan.IncludeHidden, "include hidden directories")
	add.Flags().BoolVar(&followSymlinks, "follow-symlinks", runtime.Config.Scan.FollowSymlinks, "follow directory symlinks")
	remove := &cobra.Command{
		Use: "remove <path>", Aliases: []string{"rm"}, Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := cleanPath(args[0], runtime.Cwd())
			if err != nil {
				return fail(ExitUsage, "%v", err)
			}
			if err := runtime.Store.RemoveRoot(cmd.Context(), path); err != nil {
				return fail(ExitRuntime, "%v", err)
			}
			return nil
		},
	}
	list := &cobra.Command{
		Use: "list", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			roots, err := runtime.Store.ListRoots(cmd.Context())
			if err != nil {
				return fail(ExitRuntime, "%v", err)
			}
			for _, root := range roots {
				fmt.Fprintf(runtime.Stdout, "%s\tdepth=%d\thidden=%t\tsymlinks=%t\n", root.Path, root.MaxDepth, root.IncludeHidden, root.FollowSymlinks)
			}
			return nil
		},
	}
	group.AddCommand(add, remove, list)
	return group
}

func newScanCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use: "scan", Args: cobra.NoArgs, Short: "Refresh all configured scan roots",
		RunE: func(cmd *cobra.Command, _ []string) error {
			roots, err := runtime.Store.ListRoots(cmd.Context())
			if err != nil {
				return fail(ExitRuntime, "%v", err)
			}
			total := 0
			for _, root := range roots {
				paths, err := discoverRoot(cmd.Context(), runtime, root)
				if err != nil {
					return err
				}
				if err := runtime.Store.ReplaceRootEntries(cmd.Context(), root.Path, paths, runtime.Now()); err != nil {
					return fail(ExitRuntime, "%v", err)
				}
				total += len(paths)
			}
			fmt.Fprintf(runtime.Stdout, "indexed %d directories across %d roots\n", total, len(roots))
			return nil
		},
	}
}

func discoverRoot(ctx context.Context, runtime *Runtime, root store.Root) ([]string, error) {
	paths, err := scan.Discover(ctx, root.Path, scan.Options{
		MaxDepth: root.MaxDepth, IncludeHidden: root.IncludeHidden,
		FollowSymlinks: root.FollowSymlinks, Ignore: runtime.Config.Scan.Ignore,
	})
	if err != nil {
		return nil, fail(ExitRuntime, "scan %s: %v", root.Path, err)
	}
	return paths, nil
}

func newRecordCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use: "_record <path>", Hidden: true, Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, ok, err := nav.ResolveLiteral(args, runtime.Cwd())
			if err != nil {
				return fail(ExitRuntime, "%v", err)
			}
			if !ok {
				return fail(ExitUsage, "record target is not an existing directory")
			}
			if err := runtime.Store.RecordVisit(cmd.Context(), path, runtime.Now()); err != nil {
				return fail(ExitRuntime, "%v", err)
			}
			return nil
		},
	}
}

func newHistoryCommand(runtime *Runtime) *cobra.Command {
	group := &cobra.Command{Use: "history", Short: "Inspect and clean visit history"}
	list := &cobra.Command{
		Use: "list", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			entries, err := runtime.Store.ListDirectories(cmd.Context(), true)
			if err != nil {
				return fail(ExitRuntime, "%v", err)
			}
			for _, entry := range entries {
				if entry.Visited {
					fmt.Fprintf(runtime.Stdout, "%d\t%s\n", entry.VisitCount, entry.Path)
				}
			}
			return nil
		},
	}
	forget := &cobra.Command{
		Use: "forget [path]", Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := runtime.Cwd()
			var err error
			if len(args) == 1 {
				path, err = cleanPath(args[0], runtime.Cwd())
				if err != nil {
					return fail(ExitUsage, "%v", err)
				}
			}
			if err := runtime.Store.ForgetVisit(cmd.Context(), path); err != nil {
				return fail(ExitRuntime, "%v", err)
			}
			return nil
		},
	}
	clean := &cobra.Command{
		Use: "clean", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			entries, err := runtime.Store.ListDirectories(cmd.Context(), true)
			if err != nil {
				return fail(ExitRuntime, "%v", err)
			}
			for _, entry := range entries {
				if info, statErr := os.Stat(entry.Path); os.IsNotExist(statErr) || (statErr == nil && !info.IsDir()) {
					_ = runtime.Store.MarkMissing(cmd.Context(), entry.Path, runtime.Now())
				} else if statErr == nil {
					_ = runtime.Store.MarkPresent(cmd.Context(), entry.Path, runtime.Now())
				}
			}
			removed, err := runtime.Store.CleanStale(cmd.Context(), runtime.Now(), runtime.Config.StaleDays)
			if err != nil {
				return fail(ExitRuntime, "%v", err)
			}
			fmt.Fprintf(runtime.Stdout, "removed %d stale directories\n", removed)
			return nil
		},
	}
	group.AddCommand(list, forget, clean)
	return group
}

func newConfigCommand(runtime *Runtime) *cobra.Command {
	group := &cobra.Command{Use: "config", Short: "Inspect and update configuration"}
	path := &cobra.Command{Use: "path", Args: cobra.NoArgs, Run: func(_ *cobra.Command, _ []string) { fmt.Fprintln(runtime.Stdout, runtime.Paths.ConfigFile) }}
	show := &cobra.Command{
		Use: "show", Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := config.Encode(runtime.Stdout, runtime.Config); err != nil {
				return fail(ExitRuntime, "%v", err)
			}
			return nil
		},
	}
	get := &cobra.Command{
		Use: "get <key>", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			value, err := config.Get(runtime.Config, args[0])
			if err != nil {
				return fail(ExitUsage, "%v", err)
			}
			fmt.Fprintln(runtime.Stdout, value)
			return nil
		},
	}
	set := &cobra.Command{
		Use: "set <key> <value>", Args: cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			updated := runtime.Config
			if err := config.Set(&updated, args[0], args[1]); err != nil {
				return fail(ExitUsage, "%v", err)
			}
			if err := config.Save(runtime.Paths.ConfigFile, updated); err != nil {
				return fail(ExitRuntime, "%v", err)
			}
			runtime.Config = updated
			return nil
		},
	}
	group.AddCommand(path, show, get, set)
	return group
}

func cleanPath(input, cwd string) (string, error) {
	if input == "~" || strings.HasPrefix(input, "~/") || strings.HasPrefix(input, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		input = filepath.Join(home, strings.TrimLeft(input[1:], `/\`))
	}
	if !filepath.IsAbs(input) {
		input = filepath.Join(cwd, input)
	}
	return filepath.Abs(filepath.Clean(input))
}

func newInitCommand(runtime *Runtime) *cobra.Command {
	bind := "jd"
	command := &cobra.Command{
		Use: "init <zsh|bash|fish|powershell>", Short: "Generate shell integration code", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			binary := runtime.BinaryPath
			if binary == "" {
				var err error
				binary, err = os.Executable()
				if err != nil {
					return fail(ExitRuntime, "resolve executable: %v", err)
				}
			}
			script, err := jdshell.Init(jdshell.Options{
				Shell: args[0], Binary: binary, Bind: bind, TrackShellCD: runtime.Config.TrackShellCD,
			})
			if err != nil {
				return fail(ExitUsage, "%v", err)
			}
			fmt.Fprint(runtime.Stdout, script)
			return nil
		},
	}
	command.Flags().StringVar(&bind, "bind", "jd", "shell function name")
	return command
}

func newCompletionCommand(runtime *Runtime) *cobra.Command {
	bind := "jd"
	command := &cobra.Command{
		Use: "completion <zsh|bash|fish|powershell>", Short: "Generate shell completion code", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := jdshell.ValidateBinding(bind); err != nil {
				return fail(ExitUsage, "%v", err)
			}
			root := cmd.Root()
			originalUse := root.Use
			root.Use = bind + " [query...]"
			defer func() { root.Use = originalUse }()
			switch strings.ToLower(args[0]) {
			case "zsh":
				return root.GenZshCompletion(runtime.Stdout)
			case "bash":
				return root.GenBashCompletion(runtime.Stdout)
			case "fish":
				return root.GenFishCompletion(runtime.Stdout, true)
			case "powershell", "pwsh":
				return root.GenPowerShellCompletionWithDesc(runtime.Stdout)
			default:
				return fail(ExitUsage, "unsupported shell %q", args[0])
			}
		},
	}
	command.Flags().StringVar(&bind, "bind", "jd", "shell function name")
	return command
}

func newDoctorCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use: "doctor", Short: "Check configuration, storage, and shell support", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := runtime.Config.Validate(); err != nil {
				return fail(ExitRuntime, "config: %v", err)
			}
			fmt.Fprintf(runtime.Stdout, "config: ok (%s)\n", runtime.Paths.ConfigFile)
			if err := runtime.Store.Ping(cmd.Context()); err != nil {
				return fail(ExitRuntime, "database: %v; back up or move %s aside, then run jd doctor again; jd will not delete it automatically", err, runtime.Paths.DatabaseFile)
			}
			fmt.Fprintf(runtime.Stdout, "database: ok (%s)\n", runtime.Paths.DatabaseFile)
			fmt.Fprintln(runtime.Stdout, "shells: zsh bash fish powershell")
			return nil
		},
	}
}

func newVersionCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use: "version", Short: "Print the jd version", Args: cobra.NoArgs,
		Run: func(_ *cobra.Command, _ []string) {
			version := runtime.Version
			if version == "" {
				version = "dev"
			}
			fmt.Fprintf(runtime.Stdout, "jd %s\n", version)
		},
	}
}
