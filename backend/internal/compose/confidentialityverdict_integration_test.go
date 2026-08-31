// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What the confidentiality engine does to a held thread, end to end.
//
// Under the classified posture every captured message is born held, and this
// engine is what makes that livable: it opens the ordinary conversations so
// what stays private is the correspondence that had a reason to. The tests
// below are the two halves of that promise — an ordinary thread becomes
// readable, and everything else does not.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func TestAnOrdinaryThreadIsOpenedForTheWorkspace(t *testing.T) {
	e := integration.Setup(t)
	activityID := seedHeldThreadMail(t, e, "thread-ordinary", "einkauf@kunde.example", "Nachbestellung")
	threadID := seedThreadQuestion(t, e, "thread-ordinary", activityID)

	runConfidentiality(t, e, threadID, confidentialityOrdinary, 0.95)

	if got := threadStatus(t, e, threadID); got != capture.VerdictCleared {
		t.Fatalf("thread status = %q, want cleared", got)
	}
	if got := activityAudience(t, e, activityID); got != "workspace" {
		t.Fatalf("an ordinary thread's message is %q, want workspace — the whole point of the engine "+
			"is that a classified mailbox does not stay invisible to the team", got)
	}
}

func TestASensitiveThreadStaysHeldAndNamesWhy(t *testing.T) {
	e := integration.Setup(t)
	activityID := seedHeldThreadMail(t, e, "thread-personnel", "kanzlei@example.test", "Unterlagen")
	threadID := seedThreadQuestion(t, e, "thread-personnel", activityID)

	runConfidentiality(t, e, threadID, confidentialityPersonnel, 0.95)

	if got := threadStatus(t, e, threadID); got != capture.VerdictHeld {
		t.Fatalf("thread status = %q, want held", got)
	}
	if got := activityAudience(t, e, activityID); got != "participants" {
		t.Fatalf("a personnel thread's message is %q, want participants", got)
	}
}

func TestAnOpeningAnswerBelowTheFloorHoldsTheThread(t *testing.T) {
	e := integration.Setup(t)
	activityID := seedHeldThreadMail(t, e, "thread-unsure", "someone@example.test", "Frage")
	threadID := seedThreadQuestion(t, e, "thread-unsure", activityID)

	// `ordinary` is the one answer that opens, so it is the one answer that has
	// to clear a floor. A holding answer needs none: requiring confidence to
	// hold would publish exactly the threads the model found hardest.
	runConfidentiality(t, e, threadID, confidentialityOrdinary, 0.6)

	if got := threadStatus(t, e, threadID); got != capture.VerdictUnsure {
		t.Fatalf("thread status = %q, want unsure — a below-floor opening answer is not believed", got)
	}
	if got := activityAudience(t, e, activityID); got != "participants" {
		t.Fatalf("a thread the model was unsure about is %q, want participants", got)
	}
}

func TestAHoldingAnswerNeedsNoConfidence(t *testing.T) {
	e := integration.Setup(t)
	activityID := seedHeldThreadMail(t, e, "thread-lowlegal", "counsel@example.test", "Sache")
	threadID := seedThreadQuestion(t, e, "thread-lowlegal", activityID)

	runConfidentiality(t, e, threadID, confidentialityLegal, 0.3)

	if got := threadStatus(t, e, threadID); got != capture.VerdictHeld {
		t.Fatalf("thread status = %q, want held even at 0.3 — the floor guards opening, not holding", got)
	}
}

// runConfidentiality drives the real engine over one thread with a scripted
// answer, so what is under test is the engine's own apply path rather than a
// test's idea of it.
func runConfidentiality(t *testing.T, e *integration.Env, threadID ids.UUID, kind string, confidence float64) {
	t.Helper()
	brain := &scriptedConfidentialityBrain{kind: kind, confidence: confidence, id: threadID}
	engine := NewConfidentialityVerdictEngine(e.Pool, brain, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("confidentiality pass: %v", err)
	}
}

func threadStatus(t *testing.T, e *integration.Env, id ids.UUID) string {
	t.Helper()
	var status string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT status FROM capture_thread_verdict WHERE id = $1`, id).Scan(&status)
	}); err != nil {
		t.Fatalf("reading the thread status: %v", err)
	}
	return status
}

func activityAudience(t *testing.T, e *integration.Env, id ids.UUID) string {
	t.Helper()
	var audience string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT audience FROM activity WHERE id = $1`, id).Scan(&audience)
	}); err != nil {
		t.Fatalf("reading the activity audience: %v", err)
	}
	return audience
}

