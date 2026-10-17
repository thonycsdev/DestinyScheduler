package raid

import (
	"errors"
	"slices"
	"testing"
	"time"
)

// testNow is the fixed "current time" passed to Schedule so tests don't depend
// on the wall clock. Raids in these tests start after it.
var testNow = time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)

// testStart is a valid raid start time after testNow.
var testStart = time.Date(2026, 10, 20, 21, 0, 0, 0, time.UTC)

// TestScheduleRaid covers the core requirement: a scheduled raid contains the
// raid in question, its leader as the first (and only) fireteam member, and
// the start time stored in UTC (the "universal time" every member's local
// time is derived from).
func TestScheduleRaid(t *testing.T) {
	saoPaulo, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	// 21:00 in São Paulo (UTC-3) is 00:00 UTC the next day.
	start := time.Date(2026, 10, 20, 21, 0, 0, 0, saoPaulo)

	r, err := Schedule("Last Wish", start, "leader-1", testNow)
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	t.Run("contains the raid", func(t *testing.T) {
		if r.Name != "Last Wish" {
			t.Errorf("Name = %q, want %q", r.Name, "Last Wish")
		}
	})

	t.Run("leader is the only member", func(t *testing.T) {
		if r.LeaderID != "leader-1" {
			t.Errorf("LeaderID = %q, want %q", r.LeaderID, "leader-1")
		}
		if want := []string{"leader-1"}; !slices.Equal(r.Members, want) {
			t.Errorf("Members = %v, want %v", r.Members, want)
		}
	})

	t.Run("time is stored in UTC", func(t *testing.T) {
		if r.StartsAt.Location() != time.UTC {
			t.Errorf("StartsAt location = %v, want UTC", r.StartsAt.Location())
		}
		want := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
		if !r.StartsAt.Equal(want) {
			t.Errorf("StartsAt = %v, want %v", r.StartsAt, want)
		}
	})

	t.Run("not yet persisted or posted", func(t *testing.T) {
		if r.ID != "" || r.ChannelID != "" || r.MessageID != "" {
			t.Errorf("ID/ChannelID/MessageID = %q/%q/%q, want all empty", r.ID, r.ChannelID, r.MessageID)
		}
	})
}

// TestScheduleRaidRequiresLeader ensures every raid has a leader: the person
// scheduling it, who is always part of the fireteam.
func TestScheduleRaidRequiresLeader(t *testing.T) {
	_, err := Schedule("Last Wish", testStart, "", testNow)
	if !errors.Is(err, ErrNoLeader) {
		t.Errorf("Schedule with empty leader: err = %v, want ErrNoLeader", err)
	}
}

// TestScheduleRaidRejectsUnknownRaid ensures only real Destiny 2 raids (the
// Names list) can be scheduled, so typos and made-up raids are caught early.
func TestScheduleRaidRejectsUnknownRaid(t *testing.T) {
	for _, name := range []string{"Raid of Mars", ""} {
		_, err := Schedule(name, testStart, "leader-1", testNow)
		if !errors.Is(err, ErrUnknownRaid) {
			t.Errorf("Schedule(%q): err = %v, want ErrUnknownRaid", name, err)
		}
	}
}

// TestScheduleRaidAcceptsEveryKnownRaid ensures every raid in Names can be
// scheduled, so the list and the validation can't drift apart.
func TestScheduleRaidAcceptsEveryKnownRaid(t *testing.T) {
	for _, name := range Names {
		if _, err := Schedule(name, testStart, "leader-1", testNow); err != nil {
			t.Errorf("Schedule(%q): %v", name, err)
		}
	}
}

// TestScheduleRaidRejectsPastTimes ensures a raid can't be scheduled at or
// before the current time.
func TestScheduleRaidRejectsPastTimes(t *testing.T) {
	for _, start := range []time.Time{testNow.Add(-time.Hour), testNow} {
		_, err := Schedule("Last Wish", start, "leader-1", testNow)
		if !errors.Is(err, ErrInPast) {
			t.Errorf("Schedule at %v (now %v): err = %v, want ErrInPast", start, testNow, err)
		}
	}
}

