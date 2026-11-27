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
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/clocktest"
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// A declined message is asked in one batch and once on its own, and then never
// again — not later in the same pass, and not on the next tick. Left in the
// backlog it would be the oldest row forever, re-read with nine fresh messages
// every iteration and splitting each batch it rode in into single calls.
func TestTheClassifyPassAsksADeclinedMessageOnceAcrossPasses(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	hostile := seedUnlabeledEmail(t, e, "Invoice "+hostileMarker)
	var ordinary []ids.UUID
	for i := range classifyBatchSize + 3 {
		ordinary = append(ordinary, seedUnlabeledEmail(t, e, fmt.Sprintf("offer %d", i)))
	}
	brain := &withholdingWhere{inner: &scriptedClassifyBrain{}}
	classifier := NewCaptureClassifier(e.Pool, brain, slog.New(slog.DiscardHandler))
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	for pass := 1; pass <= 2; pass++ {
		if err := classifier.RunWorkspace(ctx, 0); err != nil {
			t.Fatalf("pass %d: %v", pass, err)
		}
		if brain.withheld != 2 {
			t.Fatalf("after pass %d the declined message had been sent %d times, want 2 — its batch and itself, once", pass, brain.withheld)
		}
	}
	for _, id := range ordinary {
		if labelOf(t, e, id) == nil {
			t.Errorf("message %s was left unlabeled beside a declined one", id)
		}
	}
	if labelOf(t, e, hostile) != nil {
		t.Error("the declined message was labeled; it stays unlabeled")
	}
}

func TestTheOwedPassAsksADeclinedMessageOnceAcrossPasses(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	hostile := seedWaitingMail(t, e, "Invoice "+hostileMarker)
	var ordinary []ids.UUID
	for i := range owedBatchSize + 3 {
		ordinary = append(ordinary, seedWaitingMail(t, e, fmt.Sprintf("Monatsreporting %d", i)))
	}
	brain := &withholdingWhere{inner: &owedBrainStub{verdict: activities.OwedVerdictInformsUs, confidence: 0.95}}
	for pass := 1; pass <= 2; pass++ {
		runOwedWorker(t, e, brain)
		if brain.withheld != 2 {
			t.Fatalf("after pass %d the declined message had been sent %d times, want 2 — its batch and itself, once", pass, brain.withheld)
		}
	}
	for _, id := range ordinary {
		if verdictOf(t, e, id) == nil {
			t.Errorf("message %s was left unjudged beside a declined one", id)
		}
	}
	if verdictOf(t, e, hostile) != nil {
		t.Error("the declined message was judged; it stays unjudged")
	}
}

// unsureMarker is the subject text a scripted model answers below the floor,
// in a batch and on its own.
const unsureMarker = "UNSURE-ABOUT-THIS"

// sentCarrying counts the user turns that carried text.
func sentCarrying(prompts []string, text string) int {
	n := 0
	for _, p := range prompts {
		if strings.Contains(p, text) {
			n++
		}
	}
	return n
}

// A message whose solo answer stays below the floor is asked in one batch and
// once alone. It is not asked again under the same prompt.
func TestTheClassifyPassAsksAnUnsureMessageOnceAcrossPasses(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	unsure := seedUnlabeledEmail(t, e, "Note "+unsureMarker)
	ordinary := seedUnlabeledEmail(t, e, "please send the offer")
	brain := &scriptedClassifyBrain{confidence: map[string]float64{"Note " + unsureMarker: 0.3}, soloStaysLow: true}
	classifier, ctx := classifyPassFor(e, brain)
	for pass := 1; pass <= 2; pass++ {
		if err := classifier.RunWorkspace(ctx, 0); err != nil {
			t.Fatalf("pass %d: %v", pass, err)
		}
		if sent := sentCarrying(brain.prompts, unsureMarker); sent != 2 {
			t.Fatalf("after pass %d the unsure message had been sent %d times, want 2 — its batch and itself, once", pass, sent)
		}
	}
	if labelOf(t, e, unsure) != nil {
		t.Error("an answer below the floor was stored as a label")
	}
	if labelOf(t, e, ordinary) == nil {
		t.Error("the message batched beside an unsure one was left unlabeled")
	}
}

func TestTheOwedPassAsksAnUnsureMessageOnceAcrossPasses(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	unsure := seedWaitingMail(t, e, "Note "+unsureMarker)
	ordinary := seedWaitingMail(t, e, "Monatsreporting Juli")
	brain := &owedBrainStub{verdict: activities.OwedVerdictAsksUs, confidence: 0.95, doubtful: unsureMarker}
	for pass := 1; pass <= 2; pass++ {
		runOwedWorker(t, e, brain)
		turns := make([]string, 0, len(brain.prompts))
		for _, req := range brain.prompts {
			turns = append(turns, req.Messages[0].Content)
		}
		if sent := sentCarrying(turns, unsureMarker); sent != 2 {
			t.Fatalf("after pass %d the unsure message had been sent %d times, want 2 — its batch and itself, once", pass, sent)
		}
	}
	if got := verdictOf(t, e, unsure); got != nil {
		t.Errorf("an answer below the floor was stored as the verdict %q", *got)
	}
	if verdictOf(t, e, ordinary) == nil {
		t.Error("the message batched beside an unsure one was left unjudged")
	}
}

