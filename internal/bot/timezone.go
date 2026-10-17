package bot

import (
	"cmp"
	"slices"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// maxChoices is Discord's limit on autocomplete suggestions.
const maxChoices = 25

// commonZones are suggested before the user types anything.
var commonZones = []string{
	"UTC",
	"America/Los_Angeles", "America/Denver", "America/Chicago", "America/New_York",
	"America/Toronto", "America/Mexico_City", "America/Sao_Paulo", "America/Argentina/Buenos_Aires",
	"Europe/London", "Europe/Lisbon", "Europe/Madrid", "Europe/Paris", "Europe/Berlin",
	"Europe/Warsaw", "Europe/Moscow", "Africa/Johannesburg", "Asia/Dubai", "Asia/Kolkata",
	"Asia/Singapore", "Asia/Shanghai", "Asia/Seoul", "Asia/Tokyo", "Australia/Sydney",
	"Pacific/Auckland",
}

var foldAccents = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a", "å", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ç", "c", "ñ", "n",
)

// normalize makes "São Paulo" match "America/Sao_Paulo".
func normalize(s string) string {
	s = foldAccents.Replace(strings.ToLower(strings.TrimSpace(s)))
	return strings.ReplaceAll(s, " ", "_")
}

// rank scores how well zone matches q: 0 when its city starts with q, 1 when
// another part does, 2 when it only contains q, and -1 for no match.
func rank(zone, q string) int {
	z := normalize(zone)
	if !strings.Contains(z, q) {
		return -1
	}
	parts := strings.Split(z, "/")
	if strings.HasPrefix(parts[len(parts)-1], q) {
		return 0
	}
	for _, p := range parts {
		if strings.HasPrefix(p, q) {
			return 1
		}
	}
	return 2
}

// TimezoneChoices suggests IANA time zones for the /raid create timezone
// option, best matches first, at most 25.
func TimezoneChoices(query string) []*discordgo.ApplicationCommandOptionChoice {
	q := normalize(query)
	matches := commonZones
	if q != "" {
		type scored struct {
			zone  string
			score int
		}
		var found []scored
		for _, z := range zones {
			if s := rank(z, q); s >= 0 {
				found = append(found, scored{z, s})
			}
		}
		slices.SortFunc(found, func(a, b scored) int {
			return cmp.Or(cmp.Compare(a.score, b.score), cmp.Compare(a.zone, b.zone))
		})
		matches = nil
		for _, f := range found {
			matches = append(matches, f.zone)
		}
	}

	choices := make([]*discordgo.ApplicationCommandOptionChoice, 0, maxChoices)
	for _, z := range matches[:min(len(matches), maxChoices)] {
		choices = append(choices, &discordgo.ApplicationCommandOptionChoice{Name: z, Value: z})
	}
	return choices
}
