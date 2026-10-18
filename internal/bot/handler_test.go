package bot

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/anthonycoutinho/destinyscheduler/internal/database/dbtest"
	raidrepo "github.com/anthonycoutinho/destinyscheduler/internal/repository/raid"
)

func TestMain(m *testing.M) { os.Exit(dbtest.Run(m)) }

// handlerNow is the fixed current time for handler tests; raids are created
// for 2026-10-20 21:00 in São Paulo (2026-10-21 00:00 UTC), after it.
var handlerNow = time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)

func newHandler(t *testing.T) (*Handler, *raidrepo.Repository) {
	t.Helper()
	repo := raidrepo.New(dbtest.Pool(t))
	h := &Handler{Raids: repo, Now: func() time.Time { return handlerNow }}
	return h, repo
}

var validCreate = CreateInput{
	UserID:   "leader-1",
	Raid:     "Last Wish",
	Date:     "2026-10-20",
	Time:     "21:00",
	Timezone: "America/Sao_Paulo",
}

// createRaid runs /raid create and returns the new raid's ID.
func createRaid(t *testing.T, h *Handler) string {
	t.Helper()
	resp, err := h.Create(context.Background(), validCreate)
	if err != nil {
		t.Fatal(err)
	}
	if resp.RaidID == "" {
		t.Fatalf("Create response has no RaidID: %+v", resp)
	}
	return resp.RaidID
}

func click(t *testing.T, h *Handler, user, action, raidID string) Response {
	t.Helper()
	resp, err := h.Button(context.Background(), ButtonInput{UserID: user, CustomID: customID(action, raidID)})
	if err != nil {
		t.Fatalf("%s by %s: unexpected error %v", action, user, err)
	}
	return resp
}

func wantPrivate(t *testing.T, resp Response, contains string) {
	t.Helper()
	if !resp.Ephemeral || resp.UpdateMessage || !strings.Contains(strings.ToLower(resp.Content), strings.ToLower(contains)) {
		t.Errorf("response = %+v, want a private message containing %q", resp, contains)
	}
}

// TestCreateRaid covers /raid create: the user's date, time and time zone
// become a stored raid led by them, and the reply is the public raid message.
// RecordMessage then saves where that message was posted.
func TestCreateRaid(t *testing.T) {
	h, repo := newHandler(t)
	ctx := context.Background()

	resp, err := h.Create(ctx, validCreate)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Ephemeral || resp.UpdateMessage || resp.Embed == nil || resp.Embed.Title != "Last Wish" {
		t.Fatalf("response = %+v, want a public Last Wish raid message", resp)
	}
	if when := field(t, resp.Embed, "When").Value; !strings.Contains(when, fmt.Sprintf("<t:%d:F>", testStart.Unix())) {
		t.Errorf("When = %q, want 21:00 São Paulo = %v", when, testStart)
	}

	stored, err := repo.Get(ctx, resp.RaidID)
	if err != nil {
		t.Fatalf("raid not stored: %v", err)
	}
	if stored.LeaderID != "leader-1" || !stored.StartsAt.Equal(testStart) {
		t.Errorf("stored = %+v, want leader-1 at %v", stored, testStart)
	}

	if err := h.RecordMessage(ctx, resp.RaidID, "channel-1", "message-1"); err != nil {
		t.Fatal(err)
	}
	if stored, _ = repo.Get(ctx, resp.RaidID); stored.ChannelID != "channel-1" || stored.MessageID != "message-1" {
		t.Errorf("ChannelID/MessageID = %q/%q, want channel-1/message-1", stored.ChannelID, stored.MessageID)
	}
}

// TestCreateRaidRejectsBadInput ensures mistakes in /raid create get a private
// explanation and nothing is stored.
func TestCreateRaidRejectsBadInput(t *testing.T) {
	h, repo := newHandler(t)
	cases := []struct {
		name     string
		edit     func(*CreateInput)
		contains string
	}{
		{"unknown time zone", func(in *CreateInput) { in.Timezone = "Mars/Olympus" }, "time zone"},
		{"bad date", func(in *CreateInput) { in.Date = "20/10/2026" }, "YYYY-MM-DD"},
		{"past time", func(in *CreateInput) { in.Date = "2026-10-16" }, "past"},
		{"unknown raid", func(in *CreateInput) { in.Raid = "Raid of Mars" }, "unknown raid"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := validCreate
			c.edit(&in)
			resp, err := h.Create(context.Background(), in)
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			wantPrivate(t, resp, c.contains)
		})
	}
	if raids, _ := repo.ListUpcoming(context.Background(), handlerNow); len(raids) != 0 {
		t.Errorf("%d raids stored after bad input, want 0", len(raids))
	}
}

