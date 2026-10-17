// Package config loads the bot's settings from the environment and env files.
package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// DefaultFiles are the env files read by the bot, highest priority first:
// .env holds local secrets and overrides and is never committed;
// .env.development holds committed development defaults.
var DefaultFiles = []string{".env", ".env.development"}

var ErrMissingDatabaseURL = errors.New("DATABASE_URL is not set")

// Config holds the bot's settings.
type Config struct {
	DatabaseURL string
	// DiscordToken is the bot token. Required by cmd/bot only.
	DiscordToken string
	// DiscordGuildID, when set, registers commands in that one server only
	// (instant; for the test application). Unset in production: global.
	DiscordGuildID string
}

// Load reads settings from the process environment, then from files in
// order; the first non-empty value wins. Missing files are skipped. Values
// from files are not written into the process environment.
func Load(files ...string) (Config, error) {
	var layers []map[string]string
	for _, f := range files {
		vals, err := godotenv.Read(f)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return Config{}, fmt.Errorf("reading %s: %w", f, err)
		}
		layers = append(layers, vals)
	}
	lookup := func(key string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		for _, l := range layers {
			if v := l[key]; v != "" {
				return v
			}
		}
		return ""
	}

	c := Config{
		DatabaseURL:    lookup("DATABASE_URL"),
		DiscordToken:   lookup("DISCORD_TOKEN"),
		DiscordGuildID: lookup("DISCORD_GUILD_ID"),
	}
	if c.DatabaseURL == "" {
		return Config{}, ErrMissingDatabaseURL
	}
	return c, nil
}
