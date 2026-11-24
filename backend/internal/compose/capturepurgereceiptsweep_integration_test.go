// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The purge receipt counts what its own run deleted, and a run that stopped
// before its receipt gets one from the sweep.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/purgerun"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const sweepAddress = "gegner@example.test"

func exclusionOf(rule ids.UUID) capture.Exclusion {
	return capture.Exclusion{ID: rule, Kind: capture.ExclusionKindAddress, Value: sweepAddress}
}

// receiptDestroyed is the destroyed count of each receipt written for a subject.
func receiptDestroyed(t *testing.T, e *integration.Env, entity string, subject ids.UUID) []int {
	t.Helper()
	var out []int
	ctx := e.Admin()
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT (evidence->>'destroyed')::int FROM audit_log
			WHERE entity_type = $1 AND entity_id = $2 AND evidence ? 'destroyed' ORDER BY id`, entity, subject)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowTo[int])
		return err
	}); err != nil {
		t.Fatalf("reading receipts: %v", err)
	}
	return out
}

// stoppedPurge carries a selection out as one run and stops before the receipt,
// as a process that died between the two would. It returns the run id.
func stoppedPurge(t *testing.T, e *integration.Env, rule ids.UUID, mails ...ids.UUID) ids.UUID {
	t.Helper()
	var run ids.UUID
	err := purgerun.Hold(purgeCtx(e, e.Rep1), e.Pool, purgerun.Rule(rule), func(ctx context.Context, id ids.UUID) error {
		run = id
		return purgerFor(t, e).cascade(ctx, e.Rep1, exclusionOf(rule), false,
			capture.PurgeSubject{SoleImports: mails}, nil)
	})
	if err != nil {
		t.Fatalf("cascading: %v", err)
	}
	return run
}

func sweepReceipts(t *testing.T, e *integration.Env) purgerun.SweepResult {
	t.Helper()
	result, err := purgerFor(t, e).SweepMissingReceipts(purgeCtx(e, e.Rep1))
	if err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	return result
}

// A message the cascade skips is not certified as destroyed, whatever the
// selection said.
func TestTheReceiptCountsWhatTheCascadeDeleted(t *testing.T) {
	e := integration.Setup(t)
	gone := seedPurgeableMail(t, e, sweepAddress, "Eins", e.Rep1)
	skipped := seedPurgeableMail(t, e, sweepAddress, "Zwei", e.Rep1)
	restrict(t, e, skipped)
	rule := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindAddress, sweepAddress)

	var outcome PurgeOutcome
	err := purgerun.Hold(purgeCtx(e, e.Rep1), e.Pool, purgerun.Rule(rule), func(ctx context.Context, run ids.UUID) error {
		var err error
		outcome, err = purgerFor(t, e).destroy(ctx, run, rule, e.Rep1, exclusionOf(rule), false,
			capture.PurgeSubject{SoleImports: []ids.UUID{gone, skipped}}, nil, PurgeOutcome{Destroyed: 2})
		return err
	})
	if err != nil {
		t.Fatalf("destroying: %v", err)
	}
	if got := receiptDestroyed(t, e, purgerun.EntityRule, rule); outcome.Destroyed != 1 || !slices.Equal(got, []int{1}) {
		t.Fatalf("outcome.Destroyed = %d, receipts = %v, want the 1 message the cascade deleted", outcome.Destroyed, got)
	}
}

// Rows without a receipt get one receipt, written from the rows, and another
// rule's certified purge is left as it was.
func TestTheSweepWritesTheMissingReceiptOnce(t *testing.T) {
	e := integration.Setup(t)
	first := seedPurgeableMail(t, e, sweepAddress, "Eins", e.Rep1)
	rule := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindAddress, sweepAddress)
	seedPurgeableMail(t, e, "andere@example.test", "Drei", e.Rep1)
	other := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindAddress, "andere@example.test")
	runPurge(t, e, e.Rep1, other, false)

	run := stoppedPurge(t, e, rule, first)
	if got := receiptDestroyed(t, e, purgerun.EntityRule, rule); len(got) != 0 {
		t.Fatalf("receipts before the sweep = %v, want none", got)
	}
	if result := sweepReceipts(t, e); result.Written != 1 || result.Unattributed != 0 {
		t.Fatalf("first sweep = %+v, want one receipt written", result)
	}
	if result := sweepReceipts(t, e); result.Written != 0 {
		t.Fatalf("second sweep = %+v, want nothing more", result)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log
		WHERE entity_type = 'capture_exclusion' AND entity_id = $1 AND batch_id = $2
		  AND (evidence->>'destroyed')::int = 1 AND (evidence->>'recovered')::boolean
		  AND actor_type = 'system'`, rule, run); got != 1 {
		t.Fatalf("recovered receipts of the run = %d, want exactly 1", got)
	}
	if got := receiptDestroyed(t, e, purgerun.EntityRule, other); !slices.Equal(got, []int{1}) {
		t.Fatalf("the other rule's receipts = %v, want its own one untouched", got)
	}
}

