// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A colleague's meeting whose guest list Google withholds from the owner's copy
// names only colleagues, although outside guests were there. Nothing on the
// event proves it internal, so it is captured. The same event with the full
// list shown, all colleagues, stays out.
func TestAColleaguesMeetingWithAHiddenGuestListIsCaptured(t *testing.T) {
	env := newCaptureEnv(t)
	held := captureNow.AddDate(0, 0, -16)

	hidden := lateGuestEvent("hidden-guests", "confirmed", held, lateGuestOrganizer, ownerEntry("accepted"))
	hidden["guestsCanSeeOtherGuests"] = false
	omitted := lateGuestEvent("omitted-guests", "confirmed", held, lateGuestOrganizer,
		ownerEntry("accepted"), guestEntry(lateGuestOrganizer, "accepted"))
	omitted["attendeesOmitted"] = true
	shown := lateGuestEvent("shown-colleagues", "confirmed", held, lateGuestOrganizer,
		ownerEntry("accepted"), guestEntry(lateGuestOrganizer, "accepted"))
	ownHidden := lateGuestEvent("own-hidden", "confirmed", held, captureOwner,
		map[string]any{"email": captureOwner, "self": true, "organizer": true, "responseStatus": "accepted"},
		guestEntry(lateGuestOrganizer, "accepted"))
	ownHidden["organizer"] = map[string]any{"email": captureOwner, "self": true}
	ownHidden["guestsCanSeeOtherGuests"] = false

	connID := connectStubbedCalendar(t, env, []map[string]any{hidden, omitted, shown, ownHidden})
	if err := env.registry.SyncOnce(principal.WithWorkspaceID(context.Background(), env.e.WS), connID); err != nil {
		t.Fatalf("SyncOnce: %v", err)
	}

	for id, want := range map[string]int{"hidden-guests": 1, "omitted-guests": 1, "shown-colleagues": 0, "own-hidden": 0} {
		got := countRows(t, env.e, `SELECT count(*) FROM activity WHERE source_system = 'gcal' AND kind = 'meeting' AND source_id = '`+id+`'`)
		if got != want {
			t.Errorf("%s: %d meetings captured, want %d", id, got, want)
		}
	}
}
