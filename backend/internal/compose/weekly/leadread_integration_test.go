// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package weekly

import (
	"maps"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAWeekRemembersWhetherItsLeadsWereMeasuredWithAuthority(t *testing.T) {
	for _, readableAtFreeze := range []bool{false, true} {
		t.Run(map[bool]string{false: "withheld at freeze", true: "withheld on read"}[readableAtFreeze], func(t *testing.T) {
			e := setupWeekly(t)
			owner := ids.From[ids.UserKind](e.Rep1)
			lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: new("Lena Falk"), Status: "new", OwnerID: &owner, Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
			// Lead creation uses database time; put the fixture inside the frozen week.
			e.WsExec(t, `UPDATE lead SET created_at=@at, routed_at=@at WHERE id=@id`, pgx.StrictNamedArgs{"at": start.Add(24 * time.Hour), "id": ids.UUID(lead.Id)})
			denied, ok := principal.Actor(e.repCtx)
			if !ok {
				t.Fatal("fixture has no principal")
			}
			denied.Permissions.Objects = maps.Clone(denied.Permissions.Objects)
			delete(denied.Permissions.Objects, "lead")
			noLeads := principal.WithActor(e.repCtx, denied)
			measuredCtx := noLeads
			if readableAtFreeze {
				measuredCtx = e.repCtx
			}
			written, created, err := e.engine.AssembleFor(measuredCtx, weekClock)
			if err != nil || !created {
				t.Fatalf("assemble = %v, created=%t", err, created)
			}
			if written.LeadsWithheld == readableAtFreeze {
				t.Fatalf("frozen withheld=%t", written.LeadsWithheld)
			}
			if readableAtFreeze && written.Counts.LeadsRouted != 1 {
				t.Fatalf("authorized count=%d, want 1", written.Counts.LeadsRouted)
			}
			hidden, err := e.engine.LatestReview(noLeads, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !hidden.LeadsWithheld || hidden.Counts.LeadsRouted != 0 || (hidden.Scorecard != nil && hidden.Scorecard.Lead != nil) {
				t.Fatal("reader without lead access received lead facts")
			}
			restored, err := e.engine.LatestReview(e.repCtx, nil)
			if err != nil {
				t.Fatal(err)
			}
			if restored.LeadsWithheld == readableAtFreeze {
				t.Fatal("read changed the frozen measurement state")
			}
			if readableAtFreeze && restored.Counts.LeadsRouted != 1 {
				t.Fatal("read redaction overwrote the stored count")
			}
			team, wrote, err := e.engine.AssembleTeamFor(e.repCtx, e.Team1, "Sales", []TeamMember{{UserID: e.Rep1, DisplayName: "Lena Falk"}}, weekClock)
			if err != nil || !wrote {
				t.Fatalf("team assemble = %v, wrote=%t", err, wrote)
			}
			if team.LeadsWithheld == readableAtFreeze {
				t.Fatal("team lost its member's lead availability")
			}
			hiddenTeam, err := e.engine.TeamReview(noLeads, e.Team1, team.LocalWeekStart)
			if err != nil {
				t.Fatal(err)
			}
			if !hiddenTeam.LeadsWithheld || hiddenTeam.Counts.LeadsRouted != 0 {
				t.Fatal("team read disclosed unavailable lead counts")
			}
			restoredTeam, err := e.engine.TeamReview(e.repCtx, e.Team1, team.LocalWeekStart)
			if err != nil {
				t.Fatal(err)
			}
			if restoredTeam.LeadsWithheld == readableAtFreeze {
				t.Fatal("team read changed its frozen availability")
			}
			if readableAtFreeze && restoredTeam.Counts.LeadsRouted != 1 {
				t.Fatal("team redaction overwrote its stored count")
			}

		})
	}
}