// TestJoinAndLeaveButtons ensures Join and Leave save the change and edit the
// raid message in place with the new fireteam.
func TestJoinAndLeaveButtons(t *testing.T) {
	h, repo := newHandler(t)
	id := createRaid(t, h)

	resp := click(t, h, "u2", ActionJoin, id)
	if !resp.UpdateMessage || resp.Ephemeral {
		t.Fatalf("Join response = %+v, want an in-place message update", resp)
	}
	if got := field(t, resp.Embed, "Fireteam").Name; got != "Fireteam (2/6)" {
		t.Errorf("after Join, %q, want Fireteam (2/6)", got)
	}

	resp = click(t, h, "u2", ActionLeave, id)
	if !resp.UpdateMessage || field(t, resp.Embed, "Fireteam").Name != "Fireteam (1/6)" {
		t.Errorf("Leave response = %+v, want an update showing Fireteam (1/6)", resp)
	}
	if stored, _ := repo.Get(context.Background(), id); len(stored.Members) != 1 {
		t.Errorf("stored members = %v, want only the leader", stored.Members)
	}
}

// TestButtonErrorsArePrivate ensures every refused click is explained only to
// the person who clicked, without touching the public message.
func TestButtonErrorsArePrivate(t *testing.T) {
	h, _ := newHandler(t)
	id := createRaid(t, h)
	click(t, h, "u2", ActionJoin, id)

	wantPrivate(t, click(t, h, "u2", ActionJoin, id), "already in")
	wantPrivate(t, click(t, h, "u9", ActionLeave, id), "not in")
	wantPrivate(t, click(t, h, "leader-1", ActionLeave, id), "cancel")
	wantPrivate(t, click(t, h, "u2", ActionJoin, "0199f3a0-0000-7000-8000-000000000000"), "no longer exists")

	for _, u := range []string{"u3", "u4", "u5", "u6"} {
		click(t, h, u, ActionJoin, id)
	}
	wantPrivate(t, click(t, h, "u7", ActionJoin, id), "full")

	h.Now = func() time.Time { return testStart }
	wantPrivate(t, click(t, h, "u3", ActionLeave, id), "already started")

	resp, err := h.Button(context.Background(), ButtonInput{UserID: "u2", CustomID: "something-else"})
	if err != nil {
		t.Fatal(err)
	}
	wantPrivate(t, resp, "")
}

// TestCancelButton ensures only the leader or a moderator (Manage Messages)
// can cancel; cancelling deletes the raid and turns the message into the
// cancelled version, even after the raid has started.
func TestCancelButton(t *testing.T) {
	h, repo := newHandler(t)
	ctx := context.Background()

	id := createRaid(t, h)
	click(t, h, "u2", ActionJoin, id)
	wantPrivate(t, click(t, h, "u2", ActionCancel, id), "leader")
	if _, err := repo.Get(ctx, id); err != nil {
		t.Fatalf("raid removed after a refused cancel: %v", err)
	}

	h.Now = func() time.Time { return testStart.Add(time.Hour) }
	resp := click(t, h, "leader-1", ActionCancel, id)
	if !resp.UpdateMessage || !strings.Contains(resp.Embed.Title, "Cancelled") || len(resp.Components) != 0 {
		t.Errorf("leader cancel response = %+v, want an update to the cancelled message", resp)
	}
	if _, err := repo.Get(ctx, id); err == nil {
		t.Error("raid still stored after the leader cancelled it")
	}

	h.Now = func() time.Time { return handlerNow }
	id = createRaid(t, h)
	resp, err := h.Button(ctx, ButtonInput{UserID: "moderator", CustomID: customID(ActionCancel, id), CanManageMessages: true})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.UpdateMessage || !strings.Contains(resp.Embed.Title, "Cancelled") {
		t.Errorf("moderator cancel response = %+v, want the cancelled message", resp)
	}
}

// TestListRaids ensures /raid list replies privately with upcoming raids,
// soonest first, each with its time and fireteam size.
func TestListRaids(t *testing.T) {
	h, _ := newHandler(t)
	ctx := context.Background()

	resp, err := h.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wantPrivate(t, resp, "no upcoming raids")

	later := validCreate
	later.Raid, later.Date = "Vault of Glass", "2026-10-22"
	if _, err := h.Create(ctx, later); err != nil {
		t.Fatal(err)
	}
	id := createRaid(t, h) // Last Wish on 2026-10-20
	click(t, h, "u2", ActionJoin, id)

	resp, err = h.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wantPrivate(t, resp, "Last Wish")
	lw, vog := strings.Index(resp.Content, "Last Wish"), strings.Index(resp.Content, "Vault of Glass")
	if vog < 0 || lw > vog {
		t.Errorf("list = %q, want Last Wish before Vault of Glass", resp.Content)
	}
	for _, want := range []string{fmt.Sprintf("<t:%d:F>", testStart.Unix()), "2/6"} {
		if !strings.Contains(resp.Content, want) {
			t.Errorf("list = %q, want it to contain %q", resp.Content, want)
		}
	}
}
