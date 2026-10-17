# Test catalog

Every test in the repo, the requirement it protects, and the branch that introduced it. Keep this in sync when adding or changing tests.

Run one with `go test ./internal/raid -run <TestName> -v`.

## internal/raid

| Test | Requirement | Branch |
|---|---|---|
| `TestScheduleRaid` | A scheduled raid contains the raid name, the fireteam members (in order), and the start time converted to UTC. 21:00 `America/Sao_Paulo` must become 00:00 UTC the next day. | `feature/schedule-raid` |
| `TestScheduleRaidRejectsMoreThanSixMembers` | Scheduling with more than 6 members fails with `ErrFull` (Destiny 2 fireteam limit). | `feature/schedule-raid` |
| `TestScheduleRaidRejectsUnknownRaid` | Only raids in `raid.Names` can be scheduled; an unknown or empty name fails with `ErrUnknownRaid`. | `feature/known-raids` |
| `TestScheduleRaidAcceptsEveryKnownRaid` | Every raid in `raid.Names` is accepted, so the list and the validation stay in sync. | `feature/known-raids` |
| `TestScheduleRaidRequiresAtLeastOneMember` | Scheduling with a nil or empty member list fails with `ErrNoMembers`. | `feature/require-members` |
| `TestScheduleRaidRejectsDuplicateMembers` | A member listed twice fails with `ErrDuplicateMember`, so one player can't take two slots. | `feature/unique-members` |
| `TestScheduleRaidRejectsPastTimes` | A start time at or before `now` fails with `ErrInPast`. `Schedule` takes `now` explicitly; tests pass the fixed `testNow` (2026-10-17 12:00 UTC). | `feature/reject-past-times` |
| `TestParseStart` | `ParseStart(date, clock, timezone)` turns user input (`YYYY-MM-DD`, 24h `HH:MM`, IANA zone) into the correct UTC instant; surrounding spaces are ignored. | `feature/parse-start-input` |
| `TestParseStartRejectsBadInput` | Unknown or empty time zone, wrong date format, impossible date, 12h time, or out-of-range hour returns an error. An empty zone must not silently become UTC. | `feature/parse-start-input` |
