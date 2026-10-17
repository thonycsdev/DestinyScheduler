# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

DestinyScheduler is a Discord bot, written in Go, that lets a user schedule a Destiny 2 raid and shows the raid time converted into each fireteam member's local time zone/region. Raid times are stored in UTC. The Discord layer (planned: `github.com/bwmarrin/discordgo`) isn't built yet; the domain logic lives in `internal/raid`, which has no Discord dependency.

## Commands

```sh
go test ./...                                     # run all tests
go test ./internal/raid -run TestScheduleRaid -v  # run a single test
go vet ./...
```

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
