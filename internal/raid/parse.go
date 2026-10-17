package raid

import (
	"fmt"
	"strings"
	"time"
	// Embed the tz database so time zones load in minimal container images
	// (distroless) that have no /usr/share/zoneinfo.
	_ "time/tzdata"
)

// ParseStart interprets user input — a date (YYYY-MM-DD), a 24h time (HH:MM)
// and an IANA time zone (e.g. America/Sao_Paulo) — and returns the start time
// in UTC.
func ParseStart(date, clock, timezone string) (time.Time, error) {
	timezone = strings.TrimSpace(timezone)
	// LoadLocation("") returns UTC, which would silently hide a missing zone.
	if timezone == "" {
		return time.Time{}, fmt.Errorf("missing time zone (use an IANA name like America/Sao_Paulo)")
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, fmt.Errorf("unknown time zone %q (use an IANA name like America/Sao_Paulo)", timezone)
	}
	t, err := time.ParseInLocation("2006-01-02 15:04", strings.TrimSpace(date)+" "+strings.TrimSpace(clock), loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("couldn't read %q %q (use YYYY-MM-DD and 24h HH:MM)", date, clock)
	}
	return t.UTC(), nil
}
