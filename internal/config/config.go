package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const CurrentVersion = 1

type ScanConfig struct {
	MaxDepth       int      `toml:"max_depth"`
	IncludeHidden  bool     `toml:"include_hidden"`
	FollowSymlinks bool     `toml:"follow_symlinks"`
	Ignore         []string `toml:"ignore"`
}

func Set(cfg *Config, key, value string) error {
	next := *cfg
	switch key {
	case "max_results":
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("max_results must be an integer: %w", err)
		}
		next.MaxResults = parsed
	case "stale_days":
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("stale_days must be an integer: %w", err)
		}
		next.StaleDays = parsed
	case "track_shell_cd":
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("track_shell_cd must be true or false: %w", err)
		}
		next.TrackShellCD = parsed
	case "scan.max_depth":
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("scan.max_depth must be an integer: %w", err)
		}
		next.Scan.MaxDepth = parsed
	case "scan.include_hidden":
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("scan.include_hidden must be true or false: %w", err)
		}
		next.Scan.IncludeHidden = parsed
	case "scan.follow_symlinks":
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("scan.follow_symlinks must be true or false: %w", err)
		}
		next.Scan.FollowSymlinks = parsed
	case "scan.ignore":
		values := strings.Split(value, ",")
		next.Scan.Ignore = next.Scan.Ignore[:0]
		for _, item := range values {
			if item = strings.TrimSpace(item); item != "" {
				next.Scan.Ignore = append(next.Scan.Ignore, item)
			}
		}
	default:
		return fmt.Errorf("unknown config key %q", key)
	}
	if err := next.Validate(); err != nil {
		return err
	}
	*cfg = next
	return nil
}

func Get(cfg Config, key string) (string, error) {
	switch key {
	case "version":
		return strconv.Itoa(cfg.Version), nil
	case "max_results":
		return strconv.Itoa(cfg.MaxResults), nil
	case "stale_days":
		return strconv.Itoa(cfg.StaleDays), nil
	case "track_shell_cd":
		return strconv.FormatBool(cfg.TrackShellCD), nil
	case "scan.max_depth":
		return strconv.Itoa(cfg.Scan.MaxDepth), nil
	case "scan.include_hidden":
		return strconv.FormatBool(cfg.Scan.IncludeHidden), nil
	case "scan.follow_symlinks":
		return strconv.FormatBool(cfg.Scan.FollowSymlinks), nil
	case "scan.ignore":
		return strings.Join(cfg.Scan.Ignore, ","), nil
	default:
		return "", fmt.Errorf("unknown config key %q", key)
	}
}

func Encode(writer io.Writer, cfg Config) error {
	return toml.NewEncoder(writer).Encode(cfg)
}

type Config struct {
	Version      int        `toml:"version"`
	MaxResults   int        `toml:"max_results"`
	StaleDays    int        `toml:"stale_days"`
	TrackShellCD bool       `toml:"track_shell_cd"`
	Scan         ScanConfig `toml:"scan"`
}

type Paths struct {
	ConfigFile   string
	DatabaseFile string
}

func Default() Config {
	return Config{
		Version:      CurrentVersion,
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
}

func (c Config) Validate() error {
	if c.Version != CurrentVersion {
		return fmt.Errorf("unsupported config version %d", c.Version)
	}
	if c.MaxResults <= 0 {
		return errors.New("max_results must be greater than zero")
	}
	if c.StaleDays < 0 {
		return errors.New("stale_days cannot be negative")
	}
	if c.Scan.MaxDepth < 0 {
		return errors.New("scan.max_depth cannot be negative")
	}
	return nil
}

func Load(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Save(path string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary config: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}

func ResolvePaths() (Paths, error) {
	configDir := os.Getenv("JD_CONFIG_DIR")
	dataDir := os.Getenv("JD_DATA_DIR")
	if configDir == "" {
		var err error
		configDir, err = defaultConfigDir()
		if err != nil {
			return Paths{}, err
		}
	}
	if dataDir == "" {
		var err error
		dataDir, err = defaultDataDir()
		if err != nil {
			return Paths{}, err
		}
	}
	return Paths{
		ConfigFile:   filepath.Join(configDir, "config.toml"),
		DatabaseFile: filepath.Join(dataDir, "state.db"),
	}, nil
}

func defaultConfigDir() (string, error) {
	if runtime.GOOS == "windows" {
		if dir := os.Getenv("APPDATA"); dir != "" {
			return filepath.Join(dir, "jd"), nil
		}
	}
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "jd"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".config", "jd"), nil
}

func defaultDataDir() (string, error) {
	if runtime.GOOS == "windows" {
		if dir := os.Getenv("LOCALAPPDATA"); dir != "" {
			return filepath.Join(dir, "jd"), nil
		}
	}
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return filepath.Join(dir, "jd"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".local", "share", "jd"), nil
}
