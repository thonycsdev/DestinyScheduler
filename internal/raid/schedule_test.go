package raid

import (
	"errors"
	"slices"
	"testing"
	"time"
)

// TestScheduleRaid covers the core requirement: a scheduled raid contains the
// raid in question, the fireteam members, and the start time stored in UTC
// (the "universal time" every member's local time is derived from).
func TestScheduleRaid(t *testing.T) {
	saoPaulo, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	// 21:00 in São Paulo (UTC-3) is 00:00 UTC the next day.
	start := time.Date(2026, 10, 20, 21, 0, 0, 0, saoPaulo)
	members := []string{"Anthony", "Cayde", "Ikora", "Zavala", "Saint", "Osiris"}

	r, err := Schedule("Last Wish", start, members)
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	t.Run("contains the raid", func(t *testing.T) {
		if r.Name != "Last Wish" {
			t.Errorf("Name = %q, want %q", r.Name, "Last Wish")
		}
	})

	t.Run("contains the fireteam members", func(t *testing.T) {
		if !slices.Equal(r.Members, members) {
			t.Errorf("Members = %v, want %v", r.Members, members)
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
}

// TestScheduleRaidRejectsMoreThanSixMembers enforces the Destiny 2 raid
// fireteam limit of 6 players at scheduling time.
func TestScheduleRaidRejectsMoreThanSixMembers(t *testing.T) {
	members := []string{"Anthony", "Cayde", "Ikora", "Zavala", "Saint", "Osiris", "Eris"}

	_, err := Schedule("Last Wish", time.Date(2026, 10, 20, 21, 0, 0, 0, time.UTC), members)
	if !errors.Is(err, ErrFull) {
		t.Errorf("Schedule with 7 members: err = %v, want ErrFull", err)
	}
}

// TestScheduleRaidRejectsUnknownRaid ensures only real Destiny 2 raids (the
// Names list) can be scheduled, so typos and made-up raids are caught early.
func TestScheduleRaidRejectsUnknownRaid(t *testing.T) {
	for _, name := range []string{"Raid of Mars", ""} {
		_, err := Schedule(name, time.Date(2026, 10, 20, 21, 0, 0, 0, time.UTC), []string{"Anthony"})
		if !errors.Is(err, ErrUnknownRaid) {
			t.Errorf("Schedule(%q): err = %v, want ErrUnknownRaid", name, err)
		}
	}
}

// TestScheduleRaidAcceptsEveryKnownRaid ensures every raid in Names can be
// scheduled, so the list and the validation can't drift apart.
func TestScheduleRaidAcceptsEveryKnownRaid(t *testing.T) {
	for _, name := range Names {
		if _, err := Schedule(name, time.Date(2026, 10, 20, 21, 0, 0, 0, time.UTC), []string{"Anthony"}); err != nil {
			t.Errorf("Schedule(%q): %v", name, err)
		}
	}
}

// TestScheduleRaidRequiresAtLeastOneMember ensures a raid can't be scheduled
// with an empty fireteam; at minimum the person scheduling it is a member.
func TestScheduleRaidRequiresAtLeastOneMember(t *testing.T) {
	for _, members := range [][]string{nil, {}} {
		_, err := Schedule("Last Wish", time.Date(2026, 10, 20, 21, 0, 0, 0, time.UTC), members)
		if !errors.Is(err, ErrNoMembers) {
			t.Errorf("Schedule with members %#v: err = %v, want ErrNoMembers", members, err)
		}
	}
}
