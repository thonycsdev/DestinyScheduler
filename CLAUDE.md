# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

DestinyScheduler is a Discord bot, written in Go, that lets a user schedule a Destiny 2 raid and shows the raid time converted into each fireteam member's local time zone/region. Raid times are stored in UTC and shown with Discord timestamps (`<t:UNIX:F>`), which Discord renders in each viewer's own time zone.

Layers:
- `internal/raid`: pure domain rules (scheduling, fireteam of 6, join/leave). No database or Discord imports.
- `internal/database`: Postgres connection (pgx v5 pool) and the versioned schema. Migrations are goose SQL files in `internal/database/migrations/`, embedded into the binary. CI applies them on pushes to `master`; the bot only calls `CheckSchema` on startup and refuses to run if the database is behind.
- `internal/repository/<domain>`: one package per domain that saves and loads it, using `internal/database`.
- `internal/bot`: Discord layer. `Handler` methods (`Create`, `Button`, `List`) are plain functions from an input struct to a `Response`, tested against real Postgres; `Dispatch` in `discord.go` is the thin, untested discordgo adapter. Refused actions become private (ephemeral) replies; only unexpected errors are returned.
- `cmd/bot`: entry point. Refuses to start if `DISCORD_TOKEN` is missing or `CheckSchema` fails.
- `infra/`: CloudFormation. `bootstrap.yml` (GitHub OIDC provider + CI deploy role; deployed once by hand) and `app.yml` (VPC, On-Demand `t4g.micro` in an ASG of exactly 1 running the bot container under systemd, ECR, Secrets Manager, logs; deployed by CI).
- AWS: everything is in **us-east-2**. The account's AWS Organization SCPs deny every other region and Spot launches; GitHub OIDC provider actions needed an SCP exception.
- `internal/config`: settings, read with precedence real env var > `.env` (git-ignored, local secrets) > `.env.development` (committed dev defaults). Never commit `.env`.

## Commands

```sh
docker compose up -d --wait                       # local Postgres 18 (credentials from .env.development)
go run ./cmd/migrate up                           # apply migrations (also: down, status)
go run ./cmd/bot                                  # run the bot (needs DISCORD_TOKEN)
docker build --platform linux/arm64 -t destiny-bot .  # production image (distroless, ~5 MB)
go test ./...                                     # run all tests (database tests need Docker running)
go test ./internal/raid -run TestScheduleRaid -v  # run a single test
go vet ./...
docker run --rm -v "$PWD/infra:/infra:ro" python:3.13-slim sh -c 'pip install -q cfn-lint && cfn-lint --regions us-east-2 -- /infra/*.yml'
```

CI (`.github/workflows/ci.yml`):
- Every PR and push to `master`: `go vet`, `go test`, `cfn-lint`, `actionlint`. PRs also build the image on `ubuntu-24.04-arm`.
- Pushes to `master` only, one at a time, in the `PRD` environment: assume `vars.AWS_DEPLOY_ROLE_ARN` via OIDC → `cloudformation deploy` the app stack → read `DATABASE_URL` from Secrets Manager (`destiny-scheduler/database-url`) and `go run ./cmd/migrate up` → build and push the arm64 image tagged with the commit SHA → set `/destiny-scheduler/image-tag` → restart `destiny-bot` via SSM Run Command and require it to still be running 20s later.
- Lint the workflow locally with `docker run --rm -v "$PWD:/repo" --workdir /repo rhysd/actionlint:latest` (bundles shellcheck for `run:` blocks).

To run the bot locally, put the **test** Discord application's `DISCORD_TOKEN` and the test server's `DISCORD_GUILD_ID` in `.env` (git-ignored). With `DISCORD_GUILD_ID` set, commands register in that server only and update instantly; production leaves it unset and registers globally. Never run locally with the production token: two running copies both answer every click.

New migration: add `internal/database/migrations/NNNNN_description.sql` with `-- +goose Up` and `-- +goose Down` sections; `TestMigrationsDownAndUp` checks it rolls back.

## Tests

`docs/tests.md` catalogs every test: the requirement it protects and the branch that introduced it. Read it to learn what behavior is already pinned down, and update it whenever a test is added or changed. Each test also has a doc comment stating its requirement.

## Workflow

Development in this repo is test-first:

1. The user provides tests. Claude suggests additional tests worth having and waits for agreement before implementing.
2. Implement only enough to make the agreed tests pass.
3. For each test/feature that is implemented and passes, create a branch and commit that feature alone:
   - Branch name: `feature/<short-name>` (e.g. `feature/raid-time-parsing`).
   - Branch off the previous feature branch (features stack), unless the user asks to merge each into `master` first.
   - One commit per feature: a summary line, then a body describing what was added and which tests cover it.
   - Don't push or open PRs unless asked. Remote: `origin` = `github.com/thonycsdev/DestinyScheduler` (public).
   - PRs are stacked: each feature PR targets the previous feature branch, so it shows only that feature's diff. The first targets `master`.
