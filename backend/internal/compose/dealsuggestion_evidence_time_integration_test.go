// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestSuggestionRepairDetectsChangedMeetingAndDocumentDates(t *testing.T) {
	for _, kind := range []string{deals.EvidenceMeeting, deals.EvidenceAttachment} {
		t.Run(kind, func(t *testing.T) {
			e := setupScout(t)
			e.now = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
			company := e.SeedCompany(t, "Example buyer", nil)
			e.buyerPair(t, company, e.daysAgo(6))
			var source ids.UUID
			if kind == deals.EvidenceMeeting {
				buyer := e.employee(t, "Meeting buyer", company)
				source = e.meeting(e.Admin(), t, "Scoping workshop", &buyer, e.daysAgo(2))
			} else {
				source = e.email(t, "Proposal", "outbound", company, e.daysAgo(2))
				e.document(t, source, "Proposal.pdf", "application/pdf")
			}
			e.pass(t)
			reader := e.signalReader()
			suggestion := e.onlySuggestion(reader, t, company)
			included := false
			for _, evidence := range suggestion.Evidence {
				included = included || evidence.Kind == kind
			}
			if !included {
				t.Fatalf("fixture omitted %s evidence", kind)
			}
			corrected := e.daysAgo(1)
			if _, err := e.Activities.UpdateActivity(e.Admin(), ids.From[ids.ActivityKind](source),
				activities.UpdateActivityInput{OccurredAt: &corrected}); err != nil {
				t.Fatal(err)
			}
			if err := e.DB().Tx(reader, func(tx pgx.Tx) error {
				repair, err := deals.PreviewSuggestionRepairTx(reader, tx, suggestion.ID, e.now, dealSuggestionEffects{}.BuyerStillVerified)
				if err != nil {
					return err
				}
				if !repair.Supersede || repair.Reason != deals.RepairReasonSourceChanged {
					t.Fatalf("changed %s date was not detected: %+v", kind, repair)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
