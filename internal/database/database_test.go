package database_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anthonycoutinho/destinyscheduler/internal/database"
	"github.com/anthonycoutinho/destinyscheduler/internal/database/dbtest"
)

func TestMain(m *testing.M) { os.Exit(dbtest.Run(m)) }

func tableExists(t *testing.T, pool *pgxpool.Pool, name string) bool {
	t.Helper()
	var ok bool
	if err := pool.QueryRow(context.Background(), "SELECT to_regclass($1) IS NOT NULL", name).Scan(&ok); err != nil {
		t.Fatal(err)
	}
	return ok
}

// TestMigrationsCreateRaidSchema pins the raid schema the repository relies
// on: raids get a UUIDv7 id by default, a member can't be in the same raid
// twice, and deleting a raid deletes its members.
func TestMigrationsCreateRaidSchema(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()

	var id string
	var version int
	err := pool.QueryRow(ctx, `
		INSERT INTO raids (name, starts_at, leader_id) VALUES ('Last Wish', now() + interval '1 day', 'leader-1')
		RETURNING id, uuid_extract_version(id)`).Scan(&id, &version)
	if err != nil {
		t.Fatalf("inserting raid: %v", err)
	}
	if version != 7 {
		t.Errorf("default id %s is UUID version %d, want 7", id, version)
	}

	if _, err := pool.Exec(ctx, "INSERT INTO raid_members (raid_id, user_id) VALUES ($1, 'leader-1')", id); err != nil {
		t.Fatalf("inserting member: %v", err)
	}
	_, err = pool.Exec(ctx, "INSERT INTO raid_members (raid_id, user_id) VALUES ($1, 'leader-1')", id)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Errorf("duplicate member insert: err = %v, want unique violation (23505)", err)
	}

	if _, err := pool.Exec(ctx, "DELETE FROM raids WHERE id = $1", id); err != nil {
		t.Fatal(err)
	}
	var members int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM raid_members WHERE raid_id = $1", id).Scan(&members); err != nil {
		t.Fatal(err)
	}
	if members != 0 {
		t.Errorf("after deleting raid, %d members remain, want 0", members)
	}
}

// TestMigrationsDownAndUp ensures every migration can be rolled back to an
// empty schema and re-applied, and that CheckSchema reports a database that
// is behind (what the bot checks on startup) and accepts one that is current.
func TestMigrationsDownAndUp(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()

	if err := database.CheckSchema(ctx, pool); err != nil {
		t.Fatalf("CheckSchema on migrated database: %v", err)
	}

	if err := database.MigrateDownTo(ctx, pool, 0); err != nil {
		t.Fatalf("migrating down to 0: %v", err)
	}
	for _, table := range []string{"raids", "raid_members"} {
		if tableExists(t, pool, table) {
			t.Errorf("table %s still exists after migrating down to 0", table)
		}
	}
	if err := database.CheckSchema(ctx, pool); !errors.Is(err, database.ErrSchemaBehind) {
		t.Errorf("CheckSchema on empty schema: err = %v, want ErrSchemaBehind", err)
	}

	if err := database.MigrateUp(ctx, pool); err != nil {
		t.Fatalf("migrating back up: %v", err)
	}
	for _, table := range []string{"raids", "raid_members"} {
		if !tableExists(t, pool, table) {
			t.Errorf("table %s missing after migrating up", table)
		}
	}
	if err := database.CheckSchema(ctx, pool); err != nil {
		t.Errorf("CheckSchema after migrating up: %v", err)
	}
}

// TestPoolResetsBetweenTests ensures dbtest.Pool empties every table but keeps
// the migrated schema, so each test starts from a clean database.
func TestPoolResetsBetweenTests(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.Pool(t)
	if _, err := pool.Exec(ctx, "INSERT INTO raids (name, starts_at, leader_id) VALUES ('Last Wish', now() + interval '1 day', 'leader-1')"); err != nil {
		t.Fatal(err)
	}

	pool = dbtest.Pool(t)
	var raids int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM raids").Scan(&raids); err != nil {
		t.Fatal(err)
	}
	if raids != 0 {
		t.Errorf("after reset, %d raids remain, want 0", raids)
	}
	if err := database.CheckSchema(ctx, pool); err != nil {
		t.Errorf("reset changed the schema: %v", err)
	}
}
