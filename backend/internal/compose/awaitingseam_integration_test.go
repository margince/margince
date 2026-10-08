// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedFollowUps writes a customer, a message the admin sent them and a
// meeting the admin held with them. Both are three days old and unanswered.
func seedFollowUps(t *testing.T, e *integration.Env) (sent, met ids.UUID) {
	t.Helper()
	sent, met = ids.NewV7(), ids.NewV7()
	contact, company := ids.NewV7(), ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		for _, stmt := range []struct {
			sql  string
			args []any
		}{
			{
				`INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Contact A', 'seed', 'system')`,
				[]any{contact},
			},
			{`INSERT INTO company (id, display_name, lifecycle, source, captured_by)
			  VALUES ($1, 'Company A', 'customer', 'seed', 'system')`, []any{company}},
			{`INSERT INTO relationship (kind, contact_id, company_id, source, captured_by)
			  VALUES ('employment', $1, $2, 'seed', 'system')`, []any{contact, company}},
			{`INSERT INTO activity (id, kind, direction, subject, body, occurred_at, thread_key, source, captured_by)
			  VALUES ($1, 'email', 'outbound', 'The proposal', 'Here it is.', now() - interval '3 days',
			          $2, 'seed', 'system')`, []any{sent, "thread-" + sent.String()}},
			{
				`INSERT INTO activity_participant (activity_id, role, user_id) VALUES ($1, 'from', $2)`,
				[]any{sent, e.AdminUser},
			},
			{
				`INSERT INTO activity (id, kind, subject, occurred_at, duration_seconds, meeting_status, host_user_id, source, captured_by)
			  VALUES ($1, 'meeting', 'Discovery workshop', now() - interval '3 days', 3600, 'held', $2, 'seed', 'system')`,
				[]any{met, e.AdminUser},
			},
			{
				`INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2), ($3, 'contact', $2)`,
				[]any{sent, contact, met},
			},
		} {
			if _, err := tx.Exec(ctx, stmt.sql, stmt.args...); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding the follow-ups: %v", err)
	}
	return sent, met
}

func awaited(rows []attention.AwaitedReply, id ids.UUID) *attention.AwaitedReply {
	for i := range rows {
		if rows[i].ActivityID == id {
			return &rows[i]
		}
	}
	return nil
}

func TestTheFollowUpSeamNamesTheSentMessageAndTheMeeting(t *testing.T) {
	e := integration.Setup(t)
	sent, met := seedFollowUps(t, e)
	seam := attentionAwaiting{store: activities.NewStore(e.DB())}

	replies, cut, err := seam.AwaitingReplies(e.Admin(), time.Now())
	if err != nil || cut {
		t.Fatalf("reading follow-ups: cut=%v err=%v", cut, err)
	}
	reply := awaited(replies, sent)
	if reply == nil {
		t.Fatalf("the unanswered message did not reach the lane; %d row(s) came back", len(replies))
	}
	if reply.EmailSummary == nil {
		t.Error("the follow-up carried no email summary, so the composer has nothing to draft from")
	}

	meetings, cut, err := seam.MeetingFollowUps(e.Admin(), time.Now())
	if err != nil || cut {
		t.Fatalf("reading meeting follow-ups: cut=%v err=%v", cut, err)
	}
	meeting := awaited(meetings, met)
	if meeting == nil {
		t.Fatalf("the meeting with nothing sent since did not reach the lane; %d row(s) came back", len(meetings))
	}
	if meeting.EmailSummary != nil || meeting.ContactID.IsZero() {
		t.Errorf("the meeting row = %+v, want its contact and no email summary", *meeting)
	}
}
