# Test catalog

Every test in the repo, the requirement it protects, and the branch that introduced it. Keep this in sync when adding or changing tests.

Run one with `go test ./internal/<package> -run <TestName> -v`.

## internal/raid

| Test | Requirement | Branch |
|---|---|---|
| `TestScheduleRaid` | `Schedule(name, start, leaderID, now)` returns a raid with the raid name, the leader as `LeaderID` and only member, the start time converted to UTC (21:00 `America/Sao_Paulo` → 00:00 UTC next day), and empty `ID`/`ChannelID`/`MessageID`. | `feature/schedule-raid`, reworked in `feature/schedule-with-leader` |
| `TestScheduleRaidRequiresLeader` | An empty leader ID fails with `ErrNoLeader`. Replaced the old "at least one member" test. | `feature/schedule-with-leader` |
| `TestScheduleRaidRejectsUnknownRaid` | Only raids in `raid.Names` can be scheduled; an unknown or empty name fails with `ErrUnknownRaid`. | `feature/known-raids` |
| `TestScheduleRaidAcceptsEveryKnownRaid` | Every raid in `raid.Names` is accepted, so the list and the validation stay in sync. | `feature/known-raids` |
| `TestScheduleRaidRejectsPastTimes` | A start time at or before `now` fails with `ErrInPast`. `Schedule` takes `now` explicitly; tests pass the fixed `testNow` (2026-10-17 12:00 UTC). | `feature/reject-past-times` |
| `TestParseStart` | `ParseStart(date, clock, timezone)` turns user input (`YYYY-MM-DD`, 24h `HH:MM`, IANA zone) into the correct UTC instant; surrounding spaces are ignored. | `feature/parse-start-input` |
| `TestParseStartRejectsBadInput` | Unknown or empty time zone, wrong date format, impossible date, 12h time, or out-of-range hour returns an error. An empty zone must not silently become UTC. | `feature/parse-start-input` |
| `TestParseStartHandlesDaylightSaving` | Uses the UTC offset in effect on the raid's date, across 2026 DST changes in `Europe/London` (Mar 29, Oct 25) and `America/New_York` (Nov 1). Passed without code changes (Go's tz database handles it); verified it fails if `ParseStart` used a fixed offset. | `feature/daylight-saving` |
| `TestJoinRaid` | After scheduling, `Raid.Join` adds members up to 6; the 7th fails with `ErrFull` and leaves the fireteam unchanged; an existing member (including the leader) re-joining fails with `ErrDuplicateMember`. Since `Schedule` no longer takes a member list, this is now the only place the 6-player cap and the no-duplicates rule are enforced. | `feature/join-raid` |
| `TestLeaveRaid` | `Raid.Leave` removes a member and frees their slot for someone else; a non-member leaving fails with `ErrNotMember`; the leader leaving fails with `ErrLeaderCannotLeave` and stays in the fireteam. | `feature/leave-raid` |

## internal/config

| Test | Requirement | Branch |
|---|---|---|
| `TestLoadPriority` | Precedence is real environment variable > `.env` (local, git-ignored) > `.env.development` (committed defaults). | `feature/env-config` |
| `TestLoadDoesNotModifyEnvironment` | Values read from env files are not written into the process environment. | `feature/env-config` |
| `TestLoadRequiresDatabaseURL` | No `DATABASE_URL` anywhere fails with `ErrMissingDatabaseURL`; missing env files alone are not an error (CI/production have none). | `feature/env-config` |
| `TestDevelopmentEnvMatchesCompose` | The committed `.env.development` `DATABASE_URL` uses the same user, password and database as `POSTGRES_*` (which `docker-compose.yml` creates the container with) and points at `localhost:5432`. | `feature/env-config` |

## internal/database

These tests need Docker running. `dbtest.Run` (called from `TestMain`) starts one `postgres:18` container per test package via testcontainers-go and applies every migration from empty; each test then calls `dbtest.Pool(t)`, which `TRUNCATE`s every table except `goose_db_version` first.

| Test | Requirement | Branch |
|---|---|---|
| `TestMigrationsCreateRaidSchema` | `raids.id` defaults to a UUIDv7; inserting the same `(raid_id, user_id)` into `raid_members` twice is a unique violation (23505); deleting a raid deletes its members. | `feature/database-migrations` |
| `TestMigrationsDownAndUp` | Every migration rolls back to version 0 (tables gone) and re-applies; `CheckSchema` returns `ErrSchemaBehind` on a database that's behind and nil when current (what the bot checks on startup). | `feature/database-migrations` |
| `TestPoolResetsBetweenTests` | `dbtest.Pool` empties all tables but keeps the migrated schema. | `feature/database-migrations` |
