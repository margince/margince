// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package stagenotices

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestStageNotificationsAgreeAcrossDeliveryHistoryAndPreview(t *testing.T) {
	f := newStageNoticeFixture(t, "Qualified", "Proposal", "Renewal")
	if err := f.db.Tx(f.ctx, func(tx pgx.Tx) error { return automation.SeedStarterAutomationsTx(f.ctx, tx) }); err != nil {
		t.Fatal(err)
	}
	handler := catalogStageNotifier(t)
	automationID := reviewStageNotifier(t, f, handler)
	engine := compose.NewWorkflowEngine(f.db)
	for range 2 {
		if err := engine.HandleEvent(context.Background(), f.event); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := f.store.UnreadFor(f.human, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Origin == nil || rows[0].Origin.ActorId != f.event.Actor.ID {
		t.Fatalf("another human's move must reach the owner exactly once: %+v", rows)
	}
	assertLegacySelfStageDeliveryHidden(t, f, rows[0])

	if _, err := deals.NewStore(f.db, deals.Installation{}).AdvanceDeal(f.human, ids.From[ids.DealKind](f.dealID), deals.AdvanceDealInput{ToStageID: ids.From[ids.StageKind](f.fromID)}); err != nil {
		t.Fatal(err)
	}
	var selfMove events.Envelope
	if err := f.conn.QueryRow(f.ctx, `SELECT envelope FROM event_outbox WHERE envelope->>'type' = @type AND envelope->'entity'->>'id' = @deal AND envelope->>'event_id' <> @prior`, pgx.NamedArgs{"type": f.event.Type, "deal": f.dealID.String(), "prior": f.event.EventID.String()}).Scan(&selfMove); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := engine.HandleEvent(context.Background(), selfMove); err != nil {
			t.Fatal(err)
		}
	}
	var status, reason string
	if err := f.conn.QueryRow(f.ctx, `SELECT status, detail->>'reason' FROM workflow_run WHERE trigger_event = @event AND handler = @handler`, pgx.NamedArgs{"event": selfMove.EventID, "handler": handler}).Scan(&status, &reason); err != nil {
		t.Fatal(err)
	}
	if status != "skipped" || reason != "the owner made this stage change" {
		t.Fatalf("self move was not recorded as declined: %s %s", status, reason)
	}
	after, err := f.store.UnreadFor(f.human, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 || after[0].ID != rows[0].ID {
		t.Fatalf("self move changed the owner's unread feed: %+v", after)
	}
	var delivered int
	if err := f.conn.QueryRow(f.ctx, `SELECT count(*) FROM notice WHERE recipient_user_id = @owner`, pgx.NamedArgs{"owner": f.actor}).Scan(&delivered); err != nil {
		t.Fatal(err)
	}
	if delivered != 1 {
		t.Fatalf("self-made move created an unnecessary notice: %d deliveries", delivered)
	}
	preview, err := automation.NewAutomationStore(f.db).SetClock(func() time.Time { return selfMove.OccurredAt.Add(time.Hour) }).Preview(f.ctx, automationID, automation.AutomationPreviewInput{})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Kind != crmcontracts.AutomationPreviewKindEvent || preview.HistoricalPreviewAvailable || preview.WouldHaveFired != nil {
		t.Fatalf("an event rule's preview claimed a history count: %+v", preview)
	}
}

func catalogStageNotifier(t *testing.T) string {
	t.Helper()
	for _, entry := range automation.Catalog() {
		if entry.Trigger == "deal.stage_changed" && entry.Action == string(automation.ActionTypeNotify) {
			return entry.Key
		}
	}
	t.Fatal("no catalog stage notifier")
	return ""
}

// A seeded notifier waits for its owner's review. It acts only on moves made
// after it is switched on, so the review is dated before the fixture's move.
func reviewStageNotifier(t *testing.T, f stageNoticeFixture, handler string) ids.AutomationID {
	t.Helper()
	var rule automation.Automation
	if err := f.conn.QueryRow(f.ctx, `SELECT id, version FROM automation WHERE key = @key`, pgx.NamedArgs{"key": handler}).Scan(&rule.ID, &rule.Version); err != nil {
		t.Fatal(err)
	}
	var pipeline ids.UUID
	if err := f.conn.QueryRow(f.ctx, `SELECT pipeline_id FROM stage WHERE id = @stage`, pgx.NamedArgs{"stage": f.fromID}).Scan(&pipeline); err != nil {
		t.Fatal(err)
	}
	// The reviewer becomes the rule's owner, whose live authority each firing re-checks.
	f.env.GrantRole(t, f.env.AdminUser, "admin")
	authority, err := identity.NewService(f.env.Pool).EffectiveRBAC(f.ctx, f.env.WS, f.env.AdminUser)
	if err != nil {
		t.Fatal(err)
	}
	enabled := true
	reviewed, err := automation.NewAutomationStore(f.db).WithStageLookup(compose.NewProviderFor(f.db)).
		SetClock(func() time.Time { return f.event.OccurredAt.Add(-time.Minute) }).
		Update(f.env.As(f.env.AdminUser, authority.TeamIDs, authority.Permissions), rule.ID, automation.UpdateAutomationInput{
			Enabled:   &enabled,
			Params:    map[string]any{"pipeline_id": pipeline.String(), "stage_ids": []string{f.fromID.String(), f.toID.String()}},
			IfVersion: &rule.Version,
		})
	if err != nil {
		t.Fatal(err)
	}
	if !reviewed.Enabled {
		t.Fatalf("the reviewed notifier is still paused: %+v", reviewed)
	}
	return rule.ID
}

func assertLegacySelfStageDeliveryHidden(t *testing.T, f stageNoticeFixture, delivered notices.Notice) {
	t.Helper()
	mover, ok := principal.HumanUserID(f.event.Actor.ID)
	if !ok {
		t.Fatal("fixture event has no human mover")
	}
	reader := principal.WithActor(f.ctx, principal.Principal{Type: principal.PrincipalHuman, ID: f.event.Actor.ID, UserID: mover})
	var key string
	if err := f.conn.QueryRow(f.ctx, `SELECT dedupe_key FROM notice WHERE id = @id`, pgx.NamedArgs{"id": delivered.ID}).Scan(&key); err != nil {
		t.Fatal(err)
	}
	// The production delivery supplies the kind, target, origin and dedupe key.
	// Removing its snapshot exercises the older wire without copying its vocabulary.
	for _, historical := range []bool{false, true} {
		origin := *delivered.Origin
		suffix := ":with-snapshot"
		if historical {
			origin.StageChange = nil
			suffix = ":without-snapshot"
		}
		id, err := f.store.Create(f.ctx, notices.NewNotice{Recipient: ids.From[ids.UserKind](mover), Kind: delivered.Kind, Target: delivered.Target, Origin: &origin, DedupeKey: key + suffix, Subject: delivered.Subject, Body: delivered.Body})
		if err != nil {
			t.Fatal(err)
		}
		rows, err := f.store.UnreadFor(reader, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 0 {
			t.Fatalf("historical=%v: the producer and reader disagree about self-made moves: %+v", historical, rows)
		}
		var unread bool
		if err := f.conn.QueryRow(f.ctx, `SELECT read_at IS NULL FROM notice WHERE id = @id`, pgx.NamedArgs{"id": id}).Scan(&unread); err != nil {
			t.Fatal(err)
		}
		if !unread {
			t.Fatal("excluding a self notification changed its read state")
		}
	}
}
