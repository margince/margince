// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestErasureRefoldsARelinkedActivityBeforeItsEventIsDelivered(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	oldContact := e.SeedContact(t, "Earlier person", nil)
	subject := e.SeedContact(t, "Erasure requester", nil)
	host := ids.From[ids.UserKind](e.AdminUser)
	meeting, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{
		Kind: "meeting", Subject: new("Review"), Body: new("Agreed next steps"), Source: "manual",
		OccurredAt: new(time.Date(2020, 7, 15, 12, 0, 0, 0, time.UTC)), MeetingStatus: new("held"), HostUserID: &host,
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: oldContact}},
	})
	if err != nil {
		t.Fatal(err)
	}
	activityID := ids.UUID(meeting.Id)
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return search.RecomputeEdgesForActivities(ctx, tx, []ids.UUID{activityID})
	}); err != nil {
		t.Fatal(err)
	}
	edgeCount := func() int {
		return countRows(t, e, `SELECT count(*) FROM graph_interaction_edge WHERE contact_id=@contact`, pgx.NamedArgs{"contact": oldContact})
	}
	if got := edgeCount(); got != 1 {
		t.Fatalf("original edge count=%d, want 1", got)
	}
	if _, err := e.Activities.RelinkActivity(ctx, ids.From[ids.ActivityKind](activityID),
		activities.RelinkActivityInput{EntityType: "contact", EntityID: subject, ReplaceExistingOfType: true}); err != nil {
		t.Fatal(err)
	}
	if got := edgeCount(); got != 1 {
		t.Fatalf("undelivered relink lost the stale-edge fixture: %d", got)
	}
	if err := ComposedEraser(e.Pool, nil, nil).WithEdgeInvalidator(nil).EraseContact(ctx, subject, "subject request"); err == nil {
		t.Fatal("erasure cancelled a pending graph update without its invalidator")
	}
	broken := errors.New("graph correction unavailable")
	failing := ComposedEraser(e.Pool, nil, nil).WithEdgeInvalidator(func(context.Context, pgx.Tx, []ids.UUID) error { return broken })
	if err := failing.EraseContact(ctx, subject, "subject request"); !errors.Is(err, broken) {
		t.Fatalf("failed graph correction=%v", err)
	}
	if got := countRows(t, e, `SELECT count(*) FROM contact WHERE id=@id AND full_name='Erasure requester'`, pgx.NamedArgs{"id": subject}); got != 1 {
		t.Fatal("graph failure committed part of the erasure")
	}
	if err := ComposedEraser(e.Pool, nil, nil).EraseContact(ctx, subject, "subject request"); err != nil {
		t.Fatal(err)
	}
	if got := edgeCount(); got != 0 {
		t.Fatalf("old person's stale edge survived the erasure: %d", got)
	}
	if got := countRows(t, e, `SELECT count(*) FROM event_outbox WHERE envelope->>'type'='activity.updated' AND envelope->'entity'->>'id'=@id AND published_at IS NULL`,
		pgx.NamedArgs{"id": activityID.String()}); got != 0 {
		t.Fatalf("erased source still has %d pending update events", got)
	}
}
