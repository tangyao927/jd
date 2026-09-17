package store

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Database is the persistence contract consumed by the CLI.
type Database interface {
	Ping(context.Context) error
	RecordVisit(context.Context, string, time.Time) error
	SetPin(context.Context, string, string, time.Time) error
	DeletePin(context.Context, string) error
	ListPins(context.Context) ([]Pin, error)
	ListDirectories(context.Context, bool) ([]Directory, error)
	MarkMissing(context.Context, string, time.Time) error
	MarkPresent(context.Context, string, time.Time) error
	CleanStale(context.Context, time.Time, int) (int64, error)
	AddRoot(context.Context, Root) error
	ListRoots(context.Context) ([]Root, error)
	ReplaceRootEntries(context.Context, string, []string, time.Time) error
	RemoveRoot(context.Context, string) error
	ForgetVisit(context.Context, string) error
}

// Lazy opens the SQLite database only when a stateful command first uses it.
type Lazy struct {
	path  string
	once  sync.Once
	store *Store
	err   error
}

func NewLazy(path string) *Lazy { return &Lazy{path: path} }

func (l *Lazy) database() (*Store, error) {
	l.once.Do(func() {
		l.store, l.err = Open(l.path)
		if l.err != nil {
			l.err = fmt.Errorf("open database %s: %w", l.path, l.err)
		}
	})
	return l.store, l.err
}

func (l *Lazy) Close() error {
	if l.store == nil {
		return nil
	}
	return l.store.Close()
}

func (l *Lazy) Ping(ctx context.Context) error {
	db, err := l.database()
	if err != nil {
		return err
	}
	return db.Ping(ctx)
}

func (l *Lazy) RecordVisit(ctx context.Context, path string, at time.Time) error {
	db, err := l.database()
	if err != nil {
		return err
	}
	return db.RecordVisit(ctx, path, at)
}

func (l *Lazy) SetPin(ctx context.Context, name, path string, at time.Time) error {
	db, err := l.database()
	if err != nil {
		return err
	}
	return db.SetPin(ctx, name, path, at)
}

func (l *Lazy) DeletePin(ctx context.Context, name string) error {
	db, err := l.database()
	if err != nil {
		return err
	}
	return db.DeletePin(ctx, name)
}

func (l *Lazy) ListPins(ctx context.Context) ([]Pin, error) {
	db, err := l.database()
	if err != nil {
		return nil, err
	}
	return db.ListPins(ctx)
}

func (l *Lazy) ListDirectories(ctx context.Context, includeMissing bool) ([]Directory, error) {
	db, err := l.database()
	if err != nil {
		return nil, err
	}
	return db.ListDirectories(ctx, includeMissing)
}

func (l *Lazy) MarkMissing(ctx context.Context, path string, at time.Time) error {
	db, err := l.database()
	if err != nil {
		return err
	}
	return db.MarkMissing(ctx, path, at)
}

func (l *Lazy) MarkPresent(ctx context.Context, path string, at time.Time) error {
	db, err := l.database()
	if err != nil {
		return err
	}
	return db.MarkPresent(ctx, path, at)
}

func (l *Lazy) CleanStale(ctx context.Context, at time.Time, staleDays int) (int64, error) {
	db, err := l.database()
	if err != nil {
		return 0, err
	}
	return db.CleanStale(ctx, at, staleDays)
}

func (l *Lazy) AddRoot(ctx context.Context, root Root) error {
	db, err := l.database()
	if err != nil {
		return err
	}
	return db.AddRoot(ctx, root)
}

func (l *Lazy) ListRoots(ctx context.Context) ([]Root, error) {
	db, err := l.database()
	if err != nil {
		return nil, err
	}
	return db.ListRoots(ctx)
}

func (l *Lazy) ReplaceRootEntries(ctx context.Context, root string, paths []string, at time.Time) error {
	db, err := l.database()
	if err != nil {
		return err
	}
	return db.ReplaceRootEntries(ctx, root, paths, at)
}

func (l *Lazy) RemoveRoot(ctx context.Context, root string) error {
	db, err := l.database()
	if err != nil {
		return err
	}
	return db.RemoveRoot(ctx, root)
}

func (l *Lazy) ForgetVisit(ctx context.Context, path string) error {
	db, err := l.database()
	if err != nil {
		return err
	}
	return db.ForgetVisit(ctx, path)
}
