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
| `TestJoinAndLeaveRefusedAfterStart` (raid) | From the start time on (`now >= StartsAt`), `Join` and `Leave` fail with `ErrStarted` and the fireteam is unchanged; one second before start still works. `Join`/`Leave` take `now`. | `feature/started-raids` |

## internal/config

| Test | Requirement | Branch |
|---|---|---|
| `TestLoadPriority` | Precedence is real environment variable > `.env` (local, git-ignored) > `.env.development` (committed defaults). | `feature/env-config` |
| `TestLoadDoesNotModifyEnvironment` | Values read from env files are not written into the process environment. | `feature/env-config` |
| `TestLoadRequiresDatabaseURL` | No `DATABASE_URL` anywhere fails with `ErrMissingDatabaseURL`; missing env files alone are not an error (CI/production have none). | `feature/env-config` |
| `TestDevelopmentEnvMatchesCompose` | The committed `.env.development` `DATABASE_URL` uses the same user, password and database as `POSTGRES_*` (which `docker-compose.yml` creates the container with) and points at `localhost:5432`. | `feature/env-config` |
| `TestLoadReadsDiscordSettings` | `DISCORD_TOKEN` and optional `DISCORD_GUILD_ID` are read with the same precedence; both are optional for `Load` (`cmd/bot` requires the token itself). | `feature/discord-adapter` |

## internal/database

These tests need Docker running. `dbtest.Run` (called from `TestMain`) starts one `postgres:18` container per test package via testcontainers-go and applies every migration from empty; each test then calls `dbtest.Pool(t)`, which `TRUNCATE`s every table except `goose_db_version` first.

| Test | Requirement | Branch |
|---|---|---|
| `TestMigrationsCreateRaidSchema` | `raids.id` defaults to a UUIDv7; inserting the same `(raid_id, user_id)` into `raid_members` twice is a unique violation (23505); deleting a raid deletes its members. | `feature/database-migrations` |
| `TestMigrationsDownAndUp` | Every migration rolls back to version 0 (tables gone) and re-applies; `CheckSchema` returns `ErrSchemaBehind` on a database that's behind and nil when current (what the bot checks on startup). | `feature/database-migrations` |
| `TestPoolResetsBetweenTests` | `dbtest.Pool` empties all tables but keeps the migrated schema. | `feature/database-migrations` |

## internal/repository/raid

Needs Docker (uses `dbtest`, see above). The package is named `raid`; tests import it as `raidrepo` and the domain package as `domain`.

| Test | Requirement | Branch |
|---|---|---|
| `TestCreateAndGet` | `Create` stores a scheduled raid and returns a DB-assigned ID; `Get` returns the same name, leader, members (leader only) and start time in UTC, with empty channel/message IDs. | `feature/raid-repository` |
| `TestGetUnknownRaid` | An unknown ID, a malformed ID (`not-a-uuid`) or an empty ID returns `ErrNotFound`, not a database error. | `feature/raid-repository` |
| `TestJoinAndLeavePersist` | `Join`/`Leave` are saved and keep join order; domain errors (`ErrDuplicateMember`, `ErrLeaderCannotLeave`, `ErrNotMember`) come through; unknown or malformed raid IDs give `ErrNotFound`. | `feature/raid-repository` |
| `TestJoinFullRaid` | A 7th `Join` returns `ErrFull` and the database still holds 6 members. | `feature/raid-repository` |
| `TestConcurrentJoins` | 10 simultaneous joins on a raid with 5 members: exactly 1 succeeds, 9 get `ErrFull`, 6 stored. Verified it fails (10 join, 15 stored) without the `SELECT ... FOR UPDATE` row lock. | `feature/raid-repository` |
| `TestSetMessage` | `SetMessage` saves the Discord channel and message IDs; an unknown raid returns `ErrNotFound`. | `feature/raid-repository` |
| `TestDelete` | `Delete` removes the raid and its members; deleting again returns `ErrNotFound`. | `feature/raid-repository` |
| `TestListUpcoming` | `ListUpcoming(now)` returns only raids starting after `now`, soonest first, each with its members. | `feature/raid-repository` |
| `TestJoinAndLeaveRefusedAfterStart` (repository) | `Repository.Join`/`Leave` take `now` and the domain's `ErrStarted` rule applies to stored raids; nothing is written. | `feature/started-raids` |

## internal/bot

Needs Docker: the package's `TestMain` runs `dbtest.Run`, because handlers are tested against the real raid repository (no mocks).

