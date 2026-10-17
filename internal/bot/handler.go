package bot

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	domain "github.com/anthonycoutinho/destinyscheduler/internal/raid"
	raidrepo "github.com/anthonycoutinho/destinyscheduler/internal/repository/raid"
)

// maxListed caps /raid list so the reply stays under Discord's 2000 characters.
const maxListed = 10

// Handler answers the bot's commands and buttons.
type Handler struct {
	Raids *raidrepo.Repository
	Now   func() time.Time
}

// Response is what the adapter sends back to Discord.
type Response struct {
	Content    string
	Embed      *discordgo.MessageEmbed
	Components []discordgo.MessageComponent
	// Ephemeral replies are seen only by the user who acted.
	Ephemeral bool
	// UpdateMessage edits the message the button is on instead of replying.
	UpdateMessage bool
	// RaidID is set when a raid message is posted; the adapter reports the
	// posted message back with RecordMessage.
	RaidID string
}

func private(content string) Response {
	return Response{Content: content, Ephemeral: true}
}

// userMessage explains a refused action to the user. ok is false for
// unexpected errors, which the adapter logs.
func userMessage(err error) (string, bool) {
	switch {
	case errors.Is(err, domain.ErrFull):
		return "This fireteam is full (6/6).", true
	case errors.Is(err, domain.ErrDuplicateMember):
		return "You're already in this fireteam.", true
	case errors.Is(err, domain.ErrNotMember):
		return "You're not in this fireteam.", true
	case errors.Is(err, domain.ErrLeaderCannotLeave):
		return "The leader can't leave the raid. Use Cancel instead.", true
	case errors.Is(err, domain.ErrStarted):
		return "This raid has already started.", true
	case errors.Is(err, domain.ErrInPast):
		return "That time is in the past. Pick a future date and time.", true
	case errors.Is(err, domain.ErrUnknownRaid):
		return "Unknown raid. Pick one from the list.", true
	case errors.Is(err, raidrepo.ErrNotFound):
		return "This raid no longer exists.", true
	}
	return "", false
}

// respondErr turns refused actions into private replies and passes anything
// unexpected up.
func respondErr(err error) (Response, error) {
	if msg, ok := userMessage(err); ok {
		return private(msg), nil
	}
	return Response{}, err
}

// CreateInput holds the /raid create options.
type CreateInput struct {
	UserID                     string
	Raid, Date, Time, Timezone string
}

// Create schedules a raid led by the user and returns the public raid message.
func (h *Handler) Create(ctx context.Context, in CreateInput) (Response, error) {
	start, err := domain.ParseStart(in.Date, in.Time, in.Timezone)
	if err != nil {
		return private(capitalize(err.Error()) + "."), nil
	}
	r, err := domain.Schedule(in.Raid, start, in.UserID, h.Now())
	if err != nil {
		return respondErr(err)
	}
	if r, err = h.Raids.Create(ctx, r); err != nil {
		return Response{}, err
	}
	embed, components := RaidMessage(r)
	return Response{Embed: embed, Components: components, RaidID: r.ID}, nil
}

// RecordMessage saves where a raid's message was posted.
func (h *Handler) RecordMessage(ctx context.Context, raidID, channelID, messageID string) error {
	return h.Raids.SetMessage(ctx, raidID, channelID, messageID)
}

// ButtonInput describes a click on a raid message button.
type ButtonInput struct {
	UserID   string
	CustomID string
	// CanManageMessages is the clicker's Manage Messages permission in the
	// channel; it lets moderators cancel raids they don't lead.
	CanManageMessages bool
}

// Button handles Join, Leave and Cancel.
func (h *Handler) Button(ctx context.Context, in ButtonInput) (Response, error) {
	action, raidID, ok := ParseCustomID(in.CustomID)
	if !ok {
		return private("This button isn't recognized."), nil
	}
	var r domain.Raid
	var err error
	switch action {
	case ActionJoin:
		r, err = h.Raids.Join(ctx, raidID, in.UserID, h.Now())
	case ActionLeave:
		r, err = h.Raids.Leave(ctx, raidID, in.UserID, h.Now())
	case ActionCancel:
		return h.cancel(ctx, raidID, in)
	}
	if err != nil {
		return respondErr(err)
	}
	embed, components := RaidMessage(r)
	return Response{Embed: embed, Components: components, UpdateMessage: true}, nil
}

func (h *Handler) cancel(ctx context.Context, raidID string, in ButtonInput) (Response, error) {
	r, err := h.Raids.Get(ctx, raidID)
	if err != nil {
		return respondErr(err)
	}
	if in.UserID != r.LeaderID && !in.CanManageMessages {
		return private("Only the raid leader or a moderator can cancel this raid."), nil
	}
	if err := h.Raids.Delete(ctx, raidID); err != nil {
		return respondErr(err)
	}
	embed, components := CancelledMessage(r)
	return Response{Embed: embed, Components: components, UpdateMessage: true}, nil
}

// List replies privately with upcoming raids, soonest first.
func (h *Handler) List(ctx context.Context) (Response, error) {
	raids, err := h.Raids.ListUpcoming(ctx, h.Now())
	if err != nil {
		return Response{}, err
	}
	if len(raids) == 0 {
		return private("No upcoming raids. Schedule one with `/raid create`."), nil
	}
	var b strings.Builder
	b.WriteString("**Upcoming raids**\n")
	for i, r := range raids {
		if i == maxListed {
			fmt.Fprintf(&b, "…and %d more.", len(raids)-maxListed)
			break
		}
		unix := r.StartsAt.Unix()
		fmt.Fprintf(&b, "• **%s** · <t:%d:F> (<t:%d:R>) · %d/%d · led by <@%s>\n",
			r.Name, unix, unix, len(r.Members), domain.FireteamSize, r.LeaderID)
	}
	return private(b.String()), nil
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
