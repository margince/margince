// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// legacySettled is a settled verdict written by the given principal and then
// stripped of its evidence binding. Every verdict recorded before the binding
// is in that state; no current writer leaves a verdict unbound.
func legacySettled(writer context.Context, t *testing.T, e *loadEnv, subject string) ids.UUID {
	t.Helper()
	request := seedEmailRequest(t, e, subject, "commitment", OwedVerdictAsksUs)
	replyTo(t, e, request, "Here it is.", requestInstant.Add(time.Hour))
	mintReminder(t, e, requestInstant.Add(2*time.Hour))
	candidate := repliedIDs(t, e, bindingReadAt)[request]
	if err := storeKnowing(e).SettleRequest(writer, RequestSettlementInput{Request: candidate, Verdict: RequestSettled, Confidence: 0.9, DecidedBy: "legacy-model"}); err != nil {
		t.Fatal(err)
	}
	e.exec(t, `UPDATE activity_request_settlement SET judged_evidence = NULL WHERE request_activity_id = $1`, request)
	return request
}

func repairPage(t *testing.T, e *loadEnv, asOf time.Time) map[ids.UUID]SettlementRepairItem {
	t.Helper()
	out := map[ids.UUID]SettlementRepairItem{}
	err := storeKnowing(e).tx(asEditor(e), func(tx pgx.Tx) error {
		items, err := SettlementRepairPageTx(asEditor(e), tx, ids.Nil, 100, asOf)
		for _, item := range items {
			out[item.RequestID] = item
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func revalidate(t *testing.T, e *loadEnv, item SettlementRepairItem, asOf time.Time) bool {
	t.Helper()
	var wrote bool
	err := storeKnowing(e).tx(asEditor(e), func(tx pgx.Tx) error {
		var err error
		wrote, err = RequestSettlementRevalidationTx(asEditor(e), tx, item, asOf)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return wrote
}

func asLegacyWriter(e *loadEnv) context.Context {
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.ws), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:legacy_settlement",
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	})
}

// A provenance-backed legacy verdict is judged again once, and a still-owed
// answer then reopens the reminder the pass completed. A second run finds
// nothing left to do.
func TestARevalidatedLegacyVerdictIsJudgedAgainOnce(t *testing.T) {
	e := setupLoad(t)
	request := legacySettled(asSettler(e), t, e, "Legacy settled")
	previewAt := requestInstant.Add(72 * time.Hour)
	if _, ok := repliedIDs(t, e, previewAt)[request]; ok {
		t.Fatal("a legacy verdict was judged again before anyone reviewed the repair")
	}
	item, ok := repairPage(t, e, previewAt)[request]
	if !ok || item.Category != SettlementRevalidate {
		t.Fatalf("preview = %+v, want revalidate", item)
	}
	if !revalidate(t, e, item, previewAt) {
		t.Fatal("the reviewed revalidation was not applied")
	}
	if audits, unannounced := settlementAudits(t, e, request, settlementRevalidationCol); audits != 1 || unannounced != 0 {
		t.Fatalf("the revalidation wrote %d audit row(s), %d with no outbox row; want one, announced", audits, unannounced)
	}
	if again := repairPage(t, e, previewAt)[request]; again.Category != SettlementPending || revalidate(t, e, again, previewAt) {
		t.Fatalf("a second run did not show the request as waiting to be judged: %+v", again)
	}
	settleThroughPass(t, e, request, previewAt, RequestStillOwed)
	if _, _, done, _ := taskFor(t, e, request); done {
		t.Fatal("the still-owed verdict left the pass's completion standing")
	}
	if _, ok := repairPage(t, e, previewAt)[request]; ok {
		t.Fatal("the judged verdict is still listed for repair")
	}
}

func TestLegacyVerdictsWithoutProofOrWithHumanWorkAreKept(t *testing.T) {
	e := setupLoad(t)
	unknown := legacySettled(asLegacyWriter(e), t, e, "Unknown writer")
	human := legacySettled(asSettler(e), t, e, "Human kept")
	task, _, _, _ := taskFor(t, e, human)
	subject := "Sent, confirmed by phone"
	if _, err := storeKnowing(e).UpdateActivity(asEditor(e), ids.From[ids.ActivityKind](task), UpdateActivityInput{Subject: &subject}); err != nil {
		t.Fatal(err)
	}
	aged := legacySettled(asSettler(e), t, e, "Past the horizon")
	page := repairPage(t, e, requestInstant.AddDate(0, 0, 200))
	for request, want := range map[ids.UUID]SettlementRepairCategory{unknown: SettlementProvenanceUnknown, human: SettlementHumanWork, aged: SettlementPastHorizon} {
		item := page[request]
		if item.Category != want {
			t.Errorf("category = %q, want %q", item.Category, want)
		}
		if revalidate(t, e, item, requestInstant.AddDate(0, 0, 200)) {
			t.Errorf("a %s verdict was revalidated", want)
		}
	}
}

func TestARevalidationIsRefusedWhenTheVerdictChangedSinceReview(t *testing.T) {
	e := setupLoad(t)
	request := legacySettled(asSettler(e), t, e, "Changed since review")
	previewAt := requestInstant.Add(72 * time.Hour)
	item := repairPage(t, e, previewAt)[request]
	task, _, _, _ := taskFor(t, e, request)
	open := false
	if _, err := storeKnowing(e).UpdateActivity(asEditor(e), ids.From[ids.ActivityKind](task), UpdateActivityInput{IsDone: &open}); err != nil {
		t.Fatal(err)
	}
	if revalidate(t, e, item, previewAt) {
		t.Fatal("a revalidation was applied over a human change made after the preview")
	}
}

// An unsure answer from the repeated judgement is no evidence that work is
// owed. The reminder the pass closed stays closed.
func TestAnUnsureRevalidationReopensNothing(t *testing.T) {
	e := setupLoad(t)
	request := legacySettled(asSettler(e), t, e, "Unsure again")
	previewAt := requestInstant.Add(72 * time.Hour)
	if !revalidate(t, e, repairPage(t, e, previewAt)[request], previewAt) {
		t.Fatal("the revalidation was not applied")
	}
	settleThroughPass(t, e, request, previewAt, RequestUnsure)
	if _, _, done, _ := taskFor(t, e, request); !done {
		t.Fatal("an unsure verdict reopened the reminder the pass had closed")
	}
}

// A legacy verdict whose answers are all gone cannot be judged again, so it is
// not marked for it. A held request names nothing about itself. Neither stops
// the rest of the page.
func TestUnjudgeableAndHeldLegacyVerdictsDoNotBreakThePage(t *testing.T) {
	e := setupLoad(t)
	valid := legacySettled(asSettler(e), t, e, "Still judgeable")
	orphaned := legacySettled(asSettler(e), t, e, "Answer archived")
	held := legacySettled(asSettler(e), t, e, "Under a hold")
	e.exec(t, `UPDATE activity SET archived_at = now() WHERE direction = 'outbound'
		AND thread_key = (SELECT thread_key FROM activity WHERE id = $1)`, orphaned)
	// No writer holds an activity under retention yet, so the fixture records
	// the controller's pin the hold rests on, as the purge tests do.
	e.exec(t, `INSERT INTO activity_retention_evidence (activity_id, basis, qualified_at, decided_by_name, reason)
		VALUES ($1, 'controller_pin', now(), 'Controller', 'Statutory hold')`, held)
	e.exec(t, `UPDATE activity SET retention_class = 'commercial_correspondence', retention_class_at = now(), archived_at = now(),
		restricted_at = now(), restricted_reason = 'statutory retention', restricted_until = now() + interval '6 years'
		WHERE id = $1`, held)
	previewAt := requestInstant.Add(72 * time.Hour)
	page := repairPage(t, e, previewAt)
	for request, want := range map[ids.UUID]SettlementRepairCategory{valid: SettlementRevalidate, orphaned: SettlementNoAnswers, held: SettlementUnavailable} {
		if got := page[request]; got.Category != want || (want == SettlementUnavailable && got.Snapshot != "") {
			t.Errorf("category = %+v, want %q", got, want)
		}
	}
	if revalidate(t, e, page[orphaned], previewAt) {
		t.Fatal("a verdict with no readable answer was marked to be judged again")
	}
}

// A verdict judged again between the listing and the activation is reported
// as changed, not as an error that aborts the review.
func TestAVerdictJudgedSinceListingIsReportedChanged(t *testing.T) {
	e := setupLoad(t)
	request := legacySettled(asSettler(e), t, e, "Judged meanwhile")
	previewAt := requestInstant.Add(72 * time.Hour)
	listed := repairPage(t, e, previewAt)[request]
	if !revalidate(t, e, listed, previewAt) {
		t.Fatal("the revalidation was not applied")
	}
	settleThroughPass(t, e, request, previewAt, RequestSettled)
	var now SettlementRepairItem
	if err := storeKnowing(e).tx(asEditor(e), func(tx pgx.Tx) error {
		var err error
		now, err = SettlementRepairCurrentTx(asEditor(e), tx, request, previewAt)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if now.Category != SettlementChanged {
		t.Fatalf("category = %q, want changed", now.Category)
	}
}
