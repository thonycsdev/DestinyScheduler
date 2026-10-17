# Test catalog

Every test in the repo, the requirement it protects, and the branch that introduced it. Keep this in sync when adding or changing tests.

Run one with `go test ./internal/raid -run <TestName> -v`.

## internal/raid

| Test | Requirement | Branch |
|---|---|---|
| `TestScheduleRaid` | A scheduled raid contains the raid name, the fireteam members (in order), and the start time converted to UTC. 21:00 `America/Sao_Paulo` must become 00:00 UTC the next day. | `feature/schedule-raid` |
| `TestScheduleRaidRejectsMoreThanSixMembers` | Scheduling with more than 6 members fails with `ErrFull` (Destiny 2 fireteam limit). | `feature/schedule-raid` |
