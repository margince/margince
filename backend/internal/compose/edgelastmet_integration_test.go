// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The colleague edge knows when the pair last met, on the incremental fold and
// on the rebuild alike. Its recency then counts from a meeting as well as a reply.

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestTheColleagueEdgeKnowsWhenThePairLastMet(t *testing.T) {
	e := integration.Setup(t)
	now := time.Now().UTC().Truncate(time.Second)
	dana := contactWithEmail(t, e, "Dana Met", "dana@met.example")
	parties := []connector.MessageParticipant{{Email: "dana@met.example", Role: connector.ParticipantRoleAttendee}}
	met := integration.CalendarMeeting{Event: "evt-met", At: now.AddDate(0, 0, -6), Links: contactLink(dana), Parties: parties}
	touched := []ids.UUID{met.Capture(t, e)}

	assertMet := func(path string) {
		t.Helper()
		if got := e.WsCount(t, `SELECT count(*) FROM graph_interaction_edge
		                         WHERE user_id = $1 AND contact_id = $2 AND last_met_at = $3`,
			e.AdminUser, dana, met.At); got != 1 {
			t.Errorf("%s: the colleague edge's last_met_at is not the meeting", path)
		}
	}
	inWorkspaceTx(t, e, func(tx pgx.Tx) error { return search.RecomputeEdgesForActivities(e.Admin(), tx, touched) })
	assertMet("incremental fold")
	inWorkspaceTx(t, e, func(tx pgx.Tx) error { return search.RebuildEdges(e.Admin(), tx) })
	assertMet("rebuild")
}

// A meeting the contact declined gives the pair no last meeting.
func TestADeclinedMeetingGivesThePairNoLastMeeting(t *testing.T) {
	e := integration.Setup(t)
	now := clocktest.Now(t).UTC().Truncate(time.Second)
	dana := contactWithEmail(t, e, "Dana Declined", "dana@declined.example")
	parties := []connector.MessageParticipant{
		{Email: "dana@declined.example", Role: connector.ParticipantRoleAttendee, Response: connector.ResponseDeclined},
	}
	declined := integration.CalendarMeeting{Event: "evt-declined", At: now.AddDate(0, 0, -6), Links: contactLink(dana), Parties: parties}
	touched := []ids.UUID{declined.Capture(t, e)}
	for path, refold := range map[string]func(tx pgx.Tx) error{
		"incremental fold": func(tx pgx.Tx) error { return search.RecomputeEdgesForActivities(e.Admin(), tx, touched) },
		"rebuild":          func(tx pgx.Tx) error { return search.RebuildEdges(e.Admin(), tx) },
	} {
		inWorkspaceTx(t, e, refold)
		if got := e.WsCount(t, `SELECT count(*) FROM graph_interaction_edge
		                         WHERE contact_id = $1 AND last_met_at IS NOT NULL`, dana); got != 0 {
			t.Errorf("%s: a declined meeting gave the pair a last meeting", path)
		}
	}
}
