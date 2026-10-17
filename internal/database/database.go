// Package database owns the connection to Postgres and the versioned schema.
//
// Migrations are SQL files in migrations/, embedded into the binary and
// applied with goose. CI applies them on pushes to master (cmd/migrate);
// the bot only calls CheckSchema on startup and refuses to run if the
// database is behind.
package database

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var embedded embed.FS

var ErrSchemaBehind = errors.New("database schema is behind; run migrations (go run ./cmd/migrate up)")

// Open connects to Postgres and verifies the connection.
func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connecting to database: %w", err)
	}
	return pool, nil
}

// withMigrator runs fn with a goose provider for the embedded migrations.
func withMigrator(pool *pgxpool.Pool, fn func(*goose.Provider) error) error {
	migrations, err := fs.Sub(embedded, "migrations")
	if err != nil {
		return err
	}
	db := stdlib.OpenDBFromPool(pool) // closing db does not close pool
	defer db.Close()
	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations)
	if err != nil {
		return err
	}
	return fn(p)
}

// MigrateUp applies all pending migrations.
func MigrateUp(ctx context.Context, pool *pgxpool.Pool) error {
	return withMigrator(pool, func(p *goose.Provider) error {
		_, err := p.Up(ctx)
		return err
	})
}

// MigrateDownTo rolls migrations back until the schema is at version.
// Version 0 removes every migration.
func MigrateDownTo(ctx context.Context, pool *pgxpool.Pool, version int64) error {
	return withMigrator(pool, func(p *goose.Provider) error {
		_, err := p.DownTo(ctx, version)
		return err
	})
}

// MigrateDown rolls back the most recent migration.
func MigrateDown(ctx context.Context, pool *pgxpool.Pool) error {
	return withMigrator(pool, func(p *goose.Provider) error {
		_, err := p.Down(ctx)
		return err
	})
}

// Status lists every migration and whether it has been applied.
func Status(ctx context.Context, pool *pgxpool.Pool) ([]*goose.MigrationStatus, error) {
	var status []*goose.MigrationStatus
	err := withMigrator(pool, func(p *goose.Provider) error {
		var err error
		status, err = p.Status(ctx)
		return err
	})
	return status, err
}

// CheckSchema returns ErrSchemaBehind if any migration hasn't been applied.
func CheckSchema(ctx context.Context, pool *pgxpool.Pool) error {
	return withMigrator(pool, func(p *goose.Provider) error {
		current, target, err := p.GetVersions(ctx)
		if err != nil {
			return err
		}
		if current < target {
			return fmt.Errorf("%w: at version %d, latest is %d", ErrSchemaBehind, current, target)
		}
		return nil
	})
}
