package bot

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"

	domain "github.com/anthonycoutinho/destinyscheduler/internal/raid"
)

var testStart = time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)

func testRaid(members ...string) domain.Raid {
	return domain.Raid{
		ID:       "0199f3a0-0000-7000-8000-000000000001",
		Name:     "Last Wish",
		StartsAt: testStart,
		LeaderID: "leader-1",
		Members:  append([]string{"leader-1"}, members...),
	}
}

func field(t *testing.T, e *discordgo.MessageEmbed, prefix string) *discordgo.MessageEmbedField {
	t.Helper()
	for _, f := range e.Fields {
		if strings.HasPrefix(f.Name, prefix) {
			return f
		}
	}
	t.Fatalf("embed has no field starting with %q; fields: %+v", prefix, e.Fields)
	return nil
}

func buttons(t *testing.T, components []discordgo.MessageComponent) []discordgo.Button {
	t.Helper()
	var out []discordgo.Button
	for _, c := range components {
		row, ok := c.(discordgo.ActionsRow)
		if !ok {
			t.Fatalf("component %T is not an ActionsRow", c)
		}
		for _, b := range row.Components {
			out = append(out, b.(discordgo.Button))
		}
	}
	return out
}

// TestRaidMessageShowsTimeInEachViewersZone pins the README's core promise:
// the start time is rendered as Discord timestamps, which Discord shows in
// each viewer's own local time, absolute (F) and relative (R).
func TestRaidMessageShowsTimeInEachViewersZone(t *testing.T) {
	embed, _ := RaidMessage(testRaid())
	when := field(t, embed, "When").Value
	unix := testStart.Unix()
	for _, want := range []string{fmt.Sprintf("<t:%d:F>", unix), fmt.Sprintf("<t:%d:R>", unix)} {
		if !strings.Contains(when, want) {
			t.Errorf("When = %q, want it to contain %q", when, want)
		}
	}
	if embed.Title != "Last Wish" {
		t.Errorf("Title = %q, want %q", embed.Title, "Last Wish")
	}
}

// TestRaidMessageListsFireteam ensures the fireteam is shown as Discord
// mentions in join order, the leader is marked, and the count is out of 6.
func TestRaidMessageListsFireteam(t *testing.T) {
	embed, _ := RaidMessage(testRaid("u2", "u3"))
	f := field(t, embed, "Fireteam")
	if f.Name != "Fireteam (3/6)" {
		t.Errorf("field name = %q, want %q", f.Name, "Fireteam (3/6)")
	}
	want := "👑 <@leader-1>\n<@u2>\n<@u3>"
	if f.Value != want {
		t.Errorf("fireteam = %q, want %q", f.Value, want)
	}
}

// TestRaidMessageButtons ensures the message carries Join, Leave and Cancel
// buttons for this raid, and Join is greyed out once the fireteam is full.
func TestRaidMessageButtons(t *testing.T) {
	r := testRaid()
	_, components := RaidMessage(r)
	got := buttons(t, components)
	var ids []string
	for _, b := range got {
		ids = append(ids, b.CustomID)
	}
	want := []string{"raid:join:" + r.ID, "raid:leave:" + r.ID, "raid:cancel:" + r.ID}
	if !slices.Equal(ids, want) {
		t.Fatalf("button IDs = %v, want %v", ids, want)
	}
	if got[0].Disabled {
		t.Error("Join is disabled on a raid with free slots")
	}

	_, components = RaidMessage(testRaid("u2", "u3", "u4", "u5", "u6"))
	if join := buttons(t, components)[0]; !join.Disabled {
		t.Error("Join is enabled on a full raid")
	}
}

// TestCancelledMessage ensures a cancelled raid keeps its details visible,
// says it's cancelled, and has no buttons. Components must be an empty, non-nil
// slice so editing the message actually removes the old buttons.
func TestCancelledMessage(t *testing.T) {
	embed, components := CancelledMessage(testRaid("u2"))
	if !strings.Contains(embed.Title, "Cancelled") || !strings.Contains(embed.Title, "Last Wish") {
		t.Errorf("Title = %q, want it to say Last Wish is cancelled", embed.Title)
	}
	field(t, embed, "When")
	if components == nil || len(components) != 0 {
		t.Errorf("components = %#v, want an empty non-nil slice", components)
	}
}

// TestParseCustomID ensures button IDs round-trip and that anything else
// (other bots' buttons, tampered IDs) is rejected.
func TestParseCustomID(t *testing.T) {
	action, id, ok := ParseCustomID("raid:join:abc")
	if !ok || action != "join" || id != "abc" {
		t.Errorf("ParseCustomID(raid:join:abc) = %q, %q, %v", action, id, ok)
	}
	for _, s := range []string{"", "raid:join:", "raid:dance:abc", "other:join:abc", "raid:join"} {
		if _, _, ok := ParseCustomID(s); ok {
			t.Errorf("ParseCustomID(%q) ok = true, want false", s)
		}
	}
}
