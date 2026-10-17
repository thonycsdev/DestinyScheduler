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
