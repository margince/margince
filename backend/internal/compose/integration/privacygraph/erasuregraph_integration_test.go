// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacygraph

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestErasureRefoldsARelinkedActivityBeforeItsEventIsDelivered(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	oldContact := e.SeedContact(t, "Earlier contact", nil)
	subject := e.SeedContact(t, "Erasure requester", nil)
	activityID := seedMeetingEdge(t, e, oldContact)
	edgeCount := func() int {
		return countGraphRows(t, e, `SELECT count(*) FROM graph_interaction_edge WHERE contact_id=@contact`, pgx.NamedArgs{"contact": oldContact})
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
	if err := compose.ComposedEraser(e.Pool, nil, nil).WithEdgeInvalidator(nil).EraseContact(ctx, subject, "subject request"); err == nil {
		t.Fatal("erasure cancelled a pending graph update without its invalidator")
	}
	broken := errors.New("graph correction unavailable")
	failing := compose.ComposedEraser(e.Pool, nil, nil).WithEdgeInvalidator(func(context.Context, pgx.Tx, []ids.UUID) error { return broken })
	if err := failing.EraseContact(ctx, subject, "subject request"); !errors.Is(err, broken) {
		t.Fatalf("failed graph correction=%v", err)
	}
	if got := countGraphRows(t, e, `SELECT count(*) FROM contact WHERE id=@id AND full_name='Erasure requester'`, pgx.NamedArgs{"id": subject}); got != 1 {
		t.Fatal("graph failure committed part of the erasure")
	}
	if err := compose.ComposedEraser(e.Pool, nil, nil).EraseContact(ctx, subject, "subject request"); err != nil {
		t.Fatal(err)
	}
	if got := edgeCount(); got != 0 {
		t.Fatalf("earlier contact's stale edge survived the erasure: %d", got)
	}
	if got := countGraphRows(t, e, `SELECT count(*) FROM event_outbox WHERE envelope->>'type'='activity.updated' AND envelope->'entity'->>'id'=@id AND published_at IS NULL`,
		pgx.NamedArgs{"id": activityID.String()}); got != 0 {
		t.Fatalf("erased source still has %d pending update events", got)
	}
}

func TestErasureByARepRefusesAColleaguesContactAndKeepsItsEdge(t *testing.T) {
	e := integration.Setup(t)
	colleagues := e.SeedContact(t, "Colleague's contact", &e.Rep2)
	seedMeetingEdge(t, e, colleagues)
	// The rep holds contact.delete, so only the row's write authority can
	// refuse: every rep reads a colleague's contact, which makes this a 403.
	rep := integration.OnSeat(e.As(e.Rep1, nil, principal.Permissions{
		Objects:  map[string]principal.ObjectGrant{"contact": {Read: true, Delete: true}},
		RowScope: principal.RowScopeOwn,
	}), principal.SeatFull)
	if err := compose.ComposedEraser(e.Pool, nil, nil).EraseContact(rep, colleagues, "subject request"); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a rep's erasure of a colleague's contact answered %v, want permission denied", err)
	}
	if got := countGraphRows(t, e, `SELECT count(*) FROM contact WHERE id=@id AND full_name='Colleague''s contact'`, pgx.NamedArgs{"id": colleagues}); got != 1 {
		t.Fatal("the refused erasure anonymized the contact")
	}
	if got := countGraphRows(t, e, `SELECT count(*) FROM graph_interaction_edge WHERE contact_id=@contact`, pgx.NamedArgs{"contact": colleagues}); got != 1 {
		t.Fatalf("the refused erasure changed the contact's edges: %d", got)
	}
}

// seedMeetingEdge logs a held meeting with contact as admin and folds it into
// the interaction graph, so the contact starts with one edge.
func seedMeetingEdge(t *testing.T, e *integration.Env, contact ids.UUID) ids.UUID {
	t.Helper()
	ctx := e.Admin()
	host := ids.From[ids.UserKind](e.AdminUser)
	meeting, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{
		Kind: "meeting", Subject: new("Review"), Body: new("Agreed next steps"), Source: "manual",
		OccurredAt: new(time.Date(2020, 7, 15, 12, 0, 0, 0, time.UTC)), MeetingStatus: new("held"), HostUserID: &host,
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
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
	return activityID
}

func countGraphRows(t *testing.T, e *integration.Env, query string, args pgx.NamedArgs) int {
	t.Helper()
	var count int
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error { return tx.QueryRow(e.Admin(), query, args).Scan(&count) }); err != nil {
		t.Fatal(err)
	}
	return count
}
