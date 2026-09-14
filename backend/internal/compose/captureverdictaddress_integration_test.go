// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The two deterministic gates that stand between a model's answer and a contact
// record, written over the addresses a real ten-year Gmail import turned into
// contacts.
//
// Both exist because the verdict lane creates records and, until they were
// added, nothing between the model and EnsureCounterpartyTx could refuse one.
// The tier ladder's own record-worthiness gate governs the SINK, and a deferred
// sender never passes it — it goes to a model instead.

import (
	"context"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The addresses themselves. Each one reached a model in the real import, and a
// stray `contact` answer created the record — so the answer is read off the
// address here, where it does not vary.
//
// The mail STAYS VISIBLE and the domain keeps its company question: what the
// gate claims is only that nobody answers at this address.
func TestAnAddressNobodyAnswersNeverBecomesAContact(t *testing.T) {
	e := integration.Setup(t)
	cases := []struct {
		name    string
		email   string
		domain  string
		subject string
	}{
		{
			name:  "an expense tool mailing its own user",
			email: "receipts@expensify.com", domain: "expensify.com",
			subject: "Your receipt is ready",
		},
		{
			name:  "a billing product under its customer's letterhead",
			email: "noreply@fastbill.com", domain: "fastbill.com",
			subject: "Rechnung 2026-0417",
		},
		{
			name:  "a machine local part on an ordinary company domain",
			email: "no-reply@irgendeinshop.example", domain: "irgendeinshop.example",
			subject: "Ihre Bestellung",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			activityID := seedCapturedMail(t, e, tc.email, tc.subject)
			dispositionID := seedPendingDisposition(t, e, tc.email, tc.domain, activityID)

			// The brain answers `contact` at 0.95 for everything — the exact
			// stray answer that minted "Receipts" and "BERATUNG JUDITH
			// ANDRESEN". If the gate works, it is never consulted.
			brain := &scriptedVerdictBrain{}
			engine := NewCounterpartyVerdictEngine(e.Pool, brain, CaptureConfig{}, slog.Default())
			if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
				t.Fatalf("verdict pass: %v", err)
			}

			if brain.calls != 0 {
				t.Errorf("the model was asked %d times about an address readable without it", brain.calls)
			}
			// `real`, not `noise`: the claim is that nobody answers at this
			// ADDRESS, which creates no contact and authorizes nothing else.
			// Settling it as noise would take apply's suppression arm and refuse
			// the sender's whole domain a company — far more than the local part
			// can support. See the gate's own comment in judgeOne.
			if got := dispositionStatus(t, e, dispositionID); got != capture.PendingStatusReal {
				t.Errorf("disposition settled %q, want %q", got, capture.PendingStatusReal)
			}
			if n := countIn(t, e, `
				SELECT count(*) FROM contact_email WHERE email = $1`, tc.email); n != 0 {
				t.Errorf("%d contacts minted for %s — no human answers it", n, tc.email)
			}
		})
	}
}

// A named human at one of those companies is still a contact. The refusal is
// about the address, and a rule that swept the domain would lose the real
// counterparty it names.
func TestANamedHumanIsStillAContactWhereverTheyWork(t *testing.T) {
	e := integration.Setup(t)
	const address = "anna.mueller@irgendeinshop.example"
	activityID := seedCapturedMail(t, e, address, "Angebot für das Q3-Projekt")
	dispositionID := seedPendingDisposition(t, e, address, "irgendeinshop.example", activityID)

	brain := &scriptedVerdictBrain{}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, CaptureConfig{}, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}

	if brain.calls == 0 {
		t.Fatal("the model was never asked about an ordinary sender — the gate is too wide")
	}
	if got := dispositionStatus(t, e, dispositionID); got != capture.PendingStatusReal {
		t.Fatalf("disposition settled %q, want %q", got, capture.PendingStatusReal)
	}
	if n := countIn(t, e, `SELECT count(*) FROM contact_email WHERE email = $1`, address); n != 1 {
		t.Errorf("%d contacts for a named human, want 1", n)
	}
}

// One creating answer must not overturn an address's own settled history.
//
// The shape this pins is the one the import produced: sixteen questions about
// one address, fifteen answered `transactional`, and the sixteenth `contact` at
// 0.95 — over the create floor — minting the record. A verdict acts on the
// answer in front of it, so the rare wrong answer wins by being last unless
// something reads the others.
func TestAStrayCreatingAnswerAgainstASettledHistoryAsksAHuman(t *testing.T) {
	e := integration.Setup(t)
	const address = "updates@borderline.example"

	// Three settled non-contact answers, written the way the engine writes
	// them: a resolved ledger row carrying the kind it concluded.
	for _, subject := range []string{"release notes 1", "release notes 2", "release notes 3"} {
		activityID := seedCapturedMail(t, e, address, subject)
		id := seedPendingDisposition(t, e, address, "borderline.example", activityID)
		settleDisposition(t, e, id, capture.PendingStatusNoise, capture.KindTransactional)
	}

	// Now the stray: a fresh question, and a confident `contact`.
	activityID := seedCapturedMail(t, e, address, "quick question about your roadmap")
	dispositionID := seedPendingDisposition(t, e, address, "borderline.example", activityID)

	brain := &scriptedVerdictBrain{}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, CaptureConfig{}, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}

	// The model IS asked — this address is not one the vocabulary settles, and
	// the gate is about the answer rather than about the address.
	if brain.calls == 0 {
		t.Fatal("the model was never asked — this test proves nothing about a stray answer")
	}
	if got := dispositionStatus(t, e, dispositionID); got != capture.PendingStatusUnsure {
		t.Errorf("disposition settled %q, want %q — a human decides a contradiction",
			got, capture.PendingStatusUnsure)
	}
	if n := countIn(t, e, `SELECT count(*) FROM contact_email WHERE email = $1`, address); n != 0 {
		t.Errorf("%d contacts minted against a settled non-contact history, want 0", n)
	}
}

// A first `contact` answer, with no history behind it, still creates. The gate
// bounds a contradiction, not the ordinary case.
func TestAFirstCreatingAnswerStillCreates(t *testing.T) {
	e := integration.Setup(t)
	const address = "neu@interessent.example"
	activityID := seedCapturedMail(t, e, address, "Anfrage")
	dispositionID := seedPendingDisposition(t, e, address, "interessent.example", activityID)

	brain := &scriptedVerdictBrain{}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, CaptureConfig{}, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}

	if got := dispositionStatus(t, e, dispositionID); got != capture.PendingStatusReal {
		t.Fatalf("disposition settled %q, want %q", got, capture.PendingStatusReal)
	}
	if n := countIn(t, e, `SELECT count(*) FROM contact_email WHERE email = $1`, address); n != 1 {
		t.Errorf("%d contacts for a first confident answer, want 1", n)
	}
}
