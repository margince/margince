// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingmap

import (
	"strings"
	"testing"
)

// legacyBody is the body capture stored before it rendered descriptions: the
// same header and the raw description, with no mask and no cut.
func legacyBody(ev Event, attendees []string) string {
	header := "Organizer: " + orDash(strings.TrimSpace(ev.Organizer.Email))
	if len(attendees) > 0 {
		header += "\nAttendees: " + strings.Join(attendees, ", ")
	}
	if desc := strings.TrimSpace(ev.Description); desc != "" {
		return header + "\n\n" + desc
	}
	return header
}

func TestARerenderedStoredBodyIsTheBodyCaptureStoresToday(t *testing.T) {
	t.Parallel()
	attendees := []string{"guest@rerender.test", "other@rerender.test"}
	for name, desc := range map[string]string{
		"booking page":     "Booked by <b>Guest</b><br>Topic: intro<script>alert(1)</script>",
		"Outlook body":     "<html><body><div><p>Dial in</p><p>Agenda: <i>renewal</i></p></div></body></html>",
		"credential":       "<p>password: <b>hunter2secret</b></p>",
		"comment only":     "<!-- generated -->Plain words",
		"renders to empty": "<p></p><br>",
		"long":             "<p>" + strings.Repeat("Agenda line. ", 900) + "</p>",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ev := Event{Description: desc}
			ev.Organizer.Email = "host@rerender.test"
			want, _ := buildBody(ev, attendees)
			got, changed := RerenderStoredBody(legacyBody(ev, attendees))
			if got != want || !changed {
				t.Fatalf("rerendered %q (changed %v), capture stores %q", got, changed, want)
			}
		})
	}
}

func TestAStoredBodyWithoutTagsIsLeftAlone(t *testing.T) {
	t.Parallel()
	for name, stored := range map[string]string{
		"header only":    "Organizer: host@rerender.test",
		"plain text":     "Organizer: host@rerender.test\n\nCall me at 9",
		"bracketed link": "Organizer: host@rerender.test\n\nJoin <https://meet.rerender.test/x> or mail <a@b.test>",
		"tag in header":  "Organizer: <b>@rerender.test",
	} {
		if got, changed := RerenderStoredBody(stored); changed || got != stored {
			t.Errorf("%s: rerendered %q (changed %v), want it unchanged", name, got, changed)
		}
	}
}
