package migrations

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed *.sql
var Files embed.FS
var migrationName = regexp.MustCompile(`^[0-9]{4}_[a-z0-9_]+\.sql$`)

func Apply(ctx context.Context, pool *pgxpool.Pool, files fs.FS) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return errors.New("acquire migration connection")
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(761942061)`); err != nil {
		return errors.New("lock migrations")
	}
	defer func() { _, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock(761942061)`) }()
	if _, err = conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version text PRIMARY KEY, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return errors.New("create migration history")
	}
	names, err := fs.Glob(files, "*.sql")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	sort.Strings(names)
	var previous string
	for _, name := range names {
		if !migrationName.MatchString(name) || previous == name[:4] {
			return fmt.Errorf("invalid migration filename %q", name)
		}
		previous = name[:4]
		source, err := fs.ReadFile(files, name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if len(bytes.TrimSpace(source)) == 0 {
			return fmt.Errorf("migration %s is empty", name)
		}
		checksum := sha256.Sum256(source)
		digest := hex.EncodeToString(checksum[:])
		var applied string
		err = conn.QueryRow(ctx, `SELECT checksum FROM schema_migrations WHERE version=$1`, name).Scan(&applied)
		if err == nil {
			if applied != digest {
				return fmt.Errorf("migration %s checksum changed", name)
			}
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return errors.New("read migration history")
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", name, err)
		}
		if _, err = tx.Exec(ctx, string(source)); err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO schema_migrations(version, checksum) VALUES ($1,$2)`, name, digest)
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if err = tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
	}
	return nil
}