// A full batch of unsure messages at the head of the backlog does not end the
// pass. Once they are stamped, the newer message behind them is labeled.
func TestTenUnsureMessagesDoNotHoldBackANewerOne(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	brain := &scriptedClassifyBrain{confidence: map[string]float64{}, soloStaysLow: true}
	for i := range classifyBatchSize {
		subject := fmt.Sprintf("Note %d %s", i, unsureMarker)
		brain.confidence[subject] = 0.3
		seedUnlabeledEmail(t, e, subject)
	}
	answerable := seedUnlabeledEmail(t, e, "please send the offer")
	classifier, ctx := classifyPassFor(e, brain)
	if err := classifier.RunWorkspace(ctx, 0); err != nil {
		t.Fatalf("the pass failed: %v", err)
	}
	if labelOf(t, e, answerable) == nil {
		t.Error("the message behind ten unsure ones was left unlabeled by the pass")
	}
}

// A verdict reached under a retired prompt is re-judged once. An unsure answer
// under the current one leaves the old verdict standing and is not asked again.
func TestTheRejudgeSweepAsksAnUnsureMessageOnce(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	stale := seedWaitingMail(t, e, "Note "+unsureMarker)
	store := activities.NewStore(InstallationDB(e.Pool))
	if applied, err := store.SetOwedVerdict(owedSweepContext(e), stale, activities.OwedVerdictInformsUs, retiredPrompt, clocktest.Now(t)); err != nil || !applied {
		t.Fatalf("judging the message under the retired prompt: applied=%v err=%v", applied, err)
	}
	brain := &owedBrainStub{verdict: activities.OwedVerdictAsksUs, confidence: 0.95, doubtful: unsureMarker}
	for pass := 1; pass <= 2; pass++ {
		runOwedWorker(t, e, brain)
		if sent := len(brain.prompts); sent != 2 {
			t.Fatalf("after pass %d the re-judge sweep had sent the message %d times, want 2: its batch and itself, once", pass, sent)
		}
	}
	if got := verdictOf(t, e, stale); got == nil || *got != activities.OwedVerdictInformsUs {
		t.Errorf("the verdict under the retired prompt became %v; an unsure re-judge leaves it standing", got)
	}
}

// A stamp answers for the text the model read. Once a member edits the
// subject, both sweeps ask about the message again.
func TestAnEditedMessageIsAskedAgainAfterAnUnsureAnswer(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	unlabeled := seedUnlabeledEmail(t, e, "FYI "+unsureMarker)
	waiting := seedWaitingMail(t, e, "FYI "+unsureMarker)
	classifyBrain := &scriptedClassifyBrain{confidence: map[string]float64{"FYI " + unsureMarker: 0.3}, soloStaysLow: true}
	classifier, ctx := classifyPassFor(e, classifyBrain)
	owedBrain := &owedBrainStub{verdict: activities.OwedVerdictAsksUs, confidence: 0.95, doubtful: unsureMarker}
	if err := classifier.RunWorkspace(ctx, 0); err != nil {
		t.Fatalf("the classify pass failed: %v", err)
	}
	runOwedWorker(t, e, owedBrain)
	for _, id := range []ids.UUID{unlabeled, waiting} {
		if _, err := e.Activities.UpdateActivity(e.Admin(), ids.From[ids.ActivityKind](id),
			activities.UpdateActivityInput{Subject: new("Please approve the contract today")}); err != nil {
			t.Fatalf("editing the subject: %v", err)
		}
	}
	if err := classifier.RunWorkspace(ctx, 0); err != nil {
		t.Fatalf("the classify pass failed: %v", err)
	}
	runOwedWorker(t, e, owedBrain)
	if labelOf(t, e, unlabeled) == nil {
		t.Error("the edited message stayed out of the classify backlog")
	}
	if verdictOf(t, e, waiting) == nil {
		t.Error("the edited message stayed out of the owed backlog")
	}
}

// standInBrain is the offline stand-in an installation runs on before a
// provider is bound: it answers, and every answer is refused.
type standInBrain struct{ calls int }

func (b *standInBrain) Complete(context.Context, model.Request) (model.Response, error) {
	b.calls++
	return model.Response{}, fmt.Errorf("%w: %w", ai.ErrUnconfiguredModel, ai.ErrOutputRejected)
}

// The stand-in declines everything because nothing is bound, which says nothing
// about any message. Recording it as the models' decline would strand the whole
// backlog on the day a provider is bound, so the pass stops instead.
func TestAPassWithNoProviderBoundDeclinesNothing(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	waiting := seedWaitingMail(t, e, "Monatsreporting Juli")
	unlabeled := seedUnlabeledEmail(t, e, "please send the offer")

	brain := &standInBrain{}
	runOwedWorker(t, e, brain)
	classifier := NewCaptureClassifier(e.Pool, brain, slog.New(slog.DiscardHandler))
	if err := classifier.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("a pass with no provider bound failed rather than waiting: %v", err)
	}
	if brain.calls != 2 {
		t.Errorf("the stand-in was asked %d times, want once per pass — nothing it answers is worth a re-ask", brain.calls)
	}
	var owedDeclined, labelDeclined bool
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT (SELECT owed_verdict_declined_at IS NOT NULL FROM activity WHERE id = $1),
			       (SELECT capture_label_declined_at IS NOT NULL FROM activity WHERE id = $2)`,
			waiting, unlabeled).Scan(&owedDeclined, &labelDeclined)
	})
	if err != nil {
		t.Fatalf("reading the decline stamps: %v", err)
	}
	if owedDeclined || labelDeclined {
		t.Errorf("the stand-in's refusal was recorded as a decline (owed %v, label %v)", owedDeclined, labelDeclined)
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
