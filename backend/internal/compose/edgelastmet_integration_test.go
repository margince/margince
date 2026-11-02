// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The colleague edge knows when the pair last met, on the incremental fold and
// on the rebuild alike, so its recency counts from a meeting as well as a reply.

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/search"
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
