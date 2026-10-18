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
	ErrFull              = errors.New("fireteam is full (max 6 members)")
	ErrUnknownRaid       = errors.New("unknown raid")
	ErrNoLeader          = errors.New("a raid needs a leader")
	ErrDuplicateMember   = errors.New("already in the fireteam")
	ErrInPast            = errors.New("raid time is in the past")
	ErrNotMember         = errors.New("not in the fireteam")
	ErrLeaderCannotLeave = errors.New("the leader can't leave; cancel the raid instead")
	ErrStarted           = errors.New("this raid has already started")
)

// Raid is a scheduled raid and its fireteam.
type Raid struct {
	ID        string // assigned when stored; empty until then
	Name      string
	StartsAt  time.Time // always UTC
	LeaderID  string    // Discord user ID of whoever scheduled the raid
	Members   []string  // Discord user IDs in join order; the leader is first
	ChannelID string    // where the raid message was posted; empty until then
	MessageID string
}

// Schedule creates a raid led by leaderID, who becomes its first member. The
// start time may be in any time zone; it is stored in UTC. It must be after now.
func Schedule(name string, start time.Time, leaderID string, now time.Time) (Raid, error) {
	if !slices.Contains(Names, name) {
		return Raid{}, fmt.Errorf("%w: %q", ErrUnknownRaid, name)
	}
	if !start.After(now) {
		return Raid{}, ErrInPast
	}
	if leaderID == "" {
		return Raid{}, ErrNoLeader
	}
	return Raid{
		Name:     name,
		StartsAt: start.UTC(),
		LeaderID: leaderID,
		Members:  []string{leaderID},
	}, nil
}

// Started reports whether the raid has started at now.
func (r *Raid) Started(now time.Time) bool {
	return !now.Before(r.StartsAt)
}

// Join adds a member to the fireteam. The fireteam is locked once the raid
// has started.
func (r *Raid) Join(member string, now time.Time) error {
	if r.Started(now) {
		return ErrStarted
	}
	if slices.Contains(r.Members, member) {
		return fmt.Errorf("%w: %q", ErrDuplicateMember, member)
	}
	if len(r.Members) >= FireteamSize {
		return ErrFull
	}
	r.Members = append(r.Members, member)
	return nil
}

// Leave removes a member from the fireteam. The leader can't leave, and no
// one can once the raid has started.
func (r *Raid) Leave(member string, now time.Time) error {
	if r.Started(now) {
		return ErrStarted
	}
	if member == r.LeaderID {
		return ErrLeaderCannotLeave
	}
	i := slices.Index(r.Members, member)
	if i < 0 {
		return fmt.Errorf("%w: %q", ErrNotMember, member)
	}
	r.Members = slices.Delete(r.Members, i, i+1)
	return nil
}
