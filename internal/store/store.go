package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	_ "modernc.org/sqlite"
)

const SchemaVersion = 1

type Store struct {
	db   *sql.DB
	path string
}

type Directory struct {
	Path         string
	Pin          string
	VisitCount   int
	FirstSeen    time.Time
	LastVisited  time.Time
	LastSeen     time.Time
	MissingSince time.Time
	Visited      bool
	Scanned      bool
}

type Pin struct {
	Name string
	Path string
}

type Root struct {
	Path           string
	MaxDepth       int
	IncludeHidden  bool
	FollowSymlinks bool
	LastScanned    time.Time
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	dsn := "file:" + url.PathEscape(path) + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(2)
	store := &Store{db: db, path: path}
	if err := store.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Path() string { return s.path }

func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func (s *Store) migrate(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS directories (
			path TEXT PRIMARY KEY,
			visit_count INTEGER NOT NULL DEFAULT 0,
			first_seen INTEGER NOT NULL,
			last_visited INTEGER,
			last_seen INTEGER NOT NULL,
			missing_since INTEGER,
			visited INTEGER NOT NULL DEFAULT 0,
			scanned INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS pins (
			name TEXT PRIMARY KEY COLLATE NOCASE,
			path TEXT NOT NULL REFERENCES directories(path) ON DELETE RESTRICT,
			created_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS roots (
			path TEXT PRIMARY KEY,
			max_depth INTEGER NOT NULL,
			include_hidden INTEGER NOT NULL,
			follow_symlinks INTEGER NOT NULL,
			last_scanned INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS root_entries (
			root_path TEXT NOT NULL REFERENCES roots(path) ON DELETE CASCADE,
			dir_path TEXT NOT NULL REFERENCES directories(path) ON DELETE CASCADE,
			PRIMARY KEY (root_path, dir_path)
		)`,
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate database: %w", err)
		}
	}
	var versionText string
	err = tx.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = 'schema_version'`).Scan(&versionText)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.ExecContext(ctx, `INSERT INTO meta(key, value) VALUES ('schema_version', ?)`, strconv.Itoa(SchemaVersion))
	} else if err == nil {
		version, parseErr := strconv.Atoi(versionText)
		if parseErr != nil || version != SchemaVersion {
			return fmt.Errorf("unsupported database schema version %q", versionText)
		}
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) RecordVisit(ctx context.Context, path string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO directories(path, visit_count, first_seen, last_visited, last_seen, missing_since, visited, scanned)
		VALUES (?, 1, ?, ?, ?, NULL, 1, 0)
		ON CONFLICT(path) DO UPDATE SET
			visit_count = directories.visit_count + 1,
			last_visited = excluded.last_visited,
			last_seen = excluded.last_seen,
			missing_since = NULL,
			visited = 1`, path, at.Unix(), at.Unix(), at.Unix())
	if err != nil {
		return fmt.Errorf("record visit: %w", err)
	}
	return nil
}

func (s *Store) SetPin(ctx context.Context, name, path string, at time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO directories(path, first_seen, last_seen)
		VALUES (?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET last_seen = excluded.last_seen, missing_since = NULL`, path, at.Unix(), at.Unix()); err != nil {
		return fmt.Errorf("ensure pin directory: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO pins(name, path, created_at) VALUES (?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET path = excluded.path, created_at = excluded.created_at`, name, path, at.Unix()); err != nil {
		return fmt.Errorf("set pin: %w", err)
	}
	return tx.Commit()
}

func (s *Store) DeletePin(ctx context.Context, name string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM pins WHERE name = ? COLLATE NOCASE`, name)
	return err
}

func (s *Store) ListPins(ctx context.Context) ([]Pin, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name, path FROM pins ORDER BY lower(name), path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pins []Pin
	for rows.Next() {
		var pin Pin
		if err := rows.Scan(&pin.Name, &pin.Path); err != nil {
			return nil, err
		}
		pins = append(pins, pin)
	}
	return pins, rows.Err()
}

func (s *Store) ListDirectories(ctx context.Context, includeMissing bool) ([]Directory, error) {
	query := `SELECT d.path,
		COALESCE((SELECT min(p.name) FROM pins p WHERE p.path = d.path), ''),
		d.visit_count, d.first_seen, d.last_visited, d.last_seen, d.missing_since, d.visited, d.scanned
		FROM directories d`
	if !includeMissing {
		query += ` WHERE d.missing_since IS NULL`
	}
	query += ` ORDER BY lower(d.path)`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []Directory
	for rows.Next() {
		var entry Directory
		var first, lastSeen int64
		var lastVisited, missing sql.NullInt64
		if err := rows.Scan(&entry.Path, &entry.Pin, &entry.VisitCount, &first, &lastVisited, &lastSeen, &missing, &entry.Visited, &entry.Scanned); err != nil {
			return nil, err
		}
		entry.FirstSeen = time.Unix(first, 0)
		entry.LastSeen = time.Unix(lastSeen, 0)
		if lastVisited.Valid {
			entry.LastVisited = time.Unix(lastVisited.Int64, 0)
		}
		if missing.Valid {
			entry.MissingSince = time.Unix(missing.Int64, 0)
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (s *Store) MarkMissing(ctx context.Context, path string, since time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE directories SET missing_since = COALESCE(missing_since, ?) WHERE path = ?`, since.Unix(), path)
	return err
}

func (s *Store) MarkPresent(ctx context.Context, path string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE directories SET missing_since = NULL, last_seen = ? WHERE path = ?`, at.Unix(), path)
	return err
}

func (s *Store) CleanStale(ctx context.Context, now time.Time, staleDays int) (int64, error) {
	cutoff := now.Add(-time.Duration(staleDays) * 24 * time.Hour).Unix()
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM directories
		WHERE missing_since IS NOT NULL AND missing_since <= ?
		AND NOT EXISTS (SELECT 1 FROM pins WHERE pins.path = directories.path)`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *Store) AddRoot(ctx context.Context, root Root) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO roots(path, max_depth, include_hidden, follow_symlinks)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			max_depth = excluded.max_depth,
			include_hidden = excluded.include_hidden,
			follow_symlinks = excluded.follow_symlinks`,
		root.Path, root.MaxDepth, root.IncludeHidden, root.FollowSymlinks)
	if err != nil {
		return fmt.Errorf("add scan root: %w", err)
	}
	return nil
}

func (s *Store) ListRoots(ctx context.Context) ([]Root, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT path, max_depth, include_hidden, follow_symlinks, last_scanned
		FROM roots ORDER BY lower(path)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var roots []Root
	for rows.Next() {
		var root Root
		var last sql.NullInt64
		if err := rows.Scan(&root.Path, &root.MaxDepth, &root.IncludeHidden, &root.FollowSymlinks, &last); err != nil {
			return nil, err
		}
		if last.Valid {
			root.LastScanned = time.Unix(last.Int64, 0)
		}
		roots = append(roots, root)
	}
	return roots, rows.Err()
}

func (s *Store) ReplaceRootEntries(ctx context.Context, rootPath string, paths []string, at time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM root_entries WHERE root_path = ?`, rootPath); err != nil {
		return err
	}
	for _, path := range paths {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO directories(path, first_seen, last_seen, scanned)
			VALUES (?, ?, ?, 1)
			ON CONFLICT(path) DO UPDATE SET last_seen = excluded.last_seen, missing_since = NULL, scanned = 1`,
			path, at.Unix(), at.Unix()); err != nil {
			return fmt.Errorf("record scanned directory: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO root_entries(root_path, dir_path) VALUES (?, ?)`, rootPath, path); err != nil {
			return fmt.Errorf("link scanned directory: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE roots SET last_scanned = ? WHERE path = ?`, at.Unix(), rootPath); err != nil {
		return err
	}
	if err := reconcileScanned(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) RemoveRoot(ctx context.Context, rootPath string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM roots WHERE path = ?`, rootPath); err != nil {
		return err
	}
	if err := reconcileScanned(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

func reconcileScanned(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE directories SET scanned = EXISTS(
			SELECT 1 FROM root_entries WHERE root_entries.dir_path = directories.path
		)`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM directories
		WHERE visited = 0 AND scanned = 0
		AND NOT EXISTS (SELECT 1 FROM pins WHERE pins.path = directories.path)`); err != nil {
		return err
	}
	return nil
}

func (s *Store) ForgetVisit(ctx context.Context, path string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		UPDATE directories SET visited = 0, visit_count = 0, last_visited = NULL
		WHERE path = ?`, path); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM directories
		WHERE path = ? AND visited = 0 AND scanned = 0
		AND NOT EXISTS (SELECT 1 FROM pins WHERE pins.path = directories.path)`, path); err != nil {
		return err
	}
	return tx.Commit()
}
