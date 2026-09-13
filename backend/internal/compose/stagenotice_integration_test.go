// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/compose/integration"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const stageNoticeRepair = "1789335792_stage_notices_recover_the_change_they_report.up.sql"

func TestStageNoticeRecoversItsExactMoveAndLeavesUnlinkedNoticesAlone(t *testing.T) {
	e := integration.Setup(t)
	conn, err := pgx.Connect(context.Background(), os.Getenv("MARGINCE_TEST_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	ws, actor, pool := e.WS, ids.From[ids.UserKind](e.Rep1), e.Pool
	ctx := principal.WithWorkspaceID(context.Background(), ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: "system:test"})
	db := database.BindTo(pool, ids.From[ids.WorkspaceKind](ws))
	store := deals.NewStore(db, deals.Installation{})
	pipeline, err := store.CreatePipeline(ctx, deals.CreatePipelineInput{Name: "Sales"})
	if err != nil {
		t.Fatal(err)
	}
	from, err := store.CreateStage(ctx, deals.CreateStageInput{PipelineID: ids.From[ids.PipelineKind](ids.UUID(pipeline.Id)), Name: "Qualified", Position: 1, Semantic: "open"})
	if err != nil {
		t.Fatal(err)
	}
	to, err := store.CreateStage(ctx, deals.CreateStageInput{PipelineID: ids.From[ids.PipelineKind](ids.UUID(pipeline.Id)), Name: "Proposal", Position: 2, Semantic: "open"})
	if err != nil {
		t.Fatal(err)
	}
	deal, err := store.CreateDeal(ctx, deals.CreateDealInput{Name: "Fleet renewal", PipelineID: ids.From[ids.PipelineKind](ids.UUID(pipeline.Id)), StageID: ids.From[ids.StageKind](ids.UUID(from.Id)), OwnerID: &actor, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	human := principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + actor.String(), UserID: actor.UUID, Permissions: principal.Permissions{RoleKeys: []string{"admin"}, RowScope: principal.RowScopeAll, Objects: map[string]principal.ObjectGrant{"deal": {Read: true, Update: true}}}})
	if _, err := store.AdvanceDeal(human, ids.From[ids.DealKind](ids.UUID(deal.Id)), deals.AdvanceDealInput{ToStageID: ids.From[ids.StageKind](ids.UUID(to.Id))}); err != nil {
		t.Fatal(err)
	}
	var event events.Envelope
	if err := conn.QueryRow(ctx, `SELECT envelope FROM event_outbox WHERE envelope->>'type' = 'deal.stage_changed'`).Scan(&event); err != nil {
		t.Fatal(err)
	}
	// Rewind only the fields old event writers did not record. The causation
	// chain and actor still come from the production domain and notice writers.
	if _, err := conn.Exec(ctx, `UPDATE event_outbox SET envelope = jsonb_set(envelope, '{payload}', (envelope->'payload') - 'from_stage_name' - 'to_stage_name') WHERE envelope->>'event_id' = $1`, event.EventID.String()); err != nil {
		t.Fatal(err)
	}
	noticeStore := notices.NewStore(db)
	in := notices.NewNotice{Recipient: actor, Kind: "automation", Subject: "A deal you own changed stage", Body: "Fleet renewal moved to a new pipeline stage."}
	linked, err := noticeStore.Create(principal.WithCausationEvent(ctx, event.EventID), in)
	if err != nil {
		t.Fatal(err)
	}
	unlinked, err := noticeStore.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	// A later rename cannot be presented as the earlier name.
	if _, err := conn.Exec(ctx, `UPDATE stage SET name = 'Renamed', updated_at = $1::timestamptz + interval '1 hour' WHERE id = $2`, event.OccurredAt, to.Id); err != nil {
		t.Fatal(err)
	}
	repair, err := os.ReadFile("../../migrations/core/" + stageNoticeRepair)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := conn.Exec(ctx, string(repair)); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := noticeStore.UnreadFor(human, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("repair changed delivery count: %d", len(rows))
	}
	for _, row := range rows {
		switch row.ID {
		case linked:
			if row.Subject != "Fleet renewal" || row.Target.ID != ids.UUID(deal.Id) || row.Origin == nil {
				t.Fatalf("missing recovered record: %+v", row)
			}
			if row.Origin.ActorId != event.Actor.ID || !row.Origin.OccurredAt.Equal(event.OccurredAt) || ids.UUID(row.Origin.EventId) != event.EventID {
				t.Fatalf("lost original actor/event: %+v", row.Origin)
			}
			if row.Origin.StageChange == nil || row.Origin.StageChange.FromName == nil || *row.Origin.StageChange.FromName != "Qualified" || row.Origin.StageChange.ToName != nil {
				t.Fatalf("invented historical stage names: %+v", row.Origin.StageChange)
			}
		case unlinked:
			if row.Origin != nil || row.Target.Named() || row.Subject != in.Subject || row.Body != in.Body {
				t.Fatalf("guessed an unlinked notice: %+v", row)
			}
		default:
			t.Fatalf("unexpected notice: %s", row.ID)
		}
	}
}
