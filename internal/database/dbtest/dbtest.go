// Package dbtest gives tests a real Postgres 18 database.
//
// A test package calls Run from TestMain: it starts one throwaway Postgres
// container (testcontainers-go) and migrates it from empty. Each test then
// calls Pool, which empties every table first, so tests never see each
// other's data. Docker must be running.
package dbtest

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/anthonycoutinho/destinyscheduler/internal/database"
)

// Image matches the version pinned in docker-compose.yml.
const Image = "postgres:18"

var pool *pgxpool.Pool

// Run starts Postgres, applies all migrations, runs the tests and tears the
// container down. Use it as: func TestMain(m *testing.M) { os.Exit(dbtest.Run(m)) }
func Run(m *testing.M) int {
	ctx := context.Background()
	c, err := postgres.Run(ctx, Image,
		postgres.WithDatabase("destiny_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dbtest: starting postgres (is Docker running?):", err)
		return 1
	}
	defer testcontainers.TerminateContainer(c)

	url, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintln(os.Stderr, "dbtest:", err)
		return 1
	}
	pool, err = database.Open(ctx, url)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dbtest:", err)
		return 1
	}
	defer pool.Close()

	if err := database.MigrateUp(ctx, pool); err != nil {
		fmt.Fprintln(os.Stderr, "dbtest: migrating:", err)
		return 1
	}
	return m.Run()
}

// Pool returns the shared test database after emptying every table.
func Pool(t testing.TB) *pgxpool.Pool {
	t.Helper()
	if pool == nil {
		t.Fatal("dbtest: call dbtest.Run from TestMain")
	}
	Reset(t, pool)
	return pool
}

// Reset empties every table except goose's version table, keeping the schema.
func Reset(t testing.TB, p *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	var tables *string
	err := p.QueryRow(ctx, `
		SELECT string_agg(format('%I.%I', schemaname, tablename), ', ')
		FROM pg_tables
		WHERE schemaname = 'public' AND tablename <> 'goose_db_version'`).Scan(&tables)
	if err != nil {
		t.Fatalf("dbtest: listing tables: %v", err)
	}
	if tables == nil {
		return
	}
	if _, err := p.Exec(ctx, "TRUNCATE "+*tables+" RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("dbtest: truncating: %v", err)
	}
}
