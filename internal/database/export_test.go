// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package database

import (
	"io/fs"
	"strconv"
	"testing/fstest"
)

// MigrateFS runs the migration runner on the migrations in fsys instead of the
// embedded ones.
var MigrateFS = migrateFS

// MigrationsUpTo returns the embedded migrations of driver up to and including
// version, as a database migrated by an older release saw them.
func MigrationsUpTo(driver string, version int64) (fs.FS, error) {
	fsys, err := migrationFS(driver)
	if err != nil {
		return nil, err
	}
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	out := fstest.MapFS{}
	for _, e := range entries {
		v, err := strconv.ParseInt(e.Name()[:5], 10, 64)
		if err != nil || v > version {
			continue
		}
		data, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			return nil, err
		}
		out[e.Name()] = &fstest.MapFile{Data: data}
	}
	return out, nil
}
