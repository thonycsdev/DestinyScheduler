package raid_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anthonycoutinho/destinyscheduler/internal/database/dbtest"
	domain "github.com/anthonycoutinho/destinyscheduler/internal/raid"
	raidrepo "github.com/anthonycoutinho/destinyscheduler/internal/repository/raid"
)

func TestMain(m *testing.M) { os.Exit(dbtest.Run(m)) }

var start = time.Date(2026, 10, 20, 21, 0, 0, 0, time.UTC)

// setup returns a repository on a freshly emptied database.
func setup(t *testing.T) (*raidrepo.Repository, *pgxpool.Pool) {
	t.Helper()
	pool := dbtest.Pool(t)
	return raidrepo.New(pool), pool
}

// create schedules and stores a raid led by "leader-1", plus any extra members.
func create(t *testing.T, repo *raidrepo.Repository, startsAt time.Time, members ...string) domain.Raid {
	t.Helper()
	r, err := domain.Schedule("Last Wish", startsAt, "leader-1", startsAt.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	created, err := repo.Create(context.Background(), r)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	for _, m := range members {
		if created, err = repo.Join(context.Background(), created.ID, m); err != nil {
			t.Fatalf("Join(%q): %v", m, err)
		}
	}
	return created
}

func memberCount(t *testing.T, pool *pgxpool.Pool, id string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM raid_members WHERE raid_id = $1", id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestCreateAndGet ensures a stored raid comes back exactly as scheduled:
// same raid, start time in UTC, leader as the only member, and a database
// assigned ID.
func TestCreateAndGet(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	created := create(t, repo, start)
	if created.ID == "" {
		t.Fatal("Create returned an empty ID")
	}

	got, err := repo.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != created.ID || got.Name != "Last Wish" || got.LeaderID != "leader-1" {
		t.Errorf("Get = %+v, want ID %s, Last Wish, leader-1", got, created.ID)
	}
	if !got.StartsAt.Equal(start) || got.StartsAt.Location() != time.UTC {
		t.Errorf("StartsAt = %v (%v), want %v in UTC", got.StartsAt, got.StartsAt.Location(), start)
	}
	if want := []string{"leader-1"}; !slices.Equal(got.Members, want) {
		t.Errorf("Members = %v, want %v", got.Members, want)
	}
	if got.ChannelID != "" || got.MessageID != "" {
		t.Errorf("ChannelID/MessageID = %q/%q, want empty", got.ChannelID, got.MessageID)
	}
}

// TestGetUnknownRaid ensures a missing raid, including a malformed ID such as
// one from a stale or tampered Discord button, reports ErrNotFound.
func TestGetUnknownRaid(t *testing.T) {
	repo, _ := setup(t)
	for _, id := range []string{"0199f3a0-0000-7000-8000-000000000000", "not-a-uuid", ""} {
		if _, err := repo.Get(context.Background(), id); !errors.Is(err, raidrepo.ErrNotFound) {
			t.Errorf("Get(%q): err = %v, want ErrNotFound", id, err)
		}
	}
}

// TestJoinAndLeavePersist ensures joins and leaves are saved in join order
// and that the domain rules (no duplicates, leader can't leave) apply.
func TestJoinAndLeavePersist(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	r := create(t, repo, start, "u2", "u3")

	got, err := repo.Get(ctx, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"leader-1", "u2", "u3"}; !slices.Equal(got.Members, want) {
		t.Errorf("after joins, Members = %v, want %v", got.Members, want)
	}

	left, err := repo.Leave(ctx, r.ID, "u2")
	if err != nil {
		t.Fatalf("Leave: %v", err)
	}
	if want := []string{"leader-1", "u3"}; !slices.Equal(left.Members, want) {
		t.Errorf("Leave returned Members = %v, want %v", left.Members, want)
	}
	got, _ = repo.Get(ctx, r.ID)
	if want := []string{"leader-1", "u3"}; !slices.Equal(got.Members, want) {
		t.Errorf("after leave, stored Members = %v, want %v", got.Members, want)
	}

	if _, err := repo.Join(ctx, r.ID, "u3"); !errors.Is(err, domain.ErrDuplicateMember) {
		t.Errorf("Join(existing): err = %v, want ErrDuplicateMember", err)
	}
	if _, err := repo.Leave(ctx, r.ID, "leader-1"); !errors.Is(err, domain.ErrLeaderCannotLeave) {
		t.Errorf("Leave(leader): err = %v, want ErrLeaderCannotLeave", err)
	}
	if _, err := repo.Leave(ctx, r.ID, "u9"); !errors.Is(err, domain.ErrNotMember) {
		t.Errorf("Leave(non-member): err = %v, want ErrNotMember", err)
	}
	if _, err := repo.Join(ctx, "0199f3a0-0000-7000-8000-000000000000", "u4"); !errors.Is(err, raidrepo.ErrNotFound) {
		t.Errorf("Join(unknown raid): err = %v, want ErrNotFound", err)
	}
	if _, err := repo.Leave(ctx, "not-a-uuid", "u3"); !errors.Is(err, raidrepo.ErrNotFound) {
		t.Errorf("Leave(malformed id): err = %v, want ErrNotFound", err)
	}
}

// TestJoinFullRaid ensures the 6-player cap holds in the database.
func TestJoinFullRaid(t *testing.T) {
	repo, pool := setup(t)
	r := create(t, repo, start, "u2", "u3", "u4", "u5", "u6")

	if _, err := repo.Join(context.Background(), r.ID, "u7"); !errors.Is(err, domain.ErrFull) {
		t.Errorf("Join as 7th: err = %v, want ErrFull", err)
	}
	if n := memberCount(t, pool, r.ID); n != domain.FireteamSize {
		t.Errorf("stored members = %d, want %d", n, domain.FireteamSize)
	}
}

// TestConcurrentJoins ensures simultaneous Join clicks can't overfill a raid:
// with 5 members and 10 players joining at once, exactly one gets in.
func TestConcurrentJoins(t *testing.T) {
	repo, pool := setup(t)
	r := create(t, repo, start, "u2", "u3", "u4", "u5")

	var wg sync.WaitGroup
	errs := make([]error, 10)
	for i := range errs {
		wg.Go(func() {
			_, errs[i] = repo.Join(context.Background(), r.ID, fmt.Sprintf("racer-%d", i))
		})
	}
	wg.Wait()

	var joined, full int
	for i, err := range errs {
		switch {
		case err == nil:
			joined++
		case errors.Is(err, domain.ErrFull):
			full++
		default:
			t.Errorf("racer-%d: unexpected error %v", i, err)
		}
	}
	if joined != 1 || full != 9 {
		t.Errorf("joined = %d, full = %d; want 1 and 9", joined, full)
	}
	if n := memberCount(t, pool, r.ID); n != domain.FireteamSize {
		t.Errorf("stored members = %d, want %d", n, domain.FireteamSize)
	}
}

// TestSetMessage ensures the Discord channel and message of a posted raid are
// saved, so its buttons can be updated after a restart.
func TestSetMessage(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	r := create(t, repo, start)

	if err := repo.SetMessage(ctx, r.ID, "channel-1", "message-1"); err != nil {
		t.Fatalf("SetMessage: %v", err)
	}
	got, err := repo.Get(ctx, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ChannelID != "channel-1" || got.MessageID != "message-1" {
		t.Errorf("ChannelID/MessageID = %q/%q, want channel-1/message-1", got.ChannelID, got.MessageID)
	}
	if err := repo.SetMessage(ctx, "not-a-uuid", "c", "m"); !errors.Is(err, raidrepo.ErrNotFound) {
		t.Errorf("SetMessage(unknown): err = %v, want ErrNotFound", err)
	}
}

// TestDelete ensures cancelling a raid removes it and its fireteam.
func TestDelete(t *testing.T) {
	repo, pool := setup(t)
	ctx := context.Background()
	r := create(t, repo, start, "u2")

	if err := repo.Delete(ctx, r.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.Get(ctx, r.ID); !errors.Is(err, raidrepo.ErrNotFound) {
		t.Errorf("Get after Delete: err = %v, want ErrNotFound", err)
	}
	if n := memberCount(t, pool, r.ID); n != 0 {
		t.Errorf("members after Delete = %d, want 0", n)
	}
	if err := repo.Delete(ctx, r.ID); !errors.Is(err, raidrepo.ErrNotFound) {
		t.Errorf("Delete twice: err = %v, want ErrNotFound", err)
	}
}

// TestListUpcoming ensures only raids starting after now are listed, soonest
// first, each with its fireteam.
func TestListUpcoming(t *testing.T) {
	repo, _ := setup(t)
	now := start
	create(t, repo, now.Add(-time.Hour), "past")
	later := create(t, repo, now.Add(2*time.Hour))
	sooner := create(t, repo, now.Add(time.Hour), "u2")

	got, err := repo.ListUpcoming(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, r := range got {
		ids = append(ids, r.ID)
	}
	if want := []string{sooner.ID, later.ID}; !slices.Equal(ids, want) {
		t.Fatalf("ListUpcoming IDs = %v, want %v (soonest first, past excluded)", ids, want)
	}
	if want := []string{"leader-1", "u2"}; !slices.Equal(got[0].Members, want) {
		t.Errorf("first raid Members = %v, want %v", got[0].Members, want)
	}
}
