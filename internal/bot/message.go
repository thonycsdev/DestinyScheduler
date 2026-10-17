// Package bot turns Discord interactions into raid actions and renders the
// replies. Handlers are plain functions over the raid repository; the
// discordgo session wiring lives in cmd/bot.
package bot

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"

	domain "github.com/anthonycoutinho/destinyscheduler/internal/raid"
)

const (
	colorOpen      = 0xE0B040 // Destiny gold
	colorCancelled = 0x707070

	ActionJoin   = "join"
	ActionLeave  = "leave"
	ActionCancel = "cancel"
)

// customID builds a button ID: raid:<action>:<raid id>.
func customID(action, raidID string) string {
	return "raid:" + action + ":" + raidID
}

// ParseCustomID splits a button ID built by customID. ok is false for
// anything else.
func ParseCustomID(s string) (action, raidID string, ok bool) {
	parts := strings.SplitN(s, ":", 3)
	if len(parts) != 3 || parts[0] != "raid" || parts[2] == "" {
		return "", "", false
	}
	switch parts[1] {
	case ActionJoin, ActionLeave, ActionCancel:
		return parts[1], parts[2], true
	}
	return "", "", false
}

// whenField shows the start as Discord timestamps, which every viewer sees
// in their own local time.
func whenField(r domain.Raid) *discordgo.MessageEmbedField {
	unix := r.StartsAt.Unix()
	return &discordgo.MessageEmbedField{
		Name:  "When",
		Value: fmt.Sprintf("<t:%d:F> (<t:%d:R>)", unix, unix),
	}
}

func fireteamField(r domain.Raid) *discordgo.MessageEmbedField {
	lines := make([]string, len(r.Members))
	for i, m := range r.Members {
		lines[i] = "<@" + m + ">"
		if m == r.LeaderID {
			lines[i] = "👑 " + lines[i]
		}
	}
	return &discordgo.MessageEmbedField{
		Name:  fmt.Sprintf("Fireteam (%d/%d)", len(r.Members), domain.FireteamSize),
		Value: strings.Join(lines, "\n"),
	}
}

// RaidMessage renders a scheduled raid with its Join, Leave and Cancel buttons.
func RaidMessage(r domain.Raid) (*discordgo.MessageEmbed, []discordgo.MessageComponent) {
	embed := &discordgo.MessageEmbed{
		Title:  r.Name,
		Color:  colorOpen,
		Fields: []*discordgo.MessageEmbedField{whenField(r), fireteamField(r)},
		Footer: &discordgo.MessageEmbedFooter{Text: "Times are shown in your local time zone."},
	}
	components := []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{Label: "Join", Style: discordgo.SuccessButton, CustomID: customID(ActionJoin, r.ID), Disabled: len(r.Members) >= domain.FireteamSize},
			discordgo.Button{Label: "Leave", Style: discordgo.SecondaryButton, CustomID: customID(ActionLeave, r.ID)},
			discordgo.Button{Label: "Cancel", Style: discordgo.DangerButton, CustomID: customID(ActionCancel, r.ID)},
		}},
	}
	return embed, components
}

// CancelledMessage renders a cancelled raid. Components is empty (not nil) so
// editing the message removes its buttons.
func CancelledMessage(r domain.Raid) (*discordgo.MessageEmbed, []discordgo.MessageComponent) {
	embed := &discordgo.MessageEmbed{
		Title:  "Cancelled: " + r.Name,
		Color:  colorCancelled,
		Fields: []*discordgo.MessageEmbedField{whenField(r), fireteamField(r)},
	}
	return embed, []discordgo.MessageComponent{}
}
