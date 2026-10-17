package bot

import (
	"context"
	"log/slog"
	"time"

	"github.com/bwmarrin/discordgo"

	domain "github.com/anthonycoutinho/destinyscheduler/internal/raid"
)

// Discord expects a reply within 3 seconds.
const interactionTimeout = 2500 * time.Millisecond

// Commands defines the /raid command registered with Discord.
func Commands() []*discordgo.ApplicationCommand {
	raids := make([]*discordgo.ApplicationCommandOptionChoice, len(domain.Names))
	for i, n := range domain.Names {
		raids[i] = &discordgo.ApplicationCommandOptionChoice{Name: n, Value: n}
	}
	return []*discordgo.ApplicationCommand{{
		Name:        "raid",
		Description: "Schedule Destiny 2 raids",
		Contexts:    &[]discordgo.InteractionContextType{discordgo.InteractionContextGuild},
		Options: []*discordgo.ApplicationCommandOption{
			{
				Name:        "create",
				Description: "Schedule a raid; everyone sees the time in their own time zone",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Options: []*discordgo.ApplicationCommandOption{
					{Name: "raid", Description: "Which raid", Type: discordgo.ApplicationCommandOptionString, Required: true, Choices: raids},
					{Name: "date", Description: "Date, YYYY-MM-DD", Type: discordgo.ApplicationCommandOptionString, Required: true},
					{Name: "time", Description: "Start time, 24h HH:MM", Type: discordgo.ApplicationCommandOptionString, Required: true},
					{Name: "timezone", Description: "Your time zone, e.g. America/Sao_Paulo", Type: discordgo.ApplicationCommandOptionString, Required: true, Autocomplete: true},
				},
			},
			{
				Name:        "list",
				Description: "Show upcoming raids (only you see the reply)",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
			},
		},
	}}
}

// subcommand returns the /raid subcommand option the user invoked.
func subcommand(i *discordgo.Interaction) *discordgo.ApplicationCommandInteractionDataOption {
	opts := i.ApplicationCommandData().Options
	if len(opts) == 0 {
		return nil
	}
	return opts[0]
}

// Subcommand returns the name of the /raid subcommand, e.g. "create".
func Subcommand(i *discordgo.Interaction) string {
	if sub := subcommand(i); sub != nil {
		return sub.Name
	}
	return ""
}

func userID(i *discordgo.Interaction) string {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.ID
	}
	if i.User != nil {
		return i.User.ID
	}
	return ""
}

// CreateInputFrom reads the /raid create options.
func CreateInputFrom(i *discordgo.Interaction) CreateInput {
	in := CreateInput{UserID: userID(i)}
	if sub := subcommand(i); sub != nil {
		for _, o := range sub.Options {
			switch o.Name {
			case "raid":
				in.Raid = o.StringValue()
			case "date":
				in.Date = o.StringValue()
			case "time":
				in.Time = o.StringValue()
			case "timezone":
				in.Timezone = o.StringValue()
			}
		}
	}
	return in
}

// FocusedValue returns what the user has typed so far in the option being
// autocompleted.
func FocusedValue(i *discordgo.Interaction) string {
	if sub := subcommand(i); sub != nil {
		for _, o := range sub.Options {
			if o.Focused {
				return o.StringValue()
			}
		}
	}
	return ""
}

// ButtonInputFrom reads a button click, including whether the clicker has
// Manage Messages in the channel (Discord sends channel-resolved permissions).
func ButtonInputFrom(i *discordgo.Interaction) ButtonInput {
	in := ButtonInput{UserID: userID(i), CustomID: i.MessageComponentData().CustomID}
	if i.Member != nil {
		in.CanManageMessages = i.Member.Permissions&discordgo.PermissionManageMessages != 0
	}
	return in
}

// InteractionResponse converts a Response into Discord's reply. Slices are
// never nil: an empty components list is what removes a message's buttons.
// Mentions never ping anyone.
func InteractionResponse(r Response) *discordgo.InteractionResponse {
	data := &discordgo.InteractionResponseData{
		Content:         r.Content,
		Components:      r.Components,
		Embeds:          []*discordgo.MessageEmbed{},
		AllowedMentions: &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}},
	}
	if data.Components == nil {
		data.Components = []discordgo.MessageComponent{}
	}
	if r.Embed != nil {
		data.Embeds = append(data.Embeds, r.Embed)
	}
	if r.Ephemeral {
		data.Flags = discordgo.MessageFlagsEphemeral
	}
	typ := discordgo.InteractionResponseChannelMessageWithSource
	if r.UpdateMessage {
		typ = discordgo.InteractionResponseUpdateMessage
	}
	return &discordgo.InteractionResponse{Type: typ, Data: data}
}

// Dispatch handles one interaction from the Discord session. It's the thin,
// untested adapter between discordgo and the Handler.
func (h *Handler) Dispatch(s *discordgo.Session, ic *discordgo.InteractionCreate) {
	i := ic.Interaction
	ctx, cancel := context.WithTimeout(context.Background(), interactionTimeout)
	defer cancel()
	log := slog.With("interaction", i.ID, "user", userID(i))

	var resp Response
	var err error
	switch i.Type {
	case discordgo.InteractionApplicationCommandAutocomplete:
		err := s.InteractionRespond(i, &discordgo.InteractionResponse{
			Type: discordgo.InteractionApplicationCommandAutocompleteResult,
			Data: &discordgo.InteractionResponseData{Choices: TimezoneChoices(FocusedValue(i))},
		})
		if err != nil {
			log.Error("autocomplete reply failed", "err", err)
		}
		return
	case discordgo.InteractionApplicationCommand:
		switch Subcommand(i) {
		case "create":
			resp, err = h.Create(ctx, CreateInputFrom(i))
		case "list":
			resp, err = h.List(ctx)
		default:
			resp = private("Unknown command.")
		}
	case discordgo.InteractionMessageComponent:
		resp, err = h.Button(ctx, ButtonInputFrom(i))
	default:
		return
	}
	if err != nil {
		log.Error("handling interaction failed", "err", err)
		resp = private("Something went wrong. Please try again in a moment.")
	}

	if err := s.InteractionRespond(i, InteractionResponse(resp)); err != nil {
		log.Error("reply failed", "err", err)
		return
	}
	if resp.RaidID == "" {
		return
	}
	// Record where the new raid message landed so it can be found later.
	msg, err := s.InteractionResponse(i)
	if err != nil {
		log.Error("reading posted raid message failed", "raid", resp.RaidID, "err", err)
		return
	}
	if err := h.RecordMessage(context.Background(), resp.RaidID, msg.ChannelID, msg.ID); err != nil {
		log.Error("recording raid message failed", "raid", resp.RaidID, "err", err)
	}
}