// TestJoinRaid covers members joining after the raid is scheduled: players can
// join until the fireteam has 6, the 7th is refused with ErrFull, and someone
// already in the fireteam (including the leader) can't join again.
func TestJoinRaid(t *testing.T) {
	r, err := Schedule("Last Wish", testStart, "leader-1", testNow)
	if err != nil {
		t.Fatal(err)
	}

	if err := r.Join("leader-1", testNow); !errors.Is(err, ErrDuplicateMember) {
		t.Errorf("Join(leader): err = %v, want ErrDuplicateMember", err)
	}

	for _, m := range []string{"u2", "u3", "u4", "u5", "u6"} {
		if err := r.Join(m, testNow); err != nil {
			t.Fatalf("Join(%q) with %d members: %v", m, len(r.Members), err)
		}
	}
	want := []string{"leader-1", "u2", "u3", "u4", "u5", "u6"}
	if !slices.Equal(r.Members, want) {
		t.Errorf("Members = %v, want %v", r.Members, want)
	}

	if err := r.Join("u2", testNow); !errors.Is(err, ErrDuplicateMember) {
		t.Errorf("Join(existing member): err = %v, want ErrDuplicateMember", err)
	}
	if err := r.Join("u7", testNow); !errors.Is(err, ErrFull) {
		t.Errorf("Join as 7th member: err = %v, want ErrFull", err)
	}
	if len(r.Members) != FireteamSize {
		t.Errorf("after refused join, len(Members) = %d, want %d", len(r.Members), FireteamSize)
	}
}

// TestLeaveRaid covers members leaving a scheduled raid: a member's slot is
// freed (so someone else can take it), someone not in the fireteam can't
// leave, and the leader can't leave (they cancel the raid instead).
func TestLeaveRaid(t *testing.T) {
	r, err := Schedule("Last Wish", testStart, "leader-1", testNow)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{"u2", "u3", "u4", "u5", "u6"} {
		if err := r.Join(m, testNow); err != nil {
			t.Fatal(err)
		}
	}

	if err := r.Leave("u3", testNow); err != nil {
		t.Fatalf("Leave(member): %v", err)
	}
	if want := []string{"leader-1", "u2", "u4", "u5", "u6"}; !slices.Equal(r.Members, want) {
		t.Errorf("Members = %v, want %v", r.Members, want)
	}
	if err := r.Join("u7", testNow); err != nil {
		t.Errorf("Join after a slot was freed: %v", err)
	}

	if err := r.Leave("u3", testNow); !errors.Is(err, ErrNotMember) {
		t.Errorf("Leave(non-member): err = %v, want ErrNotMember", err)
	}
	if err := r.Leave("leader-1", testNow); !errors.Is(err, ErrLeaderCannotLeave) {
		t.Errorf("Leave(leader): err = %v, want ErrLeaderCannotLeave", err)
	}
	if !slices.Contains(r.Members, "leader-1") {
		t.Error("leader was removed after a refused Leave")
	}
}

// TestJoinAndLeaveRefusedAfterStart ensures the fireteam is locked once the
// raid starts: from the start time on, Join and Leave fail with ErrStarted
// and the fireteam is unchanged.
func TestJoinAndLeaveRefusedAfterStart(t *testing.T) {
	r, err := Schedule("Last Wish", testStart, "leader-1", testNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Join("u2", testNow); err != nil {
		t.Fatal(err)
	}

	for _, now := range []time.Time{testStart, testStart.Add(time.Minute)} {
		if err := r.Join("u3", now); !errors.Is(err, ErrStarted) {
			t.Errorf("Join at %v (start %v): err = %v, want ErrStarted", now, testStart, err)
		}
		if err := r.Leave("u2", now); !errors.Is(err, ErrStarted) {
			t.Errorf("Leave at %v (start %v): err = %v, want ErrStarted", now, testStart, err)
		}
	}
	if want := []string{"leader-1", "u2"}; !slices.Equal(r.Members, want) {
		t.Errorf("Members = %v, want %v", r.Members, want)
	}

	if err := r.Join("u3", testStart.Add(-time.Second)); err != nil {
		t.Errorf("Join one second before start: %v", err)
	}
}
