// Package raid holds the Destiny 2 raid scheduling domain, independent of Discord.
package raid

import (
	"errors"
	"slices"
	"time"
)

// FireteamSize is the maximum number of Guardians in a raid fireteam.
const FireteamSize = 6

var ErrFull = errors.New("fireteam is full (max 6 members)")

// Raid is a scheduled raid and its fireteam.
type Raid struct {
	Name     string
	StartsAt time.Time // always UTC
	Members  []string
}

// Schedule creates a raid for the given fireteam. The start time may be in any
// time zone; it is stored in UTC.
func Schedule(name string, start time.Time, members []string) (Raid, error) {
	if len(members) > FireteamSize {
		return Raid{}, ErrFull
	}
	return Raid{
		Name:     name,
		StartsAt: start.UTC(),
		Members:  slices.Clone(members),
	}, nil
}