// seedThreadQuestion opens the question the way capture opens it — through the
// store's own EnsureTx, not a hand-written INSERT, so what the engine claims is
// the row production would have written.
func seedThreadQuestion(t *testing.T, e *integration.Env, threadKey string, activityID ids.UUID) ids.UUID {
	t.Helper()
	store := capture.NewThreadVerdictStore(InstallationDB(e.Pool))
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return store.EnsureTx(context.Background(), tx, threadKey, e.Rep1, activityID, time.Now().Add(-time.Minute))
	}); err != nil {
		t.Fatalf("opening the thread question: %v", err)
	}
	var id ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT id FROM capture_thread_verdict WHERE thread_key = $1`, threadKey).Scan(&id)
	}); err != nil {
		t.Fatalf("reading back the thread question: %v", err)
	}
	return id
}

// scriptedConfidentialityBrain answers with a fixed kind, for the id it finds
// IN THE PROMPT rather than the one it was constructed with.
//
// Reading the id back out of the fence is the point: production takes it from a
// ledger row no model has seen, so a brain that answered from its own field
// would pass even if the request carried the wrong thread — or no thread at
// all. The constructed id is used only to check the prompt named the one this
// test meant.
type scriptedConfidentialityBrain struct {
	kind       string
	confidence float64
	id         ids.UUID
	calls      int
}

func (s *scriptedConfidentialityBrain) Complete(_ context.Context, req model.Request) (model.Response, error) {
	s.calls++
	askedFor := fencedIDs(req.System, req.Messages[0].Content, "id")
	if len(askedFor) != 1 {
		return model.Response{}, fmt.Errorf(
			"confidentiality prompt declared no data boundary, or fenced %d threads rather than one", len(askedFor))
	}
	if askedFor[0] != s.id.String() {
		return model.Response{}, fmt.Errorf(
			"the prompt asked about thread %s, not the one under test", askedFor[0])
	}
	payload, err := json.Marshal(map[string]any{"results": []map[string]any{
		{"id": askedFor[0], "verdict": s.kind, "confidence": s.confidence},
	}})
	if err != nil {
		return model.Response{}, err
	}
	return model.Response{Text: string(payload)}, nil
}

// seedHeldThreadMail lands one message on a thread, held to its participants,
// with the import row that makes this seat a contributor to its audience.
//
// The import row is not decoration: activity.audience is DERIVED across every
// seat that imported the message, so a message with no import row has no
// contributor asking for anything and the recompute would open it regardless of
// any verdict. Seeding one is what makes the audience assertions in this file
// measure the engine rather than the absence of a row.
func seedHeldThreadMail(t *testing.T, e *integration.Env, threadKey, from, subject string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, subject, body, direction, source_system, source_id,
			                      source, captured_by, counterparty_email, thread_key,
			                      audience, audience_reason)
			VALUES ($1, 'email', $2, 'the message body', 'inbound', 'gmail', $3,
			        'gmail:'||$3, 'connector:gmail', $4, $5, 'participants', 'pending_verdict')`,
			id, subject, "cnf-"+id.String(), from, threadKey); err != nil {
			return err
		}
		// The seat's own import row, carrying the posture the classified
		// mailbox imported under. This is the contribution the derivation reads.
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_import (activity_id, user_id, posture_at_import, verdict_status)
			VALUES ($1, $2, 'classified', 'pending')`, id, e.Rep1)
		return err
	}); err != nil {
		t.Fatalf("seeding a held thread message: %v", err)
	}
	return id
}

func TestOneSeatsVerdictDoesNotPublishAColleaguesHeldMessage(t *testing.T) {
	// The per-owner model, at the point where it is easiest to break. A thread
	// reaching two mailboxes is two people's correspondence: each seat gets its
	// own ledger row, each may conclude differently, and the message ends at
	// the STRICTEST of their answers.
	//
	// The stamp that writes a verdict onto import rows is one UPDATE. Drop its
	// user_id clause and it writes every seat's contribution at once, so one
	// seat's `ordinary` publishes a message their colleague's mailbox is
	// holding — and nothing about the change looks wrong.
	e := integration.Setup(t)
	activityID := seedHeldThreadMail(t, e, "thread-shared", "kunde@example.test", "Angebot")
	addImportRowFor(t, e, activityID, e.Rep2)
	threadID := seedThreadQuestion(t, e, "thread-shared", activityID)

	// Rep1's own verdict says ordinary. Rep2 has not been judged, so Rep2's
	// import row is still pending and still asks for the message to be held.
	runConfidentiality(t, e, threadID, confidentialityOrdinary, 0.95)

	if got := activityAudience(t, e, activityID); got != "participants" {
		t.Fatalf("the message is %q, want participants — one seat's ordinary verdict must not "+
			"publish a message a colleague's mailbox is still holding", got)
	}
	if got := importVerdictFor(t, e, activityID, e.Rep2); got != "pending" {
		t.Fatalf("the colleague's import row says %q, want pending — a verdict is scoped to the "+
			"seat whose thread ledger it came from", got)
	}
}

// addImportRowFor makes a second seat a contributor to one message's audience.
func addImportRowFor(t *testing.T, e *integration.Env, activityID, user ids.UUID) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_import (activity_id, user_id, posture_at_import, verdict_status)
			VALUES ($1, $2, 'classified', 'pending')`, activityID, user)
		return err
	}); err != nil {
		t.Fatalf("adding a second importer: %v", err)
	}
}

// importVerdictFor reads one seat's contribution to a message's audience.
func importVerdictFor(t *testing.T, e *integration.Env, activityID, user ids.UUID) string {
	t.Helper()
	var status string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT coalesce(verdict_status, '') FROM capture_import
			 WHERE activity_id = $1 AND user_id = $2`, activityID, user).Scan(&status)
	}); err != nil {
		t.Fatalf("reading a seat's import verdict: %v", err)
	}
	return status
}
