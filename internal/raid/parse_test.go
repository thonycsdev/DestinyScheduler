package raid

import (
	"testing"
	"time"
)

// TestParseStart covers turning what a user types (date, 24h time, IANA time
// zone) into the raid's absolute start time in UTC.
func TestParseStart(t *testing.T) {
	cases := []struct {
		date, clock, timezone string
		want                  time.Time
	}{
		{"2026-10-20", "21:00", "America/Sao_Paulo", time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)},
		{"2026-10-20", "21:00", "Asia/Tokyo", time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)},
		{"2026-10-20", "21:00", "UTC", time.Date(2026, 10, 20, 21, 0, 0, 0, time.UTC)},
		{" 2026-10-20 ", " 09:30 ", " Europe/Berlin ", time.Date(2026, 10, 20, 7, 30, 0, 0, time.UTC)},
	}
	for _, c := range cases {
		t.Run(c.timezone, func(t *testing.T) {
			got, err := ParseStart(c.date, c.clock, c.timezone)
			if err != nil {
				t.Fatalf("ParseStart(%q, %q, %q): %v", c.date, c.clock, c.timezone, err)
			}
			if got.Location() != time.UTC {
				t.Errorf("location = %v, want UTC", got.Location())
			}
			if !got.Equal(c.want) {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

// TestParseStartRejectsBadInput ensures malformed user input produces an
// error instead of a wrong raid time.
func TestParseStartRejectsBadInput(t *testing.T) {
	cases := []struct{ name, date, clock, timezone string }{
		{"unknown timezone", "2026-10-20", "21:00", "Mars/Olympus"},
		{"empty timezone", "2026-10-20", "21:00", ""},
		{"wrong date format", "20/10/2026", "21:00", "UTC"},
		{"impossible date", "2026-02-30", "21:00", "UTC"},
		{"12h time", "2026-10-20", "9pm", "UTC"},
		{"hour out of range", "2026-10-20", "25:00", "UTC"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got, err := ParseStart(c.date, c.clock, c.timezone); err == nil {
				t.Errorf("ParseStart(%q, %q, %q) = %v, want error", c.date, c.clock, c.timezone, got)
			}
		})
	}
}

// TestParseStartHandlesDaylightSaving ensures the UTC offset used is the one
// in effect on the raid's date, not today's, around DST changes. 2026 dates:
// London switches on Mar 29 and Oct 25; New York falls back on Nov 1.
func TestParseStartHandlesDaylightSaving(t *testing.T) {
	cases := []struct {
		name, date, timezone string
		want                 time.Time
	}{
		{"London before spring forward (GMT)", "2026-03-28", "Europe/London", time.Date(2026, 3, 28, 21, 0, 0, 0, time.UTC)},
		{"London after spring forward (BST)", "2026-03-29", "Europe/London", time.Date(2026, 3, 29, 20, 0, 0, 0, time.UTC)},
		{"London before fall back (BST)", "2026-10-24", "Europe/London", time.Date(2026, 10, 24, 20, 0, 0, 0, time.UTC)},
		{"London after fall back (GMT)", "2026-10-25", "Europe/London", time.Date(2026, 10, 25, 21, 0, 0, 0, time.UTC)},
		{"New York before fall back (EDT)", "2026-10-31", "America/New_York", time.Date(2026, 11, 1, 1, 0, 0, 0, time.UTC)},
		{"New York after fall back (EST)", "2026-11-01", "America/New_York", time.Date(2026, 11, 2, 2, 0, 0, 0, time.UTC)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseStart(c.date, "21:00", c.timezone)
			if err != nil {
				t.Fatal(err)
			}
			if !got.Equal(c.want) {
				t.Errorf("21:00 on %s in %s = %v, want %v", c.date, c.timezone, got, c.want)
			}
		})
	}
}
