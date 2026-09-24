// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// One message the models decline to judge — a provider's safety filter
// withholding the answer, say — is that message's outcome, not the sweep's. Each
// batched sweep below splits a declined batch into single calls, leaves the
// declined message in its own unjudged state and judges the rest, so one hostile
// message cannot hold a workspace's whole backlog on every tick.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// hostileMarker is the text a provider's filter refuses to judge.
const hostileMarker = "HOSTILE-PAYLOAD"

// withholdingWhere withholds every answer to a request carrying hostileMarker
// and hands every other request to inner, the way a safety filter scoped to one
// message's content behaves.
type withholdingWhere struct {
	inner    completer
	withheld int
}

func (b *withholdingWhere) Complete(ctx context.Context, req model.Request) (model.Response, error) {
	for _, message := range req.Messages {
		if strings.Contains(message.Content, hostileMarker) {
			b.withheld++
			return model.Response{}, fmt.Errorf("ai: provider: finish_reason SAFETY: %w", model.ErrOutputWithheld)
		}
	}
	return b.inner.Complete(ctx, req)
}

func TestTheOwedPassJudgesPastAMessageTheModelsDecline(t *testing.T) {
	e := integration.Setup(t)
	hostile := seedWaitingMail(t, e, "Invoice "+hostileMarker)
	ordinary := seedWaitingMail(t, e, "Monatsreporting Juli")

	brain := &withholdingWhere{inner: &owedBrainStub{verdict: activities.OwedVerdictInformsUs, confidence: 0.95}}
	runOwedWorker(t, e, brain)

	if brain.withheld == 0 {
		t.Fatal("no call carried the hostile message, so this proves nothing about declining it")
	}
	if got := verdictOf(t, e, ordinary); got == nil || *got != activities.OwedVerdictInformsUs {
		t.Errorf("the message batched beside a declined one was left %v; want it judged", got)
	}
	if got := verdictOf(t, e, hostile); got != nil {
		t.Errorf("the declined message was judged %q; it stays unjudged", *got)
	}
}

func TestTheClassifyPassLabelsPastAMessageTheModelsDecline(t *testing.T) {
	e := integration.Setup(t)
	hostile := seedUnlabeledEmail(t, e, "Invoice "+hostileMarker)
	ordinary := seedUnlabeledEmail(t, e, "please send the offer")

	brain := &withholdingWhere{inner: &scriptedClassifyBrain{}}
	classifier := NewCaptureClassifier(e.Pool, brain, slog.New(slog.DiscardHandler))
	if err := classifier.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("one declined message failed the whole pass: %v", err)
	}

	if brain.withheld == 0 {
		t.Fatal("no call carried the hostile message, so this proves nothing about declining it")
	}
	if got := labelOf(t, e, ordinary); got == nil {
		t.Error("the message batched beside a declined one was left unlabeled")
	}
	if got := labelOf(t, e, hostile); got != nil {
		t.Errorf("the declined message was labeled %q; it stays unlabeled", *got)
	}
}

func TestTheSettlePassJudgesPastAConversationTheModelsDecline(t *testing.T) {
	e := integration.Setup(t)
	hostile := seedAnsweredRequest(t, e, "Invoice "+hostileMarker)
	ordinary := seedAnsweredRequest(t, e, "Monatsreporting Juli")

	brain := &withholdingWhere{inner: settlingBrain{}}
	worker := &owedVerdictWorker{pool: e.Pool, settler: NewRequestSettler(e.Pool, brain, nil, slog.New(slog.DiscardHandler))}
	if err := worker.settleWorkspace(context.Background(), e.WS); err != nil {
		t.Fatalf("one declined conversation failed the whole pass: %v", err)
	}

	if brain.withheld == 0 {
		t.Fatal("no call carried the hostile conversation, so this proves nothing about declining it")
	}
	if verdict, _ := settlementOf(t, e, ordinary); verdict != activities.RequestSettled {
		t.Errorf("the conversation batched beside a declined one was judged %q; want settled", verdict)
	}
	// Unsure is this pass's "not judged": the request stays owed and is not
	// re-read until somebody writes on the thread again.
	if verdict, decidedBy := settlementOf(t, e, hostile); verdict != activities.RequestUnsure || decidedBy != settleDeclinedBy {
		t.Errorf("the declined conversation was recorded %q by %q; want %q by %q",
			verdict, decidedBy, activities.RequestUnsure, settleDeclinedBy)
	}
}

// settlingBrain judges every fenced conversation settled.
type settlingBrain struct{}

func (settlingBrain) Complete(_ context.Context, req model.Request) (model.Response, error) {
	fenced := fencedIDs(req.System, req.Messages[0].Content, "source_id")
	if len(fenced) == 0 {
		return model.Response{}, fmt.Errorf("settle prompt fenced no conversation: %q", req.System)
	}
	results := make([]map[string]any, 0, len(fenced))
	for _, id := range fenced {
		results = append(results, map[string]any{
			"id": id, "verdict": activities.RequestSettled, "remaining": "", "due_at": "", "confidence": 0.95,
		})
	}
	payload, err := json.Marshal(map[string]any{"results": results})
	if err != nil {
		return model.Response{}, err
	}
	return model.Response{Text: string(payload)}, nil
}

// seedAnsweredRequest writes an inbound request the owed pass recognised and
// our attested reply on the same thread, which is the settlement pass's
// candidate shape.
func seedAnsweredRequest(t *testing.T, e *integration.Env, subject string) ids.UUID {
	t.Helper()
	request := ids.NewV7()
	thread := "thread-" + request.String()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, direction, subject, body, occurred_at, thread_key,
			  counterparty_email, owed_verdict, owed_verdict_at, source, captured_by)
			VALUES ($1, 'email', 'inbound', $2, 'Please send the report.', now() - interval '3 days', $3,
			  'buyer@customer.test', 'asks_us', now(), 'seed', 'system')`, request, subject, thread); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, direction, subject, body, occurred_at, thread_key,
			  counterparty_email, counterparty_outbound_attested, source, captured_by)
			VALUES ($1, 'email', 'outbound', 'Re: request', 'Attached, as asked.', now() - interval '2 days', $2,
			  'buyer@customer.test', true, 'seed', 'system')`, ids.NewV7(), thread)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

// settlementOf reads the verdict the settlement pass recorded, or empty strings.
func settlementOf(t *testing.T, e *integration.Env, request ids.UUID) (verdict, decidedBy string) {
	t.Helper()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT coalesce(max(verdict), ''), coalesce(max(decided_by), '')
			  FROM activity_request_settlement WHERE request_activity_id = $1`, request).Scan(&verdict, &decidedBy)
	})
	if err != nil {
		t.Fatalf("reading the settlement: %v", err)
	}
	return verdict, decidedBy
}
