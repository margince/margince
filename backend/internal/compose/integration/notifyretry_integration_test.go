// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

const notifyRetryHandler = "notify_retry_probe"

// flakyNotifier fails its first calls with an infrastructure-shaped error and
// then writes through the real notices store.
// The retry is so proven against the notice writer a production firing uses.
type flakyNotifier struct {
	store    *notices.Store
	failures int
	// refusal, when set, is returned instead of the generic fault.
	refusal error
	calls   int
}

func (n *flakyNotifier) Notify(
	ctx context.Context, recipient ids.UUID, subject, body string, target datasource.EntityRef, dedupe string, _ *crmcontracts.NoticeOrigin,
) error {
	n.calls++
	if n.refusal != nil {
		return n.refusal
	}
	if n.failures > 0 {
		n.failures--
		return errors.New("connection reset by peer")
	}
	_, err := n.store.Create(ctx, notices.NewNotice{
		Recipient: ids.From[ids.UserKind](recipient),
		Kind:      "automation",
		DedupeKey: dedupe,
		Subject:   subject,
		Body:      body,
		Target:    notices.Target{Type: string(target.Type), ID: target.ID},
	})
	return err
}

// notifyRetryProbe plans one notify action through the engine's own apply path.
type notifyRetryProbe struct {
	notifier  automation.Notifier
	recipient ids.UUID
}

func (notifyRetryProbe) Spec() workflow.Spec {
	return workflow.Spec{
		Name:                    notifyRetryHandler,
		Trigger:                 workflow.Trigger{EventType: "deal.stage_changed"},
		Tier:                    mcp.TierAutoExecute,
		ReplaysAfterFailedApply: true,
	}
}

func (notifyRetryProbe) Match(context.Context, workflow.Event) (bool, error) { return true, nil }

func (p notifyRetryProbe) Plan(_ context.Context, ev workflow.Event) (workflow.Effect, error) {
	args, err := json.Marshal(map[string]any{
		"recipient": p.recipient, "subject": "Deal moved", "body": "A deal changed stage.",
		"dedupe_key": "notify_retry:" + ev.ID.String(),
	})
	if err != nil {
		return workflow.Effect{}, err
	}
	return workflow.Effect{Actions: []workflow.Action{{Kind: workflow.ActionNotify, Target: ev.Entity, Args: args}}}, nil
}

func (p notifyRetryProbe) Apply(ctx context.Context, _ workflow.Event, eff workflow.Effect, _ *workflow.ApprovalToken) (workflow.RunResult, error) {
	applied, err := automation.ApplyActions(ctx, automation.Executors{Notifier: p.notifier}, eff)
	return workflow.RunResult{Applied: applied}, err
}

func (notifyRetryProbe) IdempotencyKey(ev workflow.Event) string {
	return notifyRetryHandler + ":" + ev.ID.String()
}

// runNotifyProbe delivers one event the given number of times and answers the
// first delivery's error.
func runNotifyProbe(t *testing.T, e *Env, notifier automation.Notifier, recipient ids.UUID, deliveries int) error {
	t.Helper()
	pipeline, open, _ := DealFixture(t, e)
	dealID := e.SeedDeal(t, "Notify Retry Deal", pipeline, open, nil)
	engine := compose.NewWorkflowEngine(e.DB())
	engine.RegisterSystemWorkflow(notifyRetryProbe{notifier: notifier, recipient: recipient})
	event := kevents.Envelope{
		EventID: ids.NewV7(), Type: "deal.stage_changed", OccurredAt: clocktest.Now(t).UTC(),
		Entity: kevents.EntityRef{Type: "deal", ID: dealID},
	}
	var first error
	for i := range deliveries {
		if err := engine.HandleEvent(context.Background(), event); i == 0 {
			first = err
		}
	}
	return first
}

func notifyRunAndNoticeCount(t *testing.T, e *Env, recipient ids.UUID) (status string, notices int) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(context.Background(),
			`SELECT status FROM workflow_run WHERE handler = $1`, notifyRetryHandler).Scan(&status); err != nil {
			return err
		}
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM notice WHERE recipient_user_id = $1 AND kind = 'automation'`, recipient).Scan(&notices)
	}); err != nil {
		t.Fatal(err)
	}
	return status, notices
}

func TestATransientNoticeFailureIsRetriedOnRedeliveryAndTheNoticeLandsOnce(t *testing.T) {
	e := Setup(t)
	notifier := &flakyNotifier{store: notices.NewStore(e.DB()), failures: 1}
	if err := runNotifyProbe(t, e, notifier, e.Rep1, 3); err == nil {
		t.Error("the first delivery must surface the notice failure so the bus redelivers")
	}

	if notifier.calls != 2 {
		t.Errorf("notify calls = %d, want 2: the failed one and the retry, and none for the delivery after success", notifier.calls)
	}
	status, count := notifyRunAndNoticeCount(t, e, e.Rep1)
	if status != "applied" {
		t.Errorf("run status = %q, want applied once the redelivery succeeded", status)
	}
	if count != 1 {
		t.Errorf("notices recorded = %d, want exactly 1 across a failed delivery, a retry and a replay", count)
	}
}

func TestANoticeForAnInactiveRecipientIsRecorded(t *testing.T) {
	e := Setup(t)
	if _, err := OwnerConn(t).Exec(context.Background(), `UPDATE app_user SET status = 'deactivated' WHERE id = $1`, e.Rep2); err != nil {
		t.Fatalf("deactivating the recipient: %v", err)
	}
	notifier := &flakyNotifier{store: notices.NewStore(e.DB())}
	if err := runNotifyProbe(t, e, notifier, e.Rep2, 1); err != nil {
		t.Fatalf("delivering: %v", err)
	}

	status, count := notifyRunAndNoticeCount(t, e, e.Rep2)
	if status != "applied" || count != 1 {
		t.Errorf("run status = %q, notices = %d; want applied and the notice recorded for the inactive seat", status, count)
	}
}

func TestARefusedNoticeStaysFailedAndIsNotRetriedByRedelivery(t *testing.T) {
	e := Setup(t)
	notifier := &flakyNotifier{store: notices.NewStore(e.DB()), refusal: apperrors.ErrNotFound}
	if err := runNotifyProbe(t, e, notifier, e.Rep1, 3); err == nil {
		t.Error("the first delivery must surface the refusal")
	}

	status, count := notifyRunAndNoticeCount(t, e, e.Rep1)
	if status != "failed" || count != 0 || notifier.calls != 1 {
		t.Errorf("status = %q, notices = %d, notify calls = %d; want failed, 0 and 1: a refusal answers the same way again", status, count, notifier.calls)
	}
}