// A redaction can strip an image the release row carried, so the release is
// found by its run and not by what the row says.
func TestARecoveredReceiptStillCountsAReleaseTheMessageLaterErased(t *testing.T) {
	e := integration.Setup(t)
	shared := seedPurgeableMail(t, e, sweepAddress, "Geteilt", e.Rep1)
	addImporter(t, e, shared, e.Rep2)
	rule := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindAddress, sweepAddress)
	err := purgerun.Hold(purgeCtx(e, e.Rep1), e.Pool, purgerun.Rule(rule), func(ctx context.Context, _ ids.UUID) error {
		return purgerFor(t, e).cascade(ctx, e.Rep1, exclusionOf(rule), false,
			capture.PurgeSubject{SharedImports: []ids.UUID{shared}}, nil)
	})
	if err != nil {
		t.Fatalf("releasing: %v", err)
	}
	if _, err := purgerFor(t, e).retention.PurgeActivities(purgeCtx(e, e.Rep1), []ids.UUID{shared}, privacy.PurgeOwnerRule); err != nil {
		t.Fatalf("erasing the released message: %v", err)
	}
	if left := e.WsCount(t, `SELECT count(*) FROM audit_log
		WHERE entity_type = 'activity' AND entity_id = $1 AND action = 'archive' AND after ? 'released_by'`, shared); left != 0 {
		t.Fatalf("the release row still shows its image (%d); the erase did not redact it, so this test proves nothing", left)
	}

	if result := sweepReceipts(t, e); result.Written != 1 {
		t.Fatalf("sweep = %+v, want one receipt", result)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log
		WHERE entity_type = 'capture_exclusion' AND entity_id = $1 AND (evidence->>'released')::int = 1`, rule); got != 1 {
		t.Fatalf("receipts counting the release = %d, want 1", got)
	}
}

// Two purges of one rule run one after the other, and each certifies only what
// it destroyed.
func TestTwoConcurrentPurgesOfOneRuleGetTheirOwnReceipts(t *testing.T) {
	e := integration.Setup(t)
	seedPurgeableMail(t, e, sweepAddress, "Eins", e.Rep1)
	seedPurgeableMail(t, e, sweepAddress, "Zwei", e.Rep1)
	rule := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindAddress, sweepAddress)

	outcomes := make([]PurgeOutcome, 2)
	var wg sync.WaitGroup
	for i := range outcomes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var err error
			if outcomes[i], err = purgerFor(t, e).Purge(purgeCtx(e, e.Rep1), rule, false); err != nil {
				t.Errorf("purge %d: %v", i, err)
			}
		}()
	}
	wg.Wait()

	got := receiptDestroyed(t, e, purgerun.EntityRule, rule)
	want := []int{outcomes[0].Destroyed, outcomes[1].Destroyed}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("receipts = %v, outcomes = %v, want one receipt per purge stating that purge's own count", got, want)
	}
}

// A row that commits after another run's receipt belongs to its own run's
// receipt, and to no other.
func TestARowCommittedAfterAnotherRunsReceiptIsCountedByItsOwnRun(t *testing.T) {
	e := integration.Setup(t)
	a := seedPurgeableMail(t, e, sweepAddress, "Eins", e.Rep1)
	b := seedPurgeableMail(t, e, sweepAddress, "Zwei", e.Rep1)
	rule := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindAddress, sweepAddress)
	purger := purgerFor(t, e)
	runA, runB := purgerun.NewRunID(rule), purgerun.NewRunID(rule)
	for run, mail := range map[ids.UUID]ids.UUID{runA: a, runB: b} {
		if err := purger.cascade(storekit.WithBatch(purgeCtx(e, e.Rep1), run), e.Rep1, exclusionOf(rule), false,
			capture.PurgeSubject{SoleImports: []ids.UUID{mail}}, nil); err != nil {
			t.Fatalf("cascading: %v", err)
		}
	}
	for _, run := range []ids.UUID{runB, runA} {
		tally, err := purgerun.WriteReceipt(storekit.WithBatch(purgeCtx(e, e.Rep1), run), e.Pool, purgerun.Rule(rule), run,
			func(t purgerun.Tally) map[string]any { return t.Evidence() })
		if err != nil || tally.Destroyed != 1 {
			t.Fatalf("receipt of run %s = (%+v, %v), want its own 1 message", run, tally, err)
		}
	}
}

// Removing a rule after a purge that stopped leaves the receipt behind.
func TestRemovingARuleAfterAStoppedPurgeLeavesItsReceipt(t *testing.T) {
	e := integration.Setup(t)
	mail := seedPurgeableMail(t, e, sweepAddress, "Eins", e.Rep1)
	rule := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindAddress, sweepAddress)
	stoppedPurge(t, e, rule, mail)

	if err := newExclusionStore(e.Pool).Remove(purgeCtx(e, e.Rep1), rule); err != nil {
		t.Fatalf("removing the rule: %v", err)
	}
	if got := receiptDestroyed(t, e, purgerun.EntityRule, rule); !slices.Equal(got, []int{1}) {
		t.Fatalf("receipts after the removal = %v, want the stopped run's", got)
	}
	if result := sweepReceipts(t, e); result.Written != 0 || result.Unattributed != 0 {
		t.Fatalf("sweep after the removal = %+v, want nothing left to do", result)
	}
}

// A rule purged again after new mail arrived gets a second receipt of its own.
func TestASecondPurgeOfOneRuleGetsItsOwnReceipt(t *testing.T) {
	e := integration.Setup(t)
	seedPurgeableMail(t, e, sweepAddress, "Eins", e.Rep1)
	rule := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindAddress, sweepAddress)
	runPurge(t, e, e.Rep1, rule, false)
	seedPurgeableMail(t, e, sweepAddress, "Zwei", e.Rep1)
	second := runPurge(t, e, e.Rep1, rule, false)

	if got := receiptDestroyed(t, e, purgerun.EntityRule, rule); !slices.Equal(got, []int{1, second.Destroyed}) || second.Destroyed < 1 {
		t.Fatalf("receipts = %v, second purge = %d, want one receipt per purge with its own count", got, second.Destroyed)
	}
}

// The sweep leaves a run alone while it holds its lock, and certifies it once
// the run is over without a receipt.
func TestTheSweepLeavesARunInFlightAlone(t *testing.T) {
	e := integration.Setup(t)
	mail := seedPurgeableMail(t, e, sweepAddress, "Eins", e.Rep1)
	rule := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindAddress, sweepAddress)
	err := purgerun.Hold(purgeCtx(e, e.Rep1), e.Pool, purgerun.Rule(rule), func(ctx context.Context, _ ids.UUID) error {
		if err := purgerFor(t, e).cascade(ctx, e.Rep1, exclusionOf(rule), false,
			capture.PurgeSubject{SoleImports: []ids.UUID{mail}}, nil); err != nil {
			return err
		}
		if result := sweepReceipts(t, e); result.Written != 0 {
			t.Errorf("sweep during the run = %+v, want it to leave the run alone", result)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result := sweepReceipts(t, e); result.Written != 1 {
		t.Fatalf("sweep after the run = %+v, want its receipt", result)
	}
}

// A seat's personal-mail pass that stopped gets its receipt the same way.
func TestTheSweepCertifiesAStoppedPersonalPass(t *testing.T) {
	e := integration.Setup(t)
	mail := seedPurgeableMail(t, e, sweepAddress, "Eins", e.Rep1)
	err := purgerun.Hold(purgeCtx(e, e.Rep1), e.Pool, purgerun.Seat(e.Rep1), func(ctx context.Context, _ ids.UUID) error {
		_, err := purgerFor(t, e).carryOut(ctx, capture.PurgeSubject{SoleImports: []ids.UUID{mail}}, nil, nil,
			e.Rep1, privacy.PurgePersonalVerdict)
		return err
	})
	if err != nil {
		t.Fatalf("personal pass: %v", err)
	}
	if result := sweepReceipts(t, e); result.Written != 1 {
		t.Fatalf("sweep = %+v, want the seat's receipt", result)
	}
	if got := receiptDestroyed(t, e, purgerun.EntityPersonalSweep, e.Rep1); !slices.Equal(got, []int{1}) {
		t.Fatalf("personal receipts = %v, want one for the 1 destroyed message", got)
	}
}

// A purge run shares the bulk change's batch column and nothing else: it has
// no bulk_operation row, so the undo refuses it.
func TestAPurgeRunCannotBeUndoneAsABulkChange(t *testing.T) {
	e := integration.Setup(t)
	seedPurgeableMail(t, e, sweepAddress, "Eins", e.Rep1)
	rule := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindAddress, sweepAddress)
	runPurge(t, e, e.Rep1, rule, false)
	run := ids.MustParse(e.WsScalar(t, `SELECT batch_id::text FROM audit_log
		WHERE entity_type = 'capture_exclusion' AND entity_id = $1 AND evidence ? 'destroyed'`, rule))

	if _, err := bulkEngineFor(e).Undo(e.Admin(), run, ""); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("undoing a purge run → %v, want it refused as no such bulk change", err)
	}
}

// The endpoint answers the numbers the trail stores.
func TestThePurgeEndpointAnswersWhatTheReceiptStores(t *testing.T) {
	e := integration.Setup(t)
	held := seedPurgeableMail(t, e, sweepAddress, "Klage", e.Rep1)
	restrict(t, e, held)
	seedPurgeableMail(t, e, sweepAddress, "Zweite", e.Rep1)
	rule := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindAddress, sweepAddress)

	h := captureExclusionHandlers{purger: purgerFor(t, e)}
	rec := httptest.NewRecorder()
	h.PurgeCaptureExclusion(rec, httptest.NewRequest(http.MethodPost, "/", nil).WithContext(purgeCtx(e, e.Rep1)),
		crmcontracts.Id(rule), crmcontracts.PurgeCaptureExclusionParams{})
	var got crmcontracts.CapturePurgeOutcome
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("response %d %q: %v", rec.Code, rec.Body.String(), err)
	}

	if matching := e.WsCount(t, `
		SELECT count(*) FROM audit_log
		 WHERE entity_type = 'capture_exclusion' AND entity_id = $1
		   AND (evidence->>'destroyed')::int = $2 AND (evidence->>'released')::int = $3
		   AND (evidence->>'skipped')::int = $4 AND (evidence->>'anonymised')::int = $5
		   AND (evidence->>'kept_held')::int = $6 AND (evidence->>'kept_under_statute')::int = $7
		   AND (evidence->>'kept_under_request')::int = $8
		   AND (evidence->>'kept_under_undetermined_floor')::int = $9`,
		rule, got.Destroyed, got.Released, got.Skipped, got.Anonymised,
		got.Kept.Held, got.Kept.UnderStatute, got.Kept.UnderRequest, got.Kept.UnderUndeterminedFloor); matching != 1 {
		t.Fatalf("receipts agreeing with the response %+v = %d, want 1", got, matching)
	}
}

// The verdict worker's pass runs the receipt sweep.
func TestTheVerdictWorkerCertifiesAStoppedPurge(t *testing.T) {
	e := integration.Setup(t)
	mail := seedPurgeableMail(t, e, sweepAddress, "Eins", e.Rep1)
	rule := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindAddress, sweepAddress)
	stoppedPurge(t, e, rule, mail)

	worker := &counterpartyVerdictWorker{purger: purgerFor(t, e)}
	if err := worker.sweepPurges(purgeCtx(e, e.Rep1), purgeCtx(e, e.Rep1), e.WS); err != nil {
		t.Fatalf("worker pass: %v", err)
	}
	if got := receiptDestroyed(t, e, purgerun.EntityRule, rule); !slices.Equal(got, []int{1}) {
		t.Fatalf("receipts after the worker pass = %v, want the stopped run's", got)
	}
}
