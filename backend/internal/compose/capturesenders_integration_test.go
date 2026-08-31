// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The Senders page, and the rule that makes it worth using: a person's answer
// about a sender is permanent, and the machine consults it rather than
// overwriting it.

import (
	"context"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestTheMachineNeverOverwritesAPersonsDecision(t *testing.T) {
	// The whole reason the page is worth a click. A correction the next message
	// undoes is a suggestion: the owner would find the same sender wrong again
	// next week with no way to tell a fresh mistake from one they already
	// fixed.
	e := integration.Setup(t)
	const sender = "anne@webmail.example"
	activityID := seedCapturedMail(t, e, sender, "Fotos")
	dispositionID := seedPendingDisposition(t, e, sender, "webmail.example", activityID)

	// The owner says this is business, before any verdict runs.
	setDecision(t, e, e.Rep1, sender, capture.OverrideBusiness)

	// A brain that would answer `spam` — and must never be asked.
	brain := &scriptedVerdictBrain{verdicts: map[string]string{dispositionID.String(): capture.KindSpam}}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}

	if brain.calls != 0 {
		t.Errorf("the model was asked %d times about a sender the owner already decided — "+
			"a paid call to be told something we discard", brain.calls)
	}
	if got := dispositionKind(t, e, dispositionID); got != capture.KindPerson {
		t.Fatalf("the ledger says %q, want person — the owner said business", got)
	}
}

func TestKeepingASenderOutSurvivesTheClassifier(t *testing.T) {
	e := integration.Setup(t)
	const sender = "angebote@spam.example"
	activityID := seedCapturedMail(t, e, sender, "Angebot")
	dispositionID := seedPendingDisposition(t, e, sender, "spam.example", activityID)

	setDecision(t, e, e.Rep1, sender, capture.OverrideKeepOut)

	// A brain that would call them a person. The owner disagrees.
	brain := &scriptedVerdictBrain{verdicts: map[string]string{dispositionID.String(): capture.KindPerson}}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}

	if got := dispositionKind(t, e, dispositionID); got != capture.KindSpam {
		t.Fatalf("the ledger says %q, want spam — the owner said keep out", got)
	}
	if n := countIn(t, e, `
		SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
		 WHERE pe.email = $1`, sender); n != 0 {
		t.Fatalf("%d contacts for a sender the owner kept out, want 0", n)
	}
}

func TestTheSendersPageShowsWhatWasDecidedAndByWhom(t *testing.T) {
	e := integration.Setup(t)
	const judged = "kunde@example.test"
	const corrected = "freund@webmail.example"
	seedPendingDisposition(t, e, judged, "example.test", seedCapturedMail(t, e, judged, "Angebot"))
	seedPendingDisposition(t, e, corrected, "webmail.example", seedCapturedMail(t, e, corrected, "Hallo"))
	setDecision(t, e, e.Rep1, corrected, capture.OverrideBusiness)

	list, err := capture.SendersFor(purgeCtx(e, e.Rep1), InstallationDB(e.Pool))
	if err != nil {
		t.Fatalf("listing senders: %v", err)
	}
	byAddress := map[string]capture.SenderDecision{}
	for _, d := range list {
		byAddress[d.Address] = d
	}
	if got, ok := byAddress[judged]; !ok || got.Overruled() {
		t.Fatalf("the judged sender is %+v, want present and not overruled", got)
	}
	if got, ok := byAddress[corrected]; !ok || !got.Overruled() {
		t.Fatalf("the corrected sender is %+v, want present and overruled", got)
	}
}

func TestASeatSeesOnlyTheirOwnSenders(t *testing.T) {
	// Whose mail a person keeps out is itself private: a colleague's list is
	// not a thing this product will show, to anyone.
	e := integration.Setup(t)
	const mine = "meins@example.test"
	seedPendingDisposition(t, e, mine, "example.test", seedCapturedMail(t, e, mine, "Betreff"))
	setDecision(t, e, e.Rep1, mine, capture.OverrideKeepOut)

	list, err := capture.SendersFor(purgeCtx(e, e.Rep2), InstallationDB(e.Pool))
	if err != nil {
		t.Fatalf("listing a colleague's senders: %v", err)
	}
	for _, d := range list {
		if d.Address == mine {
			t.Fatalf("a colleague saw %q on their own senders page", mine)
		}
	}
}

func setDecision(t *testing.T, e *integration.Env, user ids.UUID, address, decision string) {
	t.Helper()
	store := capture.NewSenderOverrideStore(InstallationDB(e.Pool))
	if _, err := store.Set(purgeCtx(e, user), address, decision); err != nil {
		t.Fatalf("recording the decision: %v", err)
	}
}

func dispositionKind(t *testing.T, e *integration.Env, id ids.UUID) string {
	t.Helper()
	var kind string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT coalesce(kind, '') FROM capture_pending_counterparty WHERE id = $1`, id).Scan(&kind)
	}); err != nil {
		t.Fatalf("reading the disposition kind: %v", err)
	}
	return kind
}
