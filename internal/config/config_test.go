package config

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/joho/godotenv"
)

// unsetEnv removes key from the process environment for the duration of the
// test, restoring it afterwards.
func unsetEnv(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "")
	os.Unsetenv(key)
}

// writeFile writes an env file into dir and returns its path.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestLoadPriority pins the agreed precedence for configuration: real
// environment variables override .env (local secrets, never committed), which
// overrides .env.development (committed development defaults).
func TestLoadPriority(t *testing.T) {
	dir := t.TempDir()
	local := writeFile(t, dir, ".env", "DATABASE_URL=postgres://from-dotenv\n")
	dev := writeFile(t, dir, ".env.development", "DATABASE_URL=postgres://from-development\n")

	t.Run(".env.development alone", func(t *testing.T) {
		unsetEnv(t, "DATABASE_URL")
		got, err := Load(filepath.Join(dir, "missing"), dev)
		if err != nil {
			t.Fatal(err)
		}
		if got.DatabaseURL != "postgres://from-development" {
			t.Errorf("DatabaseURL = %q, want the .env.development value", got.DatabaseURL)
		}
	})

	t.Run(".env overrides .env.development", func(t *testing.T) {
		unsetEnv(t, "DATABASE_URL")
		got, err := Load(local, dev)
		if err != nil {
			t.Fatal(err)
		}
		if got.DatabaseURL != "postgres://from-dotenv" {
			t.Errorf("DatabaseURL = %q, want the .env value", got.DatabaseURL)
		}
	})

	t.Run("environment overrides both files", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://from-environment")
		got, err := Load(local, dev)
		if err != nil {
			t.Fatal(err)
		}
		if got.DatabaseURL != "postgres://from-environment" {
			t.Errorf("DatabaseURL = %q, want the environment value", got.DatabaseURL)
		}
	})
}

// TestLoadDoesNotModifyEnvironment ensures loading env files doesn't leak
// their values into the process environment.
func TestLoadDoesNotModifyEnvironment(t *testing.T) {
	unsetEnv(t, "DATABASE_URL")
	dev := writeFile(t, t.TempDir(), ".env.development", "DATABASE_URL=postgres://from-development\n")
	if _, err := Load(dev); err != nil {
		t.Fatal(err)
	}
	if v, ok := os.LookupEnv("DATABASE_URL"); ok {
		t.Errorf("DATABASE_URL was set in the environment to %q", v)
	}
}

// TestLoadRequiresDatabaseURL ensures a missing connection string is reported
// at startup instead of failing on the first query. Missing env files are not
// an error by themselves (CI and production have none).
func TestLoadRequiresDatabaseURL(t *testing.T) {
	unsetEnv(t, "DATABASE_URL")
	_, err := Load(filepath.Join(t.TempDir(), ".env"))
	if !errors.Is(err, ErrMissingDatabaseURL) {
		t.Errorf("err = %v, want ErrMissingDatabaseURL", err)
	}
}

// TestDevelopmentEnvMatchesCompose ensures the committed .env.development
// DATABASE_URL points at the docker-compose Postgres with the same user,
// password and database the container is created with.
func TestDevelopmentEnvMatchesCompose(t *testing.T) {
	env, err := godotenv.Read(filepath.Join("..", "..", ".env.development"))
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(env["DATABASE_URL"])
	if err != nil {
		t.Fatalf("DATABASE_URL: %v", err)
	}
	password, _ := u.User.Password()
	checks := []struct{ what, got, want string }{
		{"user", u.User.Username(), env["POSTGRES_USER"]},
		{"password", password, env["POSTGRES_PASSWORD"]},
		{"database", u.Path[1:], env["POSTGRES_DB"]},
		{"host", u.Host, "localhost:5432"},
	}
	for _, c := range checks {
		if c.want == "" || c.got != c.want {
			t.Errorf("DATABASE_URL %s = %q, want %q", c.what, c.got, c.want)
		}
	}
}
