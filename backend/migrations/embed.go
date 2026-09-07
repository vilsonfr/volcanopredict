// Package migrations embeds the SQL migration files so the backend binary
// carries its own schema history and does not depend on a bind-mounted
// directory or docker-entrypoint-initdb.d at runtime (see design.md, D3).
//
// This file is not itself a migration and is exempt from the "migrations
// are immutable" rule (project.md) — it is Go glue that ships alongside
// them. The .sql files in this directory remain numbered and immutable;
// new schema changes are added as new numbered files, never edits to
// existing ones.
package migrations

import (
	"embed"
	"io/fs"
)

// raw embeds every .sql file in this directory, including 001_init.sql
// and 002_sources.sql. Those two predate goose (they were applied via
// docker-entrypoint-initdb.d) and were never written in goose's
// "-- +goose Up/Down" format, so goose cannot parse them as migrations.
// They stay in this directory only as an immutable historical record;
// legacyFiles below excludes them from what goose actually reads.
//
//go:embed *.sql
var raw embed.FS

// legacyFiles lists the pre-goose migration files that must be hidden
// from goose's view of this directory. 003 onward is where every
// goose-managed migration lives; nothing here needs updating when new
// migrations are added.
var legacyFiles = map[string]bool{
	"001_init.sql":    true,
	"002_sources.sql": true,
}

// FS is the filesystem goose.SetBaseFS should use: every embedded .sql
// file except the pre-goose legacy ones.
var FS fs.FS = legacyFilterFS{raw}

type legacyFilterFS struct {
	inner fs.FS
}

func (f legacyFilterFS) Open(name string) (fs.File, error) {
	if legacyFiles[baseName(name)] {
		return nil, fs.ErrNotExist
	}
	return f.inner.Open(name)
}

func (f legacyFilterFS) ReadDir(name string) ([]fs.DirEntry, error) {
	entries, err := fs.ReadDir(f.inner, name)
	if err != nil {
		return nil, err
	}
	filtered := entries[:0]
	for _, e := range entries {
		if !legacyFiles[e.Name()] {
			filtered = append(filtered, e)
		}
	}
	return filtered, nil
}

func baseName(name string) string {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '/' {
			return name[i+1:]
		}
	}
	return name
}
