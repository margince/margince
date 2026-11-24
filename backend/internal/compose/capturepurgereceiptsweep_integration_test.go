// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/purgerun"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const sweepAddress = "gegner@example.test"

const destroyedReceipts = `SELECT count(*) FROM audit_log WHERE entity_type = 'capture_exclusion' AND entity_id = $1 AND (evidence->>'destroyed')::int = $2`

// A message the cascade skips is not certified as destroyed.
func TestTheReceiptCountsWhatTheCascadeDeleted(t *testing.T) {
	e := integration.Setup(t)
	gone := seedPurgeableMail(t, e, sweepAddress, "Eins", e.Rep1)
	skipped := seedPurgeableMail(t, e, sweepAddress, "Zwei", e.Rep1)
	restrict(t, e, skipped)
	rule := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindAddress, sweepAddress)
	var outcome PurgeOutcome
	err := purgerun.Hold(purgeCtx(e, e.Rep1), e.Pool, purgerun.Rule(rule), func(ctx context.Context, run ids.UUID) error {
		var err error
		outcome, err = purgerFor(t, e).destroy(ctx, run, rule, e.Rep1,
			capture.Exclusion{ID: rule, Kind: capture.ExclusionKindAddress, Value: sweepAddress}, false,
			capture.PurgeSubject{SoleImports: []ids.UUID{gone, skipped}}, nil, PurgeOutcome{Destroyed: 2})
		return err
	})
	if err != nil || outcome.Destroyed != 1 || e.WsCount(t, destroyedReceipts, rule, 1) != 1 {
		t.Fatalf("outcome = %+v, err = %v, want 1 destroyed and one receipt saying 1", outcome, err)
	}
}

// A purge run has no bulk_operation row, so the undo refuses it.
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

func TestThePurgeEndpointAnswersWhatTheReceiptStores(t *testing.T) {
	e := integration.Setup(t)
	restrict(t, e, seedPurgeableMail(t, e, sweepAddress, "Klage", e.Rep1))
	seedPurgeableMail(t, e, sweepAddress, "Zweite", e.Rep1)
	rule := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindAddress, sweepAddress)
	rec := httptest.NewRecorder()
	captureExclusionHandlers{purger: purgerFor(t, e)}.PurgeCaptureExclusion(rec,
		httptest.NewRequest(http.MethodPost, "/", nil).WithContext(purgeCtx(e, e.Rep1)),
		crmcontracts.Id(rule), crmcontracts.PurgeCaptureExclusionParams{})
	var got crmcontracts.CapturePurgeOutcome
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("response %d %q: %v", rec.Code, rec.Body.String(), err)
	}
	if matching := e.WsCount(t, destroyedReceipts+`
		   AND (evidence->>'released')::int = $3 AND (evidence->>'skipped')::int = $4
		   AND (evidence->>'anonymised')::int = $5 AND (evidence->>'kept_held')::int = $6
		   AND (evidence->>'kept_under_statute')::int = $7 AND (evidence->>'kept_under_request')::int = $8
		   AND (evidence->>'kept_under_undetermined_floor')::int = $9`,
		rule, got.Destroyed, got.Released, got.Skipped, got.Anonymised,
		got.Kept.Held, got.Kept.UnderStatute, got.Kept.UnderRequest, got.Kept.UnderUndeterminedFloor); matching != 1 {
		t.Fatalf("receipts agreeing with the response %+v = %d, want 1", got, matching)
	}
}

func TestTheVerdictWorkerCertifiesAStoppedPurge(t *testing.T) {
	e := integration.Setup(t)
	mail := seedPurgeableMail(t, e, sweepAddress, "Eins", e.Rep1)
	rule, purger := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindAddress, sweepAddress), purgerFor(t, e)
	err := purgerun.Hold(purgeCtx(e, e.Rep1), e.Pool, purgerun.Rule(rule), func(ctx context.Context, _ ids.UUID) error {
		_, err := purger.retention.PurgeActivities(ctx, []ids.UUID{mail}, privacy.PurgeOwnerRule)
		return err
	})
	worker := &counterpartyVerdictWorker{pool: e.Pool, purger: purger}
	if err != nil || worker.sweepPurges(purgeCtx(e, e.Rep1)) != nil || e.WsCount(t, destroyedReceipts, rule, 1) != 1 {
		t.Fatalf("stopped purge err = %v, want the worker pass to write its receipt", err)
	}
}
