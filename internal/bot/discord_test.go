package bot

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"

	domain "github.com/anthonycoutinho/destinyscheduler/internal/raid"
)

func strOpt(name, value string, focused bool) *discordgo.ApplicationCommandInteractionDataOption {
	return &discordgo.ApplicationCommandInteractionDataOption{Name: name, Type: discordgo.ApplicationCommandOptionString, Value: value, Focused: focused}
}

func commandInteraction(typ discordgo.InteractionType, sub string, opts ...*discordgo.ApplicationCommandInteractionDataOption) *discordgo.Interaction {
	return &discordgo.Interaction{
		Type:   typ,
		Member: &discordgo.Member{User: &discordgo.User{ID: "user-1"}},
		Data: discordgo.ApplicationCommandInteractionData{
			Name: "raid",
			Options: []*discordgo.ApplicationCommandInteractionDataOption{
				{Name: sub, Type: discordgo.ApplicationCommandOptionSubCommand, Options: opts},
			},
		},
	}
}

// TestCommandsDefinition pins the /raid command shape registered with Discord:
// create (raid dropdown of every known raid, date, time, autocompleted
// timezone, all required) and list, usable in servers only.
func TestCommandsDefinition(t *testing.T) {
	cmds := Commands()
	if len(cmds) != 1 || cmds[0].Name != "raid" {
		t.Fatalf("Commands() = %+v, want a single raid command", cmds)
	}
	raid := cmds[0]
	if raid.Contexts == nil || !slices.Equal(*raid.Contexts, []discordgo.InteractionContextType{discordgo.InteractionContextGuild}) {
		t.Errorf("Contexts = %v, want guild only", raid.Contexts)
	}

	subs := map[string]*discordgo.ApplicationCommandOption{}
	for _, o := range raid.Options {
		if o.Type != discordgo.ApplicationCommandOptionSubCommand {
			t.Errorf("option %q is %v, want a subcommand", o.Name, o.Type)
		}
		subs[o.Name] = o
	}
	if subs["list"] == nil || subs["create"] == nil {
		t.Fatalf("subcommands = %v, want create and list", subs)
	}

	opts := map[string]*discordgo.ApplicationCommandOption{}
	var order []string
	for _, o := range subs["create"].Options {
		opts[o.Name] = o
		order = append(order, o.Name)
		if !o.Required {
			t.Errorf("create option %q is optional, want required", o.Name)
		}
	}
	if want := []string{"raid", "date", "time", "timezone"}; !slices.Equal(order, want) {
		t.Errorf("create options = %v, want %v", order, want)
	}
	var choices []string
	for _, c := range opts["raid"].Choices {
		choices = append(choices, c.Value.(string))
	}
	if !slices.Equal(choices, domain.Names) {
		t.Errorf("raid choices = %v, want raid.Names %v", choices, domain.Names)
	}
	if !opts["timezone"].Autocomplete {
		t.Error("timezone option has no autocomplete")
	}
}

// TestReadInteractions ensures the adapter reads who acted and what they
// chose from Discord's interaction payloads.
func TestReadInteractions(t *testing.T) {
	i := commandInteraction(discordgo.InteractionApplicationCommand, "create",
		strOpt("raid", "Last Wish", false), strOpt("date", "2026-10-20", false),
		strOpt("time", "21:00", false), strOpt("timezone", "America/Sao_Paulo", false))
	if sub := Subcommand(i); sub != "create" {
		t.Errorf("Subcommand = %q, want create", sub)
	}
	want := CreateInput{UserID: "user-1", Raid: "Last Wish", Date: "2026-10-20", Time: "21:00", Timezone: "America/Sao_Paulo"}
	if got := CreateInputFrom(i); got != want {
		t.Errorf("CreateInputFrom = %+v, want %+v", got, want)
	}

	ac := commandInteraction(discordgo.InteractionApplicationCommandAutocomplete, "create",
		strOpt("raid", "Last Wish", false), strOpt("timezone", "sao", true))
	if got := FocusedValue(ac); got != "sao" {
		t.Errorf("FocusedValue = %q, want sao", got)
	}

	btn := &discordgo.Interaction{
		Type:   discordgo.InteractionMessageComponent,
		Member: &discordgo.Member{User: &discordgo.User{ID: "user-2"}, Permissions: discordgo.PermissionManageMessages | discordgo.PermissionSendMessages},
		Data:   discordgo.MessageComponentInteractionData{CustomID: "raid:join:abc"},
	}
	wantBtn := ButtonInput{UserID: "user-2", CustomID: "raid:join:abc", CanManageMessages: true}
	if got := ButtonInputFrom(btn); got != wantBtn {
		t.Errorf("ButtonInputFrom = %+v, want %+v", got, wantBtn)
	}
	btn.Member.Permissions = discordgo.PermissionSendMessages
	if ButtonInputFrom(btn).CanManageMessages {
		t.Error("CanManageMessages = true without the Manage Messages permission")
	}
}

func marshal(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestInteractionResponse ensures Responses map onto Discord's reply types:
// new message vs in-place update, the ephemeral flag, no pings from mentions,
// and an explicit empty component list so a cancelled message loses its buttons.
func TestInteractionResponse(t *testing.T) {
	embed, components := RaidMessage(testRaid())
	public := InteractionResponse(Response{Embed: embed, Components: components})
	if public.Type != discordgo.InteractionResponseChannelMessageWithSource || public.Data.Flags&discordgo.MessageFlagsEphemeral != 0 {
		t.Errorf("public raid message: type %v flags %v, want a new non-ephemeral message", public.Type, public.Data.Flags)
	}
	if len(public.Data.Embeds) != 1 || len(public.Data.Components) != 1 {
		t.Errorf("public raid message data = %+v, want 1 embed and 1 button row", public.Data)
	}

	priv := InteractionResponse(private("nope"))
	if priv.Type != discordgo.InteractionResponseChannelMessageWithSource || priv.Data.Flags&discordgo.MessageFlagsEphemeral == 0 || priv.Data.Content != "nope" {
		t.Errorf("private reply = %+v, want an ephemeral new message", priv)
	}

	embed, components = CancelledMessage(testRaid())
	upd := InteractionResponse(Response{Embed: embed, Components: components, UpdateMessage: true})
	if upd.Type != discordgo.InteractionResponseUpdateMessage {
		t.Errorf("update type = %v, want UpdateMessage", upd.Type)
	}
	if js := marshal(t, upd); !strings.Contains(js, `"components":[]`) {
		t.Errorf("cancelled update JSON = %s, want \"components\":[] to clear the buttons", js)
	}

	for _, r := range []*discordgo.InteractionResponse{public, priv, upd} {
		if r.Data.AllowedMentions == nil || len(r.Data.AllowedMentions.Parse) != 0 {
			t.Errorf("AllowedMentions = %+v, want an empty allow list (no pings)", r.Data.AllowedMentions)
		}
	}
}