| Test | Requirement | Branch |
|---|---|---|
| `TestRaidMessageShowsTimeInEachViewersZone` | The raid message shows the start as Discord timestamps `<t:UNIX:F>` and `<t:UNIX:R>`, which each viewer sees in their own time zone (the README's core promise). Title is the raid name. | `feature/raid-message` |
| `TestRaidMessageListsFireteam` | The fireteam field is `Fireteam (n/6)` and lists `<@id>` mentions in join order, leader prefixed with 👑. | `feature/raid-message` |
| `TestRaidMessageButtons` | Buttons are Join, Leave, Cancel with IDs `raid:<action>:<raid id>`; Join is disabled when the fireteam is full. | `feature/raid-message` |
| `TestCancelledMessage` | A cancelled raid's title says "Cancelled" with the raid name, keeps the When field, and has an empty non-nil component list (so editing removes the buttons). | `feature/raid-message` |
| `TestParseCustomID` | `raid:<join\|leave\|cancel>:<id>` parses; empty, unknown action, other prefixes or missing ID are rejected. | `feature/raid-message` |
| `TestCreateRaid` | `/raid create` turns date + time + time zone into a stored raid led by the user (21:00 São Paulo → 00:00 UTC) and replies with the public raid message and its `RaidID`; `RecordMessage` saves the posted channel/message IDs. | `feature/bot-handlers` |
| `TestCreateRaidRejectsBadInput` | Unknown time zone, bad date format, past time or unknown raid gives a private explanation and stores nothing. | `feature/bot-handlers` |
| `TestJoinAndLeaveButtons` | Join and Leave are saved and reply with an in-place update of the raid message showing the new count. | `feature/bot-handlers` |
| `TestButtonErrorsArePrivate` | Already in, not in, leader leaving, raid gone, raid full, already started, and unrecognized button IDs each get a private message and no message update. | `feature/bot-handlers` |
| `TestCancelButton` | A non-leader without Manage Messages can't cancel (raid kept); the leader can, even after the start time; a moderator (`CanManageMessages`) can. Cancelling deletes the raid and updates the message to the cancelled version with no buttons. | `feature/bot-handlers` |
| `TestListRaids` | `/raid list` replies privately: "no upcoming raids" when empty, otherwise raids soonest first with Discord timestamps and `n/6`. | `feature/bot-handlers` |
| `TestTimezoneChoicesMatchCities` | Time zone autocomplete finds a zone from part of its city, ignoring case, spaces vs underscores and accents (`São Paulo` → `America/Sao_Paulo`). | `feature/timezone-autocomplete` |
| `TestTimezoneChoicesRankCityPrefixFirst` | Zones whose city starts with the query come first (`paris` → `Europe/Paris`). | `feature/timezone-autocomplete` |
| `TestTimezoneChoicesLimit` | At most 25 suggestions (Discord's limit); an empty query suggests common zones incl. `UTC` and `America/Sao_Paulo`; no match gives none. | `feature/timezone-autocomplete` |
| `TestTimezoneChoicesAreLoadable` | Every suggested zone (the generated `zones` list and `commonZones`) loads with `time.LoadLocation`. | `feature/timezone-autocomplete` |
| `TestCommandsDefinition` | `/raid` is guild-only, with subcommands `create` (required options raid → choices exactly `raid.Names`, date, time, timezone with autocomplete) and `list`. | `feature/discord-adapter` |
| `TestReadInteractions` | The adapter reads the subcommand, `/raid create` options and user ID, the focused autocomplete text, and a button click's custom ID and Manage Messages permission from discordgo payloads. | `feature/discord-adapter` |
| `TestInteractionResponse` | Responses map to a new message or an in-place update, ephemeral replies set the ephemeral flag, mentions never ping (empty allow list), and a cancelled update serializes `"components":[]` so the buttons are removed. | `feature/discord-adapter` |

## infra

Go tests over the CloudFormation templates (`infra/app.yml`, `infra/bootstrap.yml`). Templates must use long-form intrinsics (`{"Ref": ...}`, not `!Ref`) so they parse as plain YAML. `cfn-lint` runs separately (see CLAUDE.md).

| Test | Requirement | Branch |
|---|---|---|
| `TestAppRunsExactlyOneBot` | Never two bots (both would answer every click): ASG min/max/desired = 1, `CapacityRebalance: false`, rolling update `MinInstancesInService: 0`. Verified it fails with `CapacityRebalance: true`. | `feature/aws-infrastructure` |
| `TestAppUsesOnDemandGraviton` | One On-Demand `t4g.micro` (no `MixedInstancesPolicy` or Spot market options; the organization's SCP denies Spot), 2 subnets, AMI from the latest AL2023 arm64 SSM parameter, 8 GiB gp3 root volume. Replaced `TestAppUsesSpotGraviton`. | `feature/us-east-2-on-demand` |
| `TestDeploysToUsEast2` | The workflow deploys (`AWS_REGION`, `configure-aws-credentials`) and runs `cfn-lint` for `us-east-2`, and never mentions `us-east-1`; the SCP allows only us-east-2. | `feature/us-east-2-on-demand` |
| `TestAppAllowsNoInboundTraffic` | No ingress rules or `SecurityGroupIngress` resources, no NAT gateway, no SSH key, IMDSv2 required, instance role has `AmazonSSMManagedInstanceCore` (Session Manager). | `feature/aws-infrastructure` |
| `TestAppStorageLimits` | ECR lifecycle keeps the last 5 images; logs retained 14 days. | `feature/aws-infrastructure` |
| `TestAppNamesMatchStartScript` | The secrets, image-tag parameter, ECR repository and log group the template creates have exactly the names the instance script uses; the image tag starts as `none`. | `feature/aws-infrastructure` |
| `TestInstanceScriptsPassShellcheck` | The user data and the `destiny-bot-start` script it installs pass `shellcheck --severity=style`. The start script is linted on its own because shellcheck ignores quoted heredoc contents (found by a mutation check). Uses local shellcheck or Docker; required in CI. | `feature/aws-infrastructure` |
| `TestBootstrapTrustsOnlyThisRepo` | The deploy role trusts only GitHub's OIDC provider with exact `StringEquals`: aud `sts.amazonaws.com`, sub `repo:thonycsdev/DestinyScheduler:environment:PRD` (written out, not a parameter). | `feature/aws-infrastructure` |
| `TestBootstrapDeployRoleIsScoped` | The deploy role has no wildcard actions and can `iam:PassRole` only the CloudFormation execution role. | `feature/aws-infrastructure` |
