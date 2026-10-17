// Command migrate applies or rolls back database migrations.
//
//	go run ./cmd/migrate up      # apply all pending migrations
//	go run ./cmd/migrate down    # roll back the most recent migration
//	go run ./cmd/migrate status  # list migrations and whether they're applied
//
// It connects using DATABASE_URL (environment, then .env, then .env.development).
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/anthonycoutinho/destinyscheduler/internal/config"
	"github.com/anthonycoutinho/destinyscheduler/internal/database"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 2 {
		return fmt.Errorf("usage: migrate up|down|status")
	}
	cfg, err := config.Load(config.DefaultFiles...)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	pool, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	switch os.Args[1] {
	case "up":
		return database.MigrateUp(ctx, pool)
	case "down":
		return database.MigrateDown(ctx, pool)
	case "status":
		status, err := database.Status(ctx, pool)
		if err != nil {
			return err
		}
		for _, s := range status {
			fmt.Printf("%-8s %s\n", s.State, s.Source.Path)
		}
		return nil
	default:
		return fmt.Errorf("unknown command %q (use up, down or status)", os.Args[1])
	}
}
