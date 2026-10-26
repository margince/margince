// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A guessed seat row sets no "last spoke" baseline. The meeting brief says what
// moved on the deal "since" the reader last spoke with the room. Here a
// connector captured the prior meeting. Its only seat row names the reader by
// user_id, with no create audit on their behalf. That is the shape the legacy
// backfill guessed, so nobody knows the reader was there. The same meeting with the
// connector's create audit is real, and the movement sentence appears.
func TestAGuessedSeatSetsNoLastSpokeBaseline(t *testing.T) {
	for _, tc := range []struct {
		name string
		made bool
	}{{"guessed seat", false}, {"captured for the reader", true}} {
		t.Run(tc.name, func(t *testing.T) {
			e := Setup(t)
			pipeline, open, _ := DealFixture(t, e)
			deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{Name: "Rollout", PipelineID: pipeline, StageID: open, Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			var changed time.Time
			if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
				return tx.QueryRow(e.Admin(), `SELECT changed_at FROM deal_stage_history WHERE deal_id = $1`, deal.Id).Scan(&changed)
			}); err != nil {
				t.Fatal(err)
			}
			owner := OwnerConn(t)
			attendee := e.SeedContact(t, "Room contact", &e.Rep1)
			meeting := SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
				VALUES ($1, 'meeting', 'Next discussion', $2, 'manual', 'human:x')`, changed.Add(2*time.Hour))
			LinkActivity(t, owner, meeting, "contact", attendee)
			LinkActivity(t, owner, meeting, "deal", ids.UUID(deal.Id))
			seatInRoom(t, owner, e.WS, meeting, attendee)
			prior := SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
				VALUES ($1, 'meeting', 'Prior discussion', $2, 'gcal:x', 'connector:gcal')`, changed.Add(-time.Hour))
			LinkActivity(t, owner, prior, "contact", attendee)
			seatInRoom(t, owner, e.WS, prior, attendee)
			seatUserInRoom(t, owner, prior, e.AdminUser)
			if tc.made {
				if _, err := owner.Exec(context.Background(), `
					INSERT INTO audit_log (id, actor_type, actor_id, on_behalf_of, action, entity_type, entity_id)
					VALUES ($1, 'connector', 'connector:gcal', $2, 'create', 'activity', $3)`,
					ids.NewV7(), e.AdminUser, prior); err != nil {
					t.Fatal(err)
				}
			}
			brief, err := meetingBriefServiceAt(e, changed.Add(time.Hour)).Get(e.Admin(), meeting)
			if err != nil {
				t.Fatal(err)
			}
			said := false
			for _, section := range brief.Sections {
				for _, sentence := range section.Sentences {
					if strings.HasPrefix(sentence.Text, "Since then the deal") {
						said = true
					}
				}
			}
			if said != tc.made {
				t.Fatalf("movement since last spoke shown = %t, want %t", said, tc.made)
			}
		})
	}
}
