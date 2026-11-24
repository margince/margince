// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The purge receipt counts what the cascade deleted, and a purge that stopped
// before its receipt gets one from the sweep.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const sweepAddress = "gegner@example.test"

func receiptRows(t *testing.T, e *integration.Env, rule ids.UUID) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM audit_log
		WHERE entity_type = 'capture_exclusion' AND entity_id = $1 AND evidence ? 'destroyed'`, rule)
}

func exclusionOf(rule ids.UUID) capture.Exclusion {
	return capture.Exclusion{ID: rule, Kind: capture.ExclusionKindAddress, Value: sweepAddress}
}

// A message the cascade skips is not certified as destroyed, whatever the
// selection said.
func TestTheReceiptCountsWhatTheCascadeDeleted(t *testing.T) {
	e := integration.Setup(t)
	gone := seedPurgeableMail(t, e, sweepAddress, "Eins", e.Rep1)
	skipped := seedPurgeableMail(t, e, sweepAddress, "Zwei", e.Rep1)
	restrict(t, e, skipped)
	rule := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindAddress, sweepAddress)

	subject := capture.PurgeSubject{SoleImports: []ids.UUID{gone, skipped}}
	outcome, err := purgerFor(t, e).destroy(purgeCtx(e, e.Rep1), rule, e.Rep1, exclusionOf(rule), false,
		subject, nil, PurgeOutcome{Destroyed: 2})
	if err != nil {
		t.Fatalf("destroying: %v", err)
	}
	if outcome.Destroyed != 1 {
		t.Fatalf("destroyed = %d, want the 1 message the cascade deleted", outcome.Destroyed)
	}
	if rows := e.WsCount(t, `SELECT count(*) FROM audit_log
		WHERE entity_type = 'capture_exclusion' AND entity_id = $1 AND (evidence->>'destroyed')::int = 1`, rule); rows != 1 {
		t.Fatalf("receipts stating 1 destroyed = %d, want 1", rows)
	}
}

// Per-message rows without a receipt get exactly one, written from the rows,
// and a second pass or another rule's receipt changes nothing.
func TestTheSweepWritesTheMissingReceiptOnce(t *testing.T) {
	e := integration.Setup(t)
	seedPurgeableMail(t, e, sweepAddress, "Eins", e.Rep1)
	first := seedPurgeableMail(t, e, sweepAddress, "Zwei", e.Rep1)
	rule := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindAddress, sweepAddress)
	subject := capture.PurgeSubject{SoleImports: []ids.UUID{first}}
	otherMail := seedPurgeableMail(t, e, "andere@example.test", "Drei", e.Rep1)
	other := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindAddress, "andere@example.test")
	runPurge(t, e, e.Rep1, other, false)

	purger := purgerFor(t, e)
	if _, err := purger.cascade(purgeCtx(e, e.Rep1), rule, e.Rep1, exclusionOf(rule), false, subject, nil); err != nil {
		t.Fatalf("cascading: %v", err)
	}
	if got := receiptRows(t, e, rule); got != 0 {
		t.Fatalf("receipts before the sweep = %d, want 0", got)
	}

	written, err := purger.SweepMissingReceipts(purgeCtx(e, e.Rep1), time.Hour)
	if err != nil || written != 0 {
		t.Fatalf("sweep inside the grace period = (%d, %v), want it to leave a young purge alone", written, err)
	}
	for pass, want := range []int{1, 0} {
		written, err := purger.SweepMissingReceipts(purgeCtx(e, e.Rep1), 0)
		if err != nil || written != want {
			t.Fatalf("sweep pass %d = (%d, %v), want %d receipt written", pass, written, err, want)
		}
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log
		WHERE entity_type = 'capture_exclusion' AND entity_id = $1 AND (evidence->>'destroyed')::int = 1
		  AND (evidence->>'recovered')::boolean AND actor_type = 'system'`, rule); got != 1 {
		t.Fatalf("recovered receipts stating 1 destroyed = %d, want exactly 1", got)
	}
	if got := receiptRows(t, e, other); got != 1 {
		t.Fatalf("the other rule has %d receipts, want its own one untouched", got)
	}
	if body := activityBody(t, e, otherMail); body != "" {
		t.Fatalf("the other rule's mail body = %q, want it destroyed by its own purge", body)
	}
}
