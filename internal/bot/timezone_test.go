package bot

import (
	"slices"
	"testing"
	"time"
)

func choiceValues(query string) []string {
	var out []string
	for _, c := range TimezoneChoices(query) {
		out = append(out, c.Value.(string))
		if c.Name != c.Value {
			panic("choice name and value differ: " + c.Name)
		}
	}
	return out
}

// TestTimezoneChoicesMatchCities ensures typing part of a city finds its zone,
// ignoring case, spaces vs underscores, and accents (São Paulo).
func TestTimezoneChoicesMatchCities(t *testing.T) {
	cases := map[string]string{
		"sao":       "America/Sao_Paulo",
		"São Paulo": "America/Sao_Paulo",
		"LONDON":    "Europe/London",
		"new york":  "America/New_York",
		"tokyo":     "Asia/Tokyo",
		"utc":       "UTC",
	}
	for query, want := range cases {
		if got := choiceValues(query); !slices.Contains(got, want) {
			t.Errorf("TimezoneChoices(%q) = %v, want it to include %s", query, got, want)
		}
	}
}

// TestTimezoneChoicesRankCityPrefixFirst ensures a zone whose city starts with
// the query is suggested before zones that merely contain it.
func TestTimezoneChoicesRankCityPrefixFirst(t *testing.T) {
	got := choiceValues("paris")
	if len(got) == 0 || got[0] != "Europe/Paris" {
		t.Errorf("TimezoneChoices(paris) = %v, want Europe/Paris first", got)
	}
	got = choiceValues("york")
	if len(got) == 0 || got[0] != "America/New_York" {
		t.Errorf("TimezoneChoices(york) = %v, want America/New_York first", got)
	}
}

// TestTimezoneChoicesLimit ensures Discord's 25-choice autocomplete limit is
// respected, including for an empty query (which suggests common zones).
func TestTimezoneChoicesLimit(t *testing.T) {
	for _, q := range []string{"", "a"} {
		got := choiceValues(q)
		if len(got) == 0 || len(got) > 25 {
			t.Errorf("TimezoneChoices(%q) returned %d choices, want 1-25", q, len(got))
		}
	}
	if got := choiceValues(""); !slices.Contains(got, "UTC") || !slices.Contains(got, "America/Sao_Paulo") {
		t.Errorf("TimezoneChoices(\"\") = %v, want common zones including UTC and America/Sao_Paulo", got)
	}
	if got := choiceValues("zzzz"); len(got) != 0 {
		t.Errorf("TimezoneChoices(zzzz) = %v, want none", got)
	}
}

// TestTimezoneChoicesAreLoadable ensures every suggestion (and every common
// default) is a zone ParseStart accepts, including in a container with no
// system tz database (internal/raid embeds time/tzdata).
func TestTimezoneChoicesAreLoadable(t *testing.T) {
	for _, z := range append(slices.Clone(zones), commonZones...) {
		if _, err := time.LoadLocation(z); err != nil {
			t.Errorf("zone %q doesn't load: %v", z, err)
		}
	}
}
