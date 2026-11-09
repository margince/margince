// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestDeletingAMeetingDoesNotReopenUnrelatedSignatures(t *testing.T) {
	h := setupHistory(t)
	mail := h.writes(t, 0, signed("Director"))
	h.brain.answers[mail] = statesTitle("Director")
	h.pass(t)
	at := clocktest.Now(t)
	subject := "A completed meeting"
	meeting, _, err := h.e.Activities.LogActivity(h.e.Admin(), activities.LogActivityInput{Kind: "meeting", Source: "manual", Subject: &subject, OccurredAt: &at, Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: h.contact}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.WithWorkspaceTx(h.e.Admin(), h.e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `DELETE FROM activity WHERE id=@id`, pgx.NamedArgs{"id": ids.UUID(meeting.Id)})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got := h.e.WsCount(t, `SELECT count(*) FROM contact_signature_enrich_state WHERE contact_id=@contact AND activity_id=@mail AND requested_generation=processed_generation`, pgx.NamedArgs{"contact": h.contact, "mail": mail}); got != 1 {
		t.Fatal("meeting deletion invalidated an unrelated signature")
	}
}
