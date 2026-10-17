// Package raid holds the Destiny 2 raid scheduling domain, independent of Discord.
package raid

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// FireteamSize is the maximum number of Guardians in a raid fireteam.
const FireteamSize = 6

// Names lists the Destiny 2 raids that can be scheduled.
var Names = []string{
	"Last Wish",
	"Garden of Salvation",
	"Deep Stone Crypt",
	"Vault of Glass",
	"Vow of the Disciple",
	"King's Fall",
	"Root of Nightmares",
	"Crota's End",
	"Salvation's Edge",
	"The Desert Perpetual",
}

var (
	ErrFull        = errors.New("fireteam is full (max 6 members)")
	ErrUnknownRaid = errors.New("unknown raid")
	ErrNoMembers   = errors.New("a raid needs at least one member")
)

// Raid is a scheduled raid and its fireteam.
type Raid struct {
	Name     string
	StartsAt time.Time // always UTC
	Members  []string
}

// Schedule creates a raid for the given fireteam. The start time may be in any
// time zone; it is stored in UTC.
func Schedule(name string, start time.Time, members []string) (Raid, error) {
	if !slices.Contains(Names, name) {
		return Raid{}, fmt.Errorf("%w: %q", ErrUnknownRaid, name)
	}
	if len(members) == 0 {
		return Raid{}, ErrNoMembers
	}
	if len(members) > FireteamSize {
		return Raid{}, ErrFull
	}
	return Raid{
		Name:     name,
		StartsAt: start.UTC(),
		Members:  slices.Clone(members),
	}, nil
}
