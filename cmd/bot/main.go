// Command bot runs the DestinyScheduler Discord bot.
//
// It reads DISCORD_TOKEN, DATABASE_URL and optionally DISCORD_GUILD_ID
// (environment, then .env, then .env.development), refuses to start if the
// database schema is behind, registers the /raid command and serves
// interactions until SIGINT or SIGTERM.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/anthonycoutinho/destinyscheduler/internal/bot"
	"github.com/anthonycoutinho/destinyscheduler/internal/config"
	"github.com/anthonycoutinho/destinyscheduler/internal/database"
	raidrepo "github.com/anthonycoutinho/destinyscheduler/internal/repository/raid"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("bot stopped", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(config.DefaultFiles...)
	if err != nil {
		return err
	}
	if cfg.DiscordToken == "" {
		return errors.New("DISCORD_TOKEN is not set")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := database.CheckSchema(ctx, pool); err != nil {
		return err
	}

	session, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		return err
	}
	// Interactions arrive without privileged intents; Guilds is the minimum.
	session.Identify.Intents = discordgo.IntentsGuilds

	h := &bot.Handler{Raids: raidrepo.New(pool), Now: time.Now}
	session.AddHandler(h.Dispatch)
	if err := session.Open(); err != nil {
		return fmt.Errorf("connecting to Discord: %w", err)
	}
	defer session.Close()

	scope := "global"
	if cfg.DiscordGuildID != "" {
		scope = "guild " + cfg.DiscordGuildID
	}
	if _, err := session.ApplicationCommandBulkOverwrite(session.State.User.ID, cfg.DiscordGuildID, bot.Commands()); err != nil {
		return fmt.Errorf("registering commands: %w", err)
	}
	slog.Info("bot ready", "user", session.State.User.Username, "commands", scope)

	<-ctx.Done()
	slog.Info("shutting down")
	return nil
}
