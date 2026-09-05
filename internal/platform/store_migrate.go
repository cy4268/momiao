package platform

import (
	"context"
	"crypto/sha256"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

//go:embed migrations/*.sql
var migrations embed.FS
var ErrMigrationMismatch = errors.New("migration version or checksum mismatch")

// Migrate is an explicit administration operation. One transaction and a fixed
// advisory lock protect the migration registry and all pending schema changes.
// Runtime startup must not call this method or use the database-owner role.
func (s *Store) Migrate(ctx context.Context) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(714622314783630001)"); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS platform_meta; CREATE TABLE IF NOT EXISTS platform_meta.schema_migrations(version BIGINT PRIMARY KEY,checksum TEXT NOT NULL,applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	files, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return err
	}
	applied := map[int64]string{}
	rows, err := tx.Query(ctx, "SELECT version,checksum FROM platform_meta.schema_migrations ORDER BY version")
	if err != nil {
		return err
	}
	for rows.Next() {
		var v int64
		var h string
		if err = rows.Scan(&v, &h); err != nil {
			rows.Close()
			return err
		}
		applied[v] = h
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	// Validate the full manifest before running any migration; unknown versions
	// reject an older executable instead of silently accepting a newer schema.
	type migration struct {
		version       int64
		checksum, sql string
	}
	pending := []migration{}
	seen := map[int64]bool{}
	for i, file := range files {
		name := strings.TrimPrefix(file, "migrations/")
		v, err := strconv.ParseInt(strings.SplitN(name, "_", 2)[0], 10, 64)
		if err != nil || v != int64(i+1) {
			return fmt.Errorf("%w: nonsequential embedded migration", ErrMigrationMismatch)
		}
		data, err := migrations.ReadFile(file)
		if err != nil {
			return err
		}
		checksum := fmt.Sprintf("%x", sha256.Sum256(data))
		seen[v] = true
		if old, ok := applied[v]; ok {
			if old != checksum {
				return fmt.Errorf("%w: version %d", ErrMigrationMismatch, v)
			}
		} else {
			pending = append(pending, migration{v, checksum, string(data)})
		}
	}
	for v := range applied {
		if !seen[v] {
			return fmt.Errorf("%w: unknown version %d", ErrMigrationMismatch, v)
		}
		for previous := int64(1); previous < v; previous++ {
			if _, ok := applied[previous]; !ok {
				return fmt.Errorf("%w: migration gap", ErrMigrationMismatch)
			}
		}
	}
	for _, m := range pending {
		if _, err = tx.Exec(ctx, m.sql, pgx.QueryExecModeSimpleProtocol); err != nil {
			return fmt.Errorf("migration %d: %w", m.version, err)
		}
		if _, err = tx.Exec(ctx, "INSERT INTO platform_meta.schema_migrations(version,checksum) VALUES($1,$2)", m.version, m.checksum); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
