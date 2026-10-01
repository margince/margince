// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package weekly

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAResponseAfterTheWeekClosedDoesNotImproveThatWeek(t *testing.T) {
	e := setupWeekly(t)
	owner := ids.From[ids.UserKind](e.Rep1)
	name := "Arrival cohort"
	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: &name, Status: "new", OwnerID: &owner, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	start, end := lead.CreatedAt.Add(-time.Hour), lead.CreatedAt.Add(time.Hour)
	leadID := ids.From[ids.LeadKind](ids.UUID(lead.Id))
	if wrote, err := e.Contacts.RecordLeadFirstResponse(e.Admin(), leadID, end); err != nil || !wrote {
		t.Fatalf("record response: %v, wrote=%v", err, wrote)
	}
	check := func(want int) {
		t.Helper()
		err := database.WithWorkspaceTx(e.repCtx, e.Pool, func(tx pgx.Tx) error {
			routed, answered, _, err := countWeekLeads(e.repCtx, tx, e.Rep1, start, end)
			if err != nil {
				return err
			}
			if routed != 1 || answered != want {
				t.Errorf("arrival cohort = %d, answered = %d; want 1 and %d", routed, answered, want)
			}
			card, err := scoreLeads(e.repCtx, tx, e.Rep1, start, end)
			if err != nil {
				return err
			}
			if card == nil || card.AnsweredInTarget != want {
				t.Errorf("funnel does not share the response cutoff: %+v", card)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	check(0)
	if wrote, err := e.Contacts.RecordLeadFirstResponse(e.Admin(), leadID, end.Add(-time.Minute)); err != nil || !wrote {
		t.Fatalf("record earlier response: %v, wrote=%v", err, wrote)
	}
	check(1)
}

func TestImportedHistoryDoesNotCountAsThisWeeksMissedIntake(t *testing.T) {
	e := setupWeekly(t)
	owner := ids.From[ids.UserKind](e.Rep1)
	name, sourceSystem, sourceID := "Historical prospect", "mirror:hubspot", "leads:400"
	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{
		FullName: &name, OwnerID: &owner, Source: "import", SourceSystem: &sourceSystem, SourceID: &sourceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	at := lead.CreatedAt.Add(time.Minute)
	e.WsExec(t, `UPDATE lead SET sla_breached_at = $2, first_response_at = $2 WHERE id = $1`, lead.Id, at)
	err = database.WithWorkspaceTx(e.repCtx, e.Pool, func(tx pgx.Tx) error {
		routed, answered, breached, err := countWeekLeads(e.repCtx, tx, e.Rep1, lead.CreatedAt.Add(-time.Hour), at.Add(time.Hour))
		if err != nil {
			return err
		}
		if routed != 0 || answered != 0 || breached != 0 {
			t.Errorf("imported intake counts = %d/%d/%d", routed, answered, breached)
		}
		card, err := scoreLeads(e.repCtx, tx, e.Rep1, lead.CreatedAt.Add(-time.Hour), at.Add(time.Hour))
		if err != nil {
			return err
		}
		if card != nil && (card.AnsweredInTarget != 0 || card.Breached != 0) {
			t.Errorf("imported SLA scorecard = %+v", card)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count := e.WsCount(t, `SELECT count(*) FROM lead WHERE id = $1`, lead.Id); count != 1 {
		t.Fatal("imported lead history was removed")
	}
}
