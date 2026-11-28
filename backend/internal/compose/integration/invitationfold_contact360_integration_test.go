// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/contact360"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The contact page's recent activity folds a meeting's invitation mail onto
// the meeting, as History does, and keeps a written reply.
func TestTheContactPageFoldsAMeetingsInvitationMailOntoIt(t *testing.T) {
	e := Setup(t)
	contact := e.SeedContact(t, "Jana Novak", &e.Rep1)
	meeting, invitation, written := ids.NewV7(), ids.NewV7(), ids.NewV7()
	uid := "kickoff-" + ids.NewV7().String() + "@google.com"
	starts := clocktest.Now(t).Add(-48 * time.Hour).UTC()
	if err := database.WithTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := e.Admin()
		for _, step := range []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by) VALUES ($1, 'meeting', 'Kickoff', $2, 'seed', 'system')`, []any{meeting, starts}},
			{`INSERT INTO activity_identity (identity_kind, identity_key, activity_id, attested_by) VALUES ('meeting', $1, $2, 'seed')`, []any{activities.MeetingIdentityKey(uid, starts.Format(time.RFC3339)), meeting}},
			{`INSERT INTO activity (id, kind, direction, subject, occurred_at, source, captured_by, calendar_method, calendar_uid) VALUES ($1, 'email', 'outbound', 'Invitation: Kickoff', $2, 'seed', 'system', 'REQUEST', $3)`, []any{invitation, starts.Add(-time.Hour), uid}},
			{`INSERT INTO activity (id, kind, direction, subject, occurred_at, source, captured_by) VALUES ($1, 'email', 'inbound', 'Re: Invitation: Kickoff', $2, 'seed', 'system')`, []any{written, starts.Add(-30 * time.Minute)}},
		} {
			if _, err := tx.Exec(ctx, step.sql, step.args...); err != nil {
				return err
			}
		}
		for _, activity := range []ids.UUID{meeting, invitation, written} {
			if _, err := tx.Exec(ctx, `INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`, activity, contact); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding the meeting and its mail: %v", err)
	}

	page, err := contactRoomService(e).AssembleScoped(e.Admin(), ContactIDOf(contact), contact360.AssembleOptions{})
	if err != nil {
		t.Fatalf("assembling the contact page: %v", err)
	}
	shown := contactActivityIDs(page)
	if shown[invitation.String()] || !shown[written.String()] || !shown[meeting.String()] {
		t.Fatalf("the contact page shows %v; want the meeting and the written reply, not the invitation", shown)
	}
}
