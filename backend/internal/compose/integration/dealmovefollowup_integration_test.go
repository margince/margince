// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A cached follow-up move is checked again on every queue read. Once the
// reader has written to an attendee, the meeting is no longer owed and the
// stored suggestion must not reach the Worklist.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/dealstatus"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestACachedFollowUpLeavesTheQueueOnceSomethingIsSent(t *testing.T) {
	e := SetupSearch(t)
	now := clocktest.Now(t)

	pipeline := e.SeedID(t, `INSERT INTO pipeline (id, name, is_default, position)
		VALUES ($1, 'Sales', true, 0)`)
	stage := e.SeedID(t, `INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
		VALUES ($1, $2, 'Qualify', 0, 'open', 10)`, pipeline)
	deal := e.SeedID(t, `INSERT INTO deal (id, owner_id, name, status, pipeline_id, stage_id, source, captured_by)
		VALUES ($1, $2, 'PIM-Rollout Phase 2', 'open', $3, $4, 'manual', 'human:x')`, e.Rep1, pipeline, stage)
	attendee := e.SeedID(t, `INSERT INTO contact (id, owner_id, full_name, source, captured_by, visibility)
		VALUES ($1, $2, 'Annabelle Malherbe', 'manual', 'human:x', 'workspace')`, e.Rep1)
	reader := dealAndContactReader(e)
	readerID := readerOf(reader, t)
	meeting := ids.NewV7()
	seedCachedCard(t, e, readerID, deal, crmcontracts.DealStatusCardMove{
		Action: "draft_email",
		Reason: "You met 3 days ago and nothing has been sent since. Draft the follow-up.",
		Arguments: &map[string]any{
			"activity_id": meeting.String(),
			"links":       []any{map[string]any{"entity_type": "contact", "entity_id": attendee.String()}},
		},
	})
	ownerExec(t, e, `INSERT INTO activity (id, kind, subject, occurred_at, duration_seconds, meeting_status, host_user_id, source, captured_by)
		VALUES ($1, 'meeting', 'Pricing review', $2::timestamptz - interval '3 days', 3600, 'held', $3, 'seed', 'system')`,
		meeting, now, readerID)
	ownerExec(t, e, `INSERT INTO activity_link (id, activity_id, entity_type, deal_id)
		VALUES ($1, $2, 'deal', $3)`, ids.NewV7(), meeting, deal)
	ownerExec(t, e, `INSERT INTO activity_link (id, activity_id, entity_type, contact_id)
		VALUES ($1, $2, 'contact', $3)`, ids.NewV7(), meeting, attendee)
	svc := dealstatus.NewService(e.Pool, nil, activities.NewStore(e.DB()), nil, func() time.Time { return now })

	before, err := svc.CachedMoves(reader, []ids.UUID{deal})
	if err != nil {
		t.Fatalf("reading the cached moves: %v", err)
	}
	if _, present := before[deal]; !present {
		t.Fatal("the follow-up still owed did not reach the queue, so the check below proves nothing")
	}

	sent := ids.NewV7()
	ownerExec(t, e, `INSERT INTO activity (id, kind, direction, subject, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'outbound', 'Thanks for today', $2::timestamptz - interval '1 hour', 'seed', 'system')`,
		sent, now)
	ownerExec(t, e, `INSERT INTO activity_participant (id, activity_id, contact_id, role)
		VALUES ($1, $2, $3, 'to')`, ids.NewV7(), sent, attendee)

	after, err := svc.CachedMoves(reader, []ids.UUID{deal})
	if err != nil {
		t.Fatalf("reading the cached moves after the mail: %v", err)
	}
	if move, present := after[deal]; present {
		t.Fatalf("the follow-up survived the mail that settled it: %+v", move)
	}
}

func ownerExec(t *testing.T, e *SearchEnv, sql string, args ...any) {
	t.Helper()
	if _, err := e.Owner.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("seeding: %v", err)
	}
}
