// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/viability"
)

// RecordReceipts stores this pass's viability answer for every deal it walked,
// eligible ones included, replacing each deal's previous receipt.
//
// A deal with no assessment gets an unknown receipt rather than none. The
// qualified forecast must tell "checked and could not judge" from "checked and
// fine", and absence would read as neither.
//
// The run's own audit row covers these, as it covers the findings. A receipt
// per deal in the audit log would bury the night's one verdict.
func (s *Store) RecordReceipts(
	ctx context.Context, tx pgx.Tx, runID ids.UUID, asOf time.Time, lifetime time.Duration, subjects []Subject,
) error {
	if err := auth.Require(ctx, "forecast", principal.ActionCreate); err != nil {
		return err
	}
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	batch := &pgx.Batch{}
	for _, subject := range subjects {
		dealID, err := ids.Parse(subject.DealID)
		if err != nil {
			return fmt.Errorf("assurance: a subject names a deal id it cannot parse: %w", err)
		}
		answer := receiptOf(subject.Viability)
		batch.Queue(`
			INSERT INTO assurance_deal_receipt
			    (deal_id, run_id, outcome, reasons, silent_days, policy_version,
			     input_fingerprint, assessed_at, valid_until, captured_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (deal_id) DO UPDATE
			SET run_id = EXCLUDED.run_id, outcome = EXCLUDED.outcome, reasons = EXCLUDED.reasons,
			    silent_days = EXCLUDED.silent_days, policy_version = EXCLUDED.policy_version,
			    input_fingerprint = EXCLUDED.input_fingerprint, assessed_at = EXCLUDED.assessed_at,
			    valid_until = EXCLUDED.valid_until, captured_by = EXCLUDED.captured_by,
			    updated_at = now()`,
			dealID, runID, string(answer.Outcome), reasonCodes(answer.Reasons), answer.SilentDays,
			viability.PolicyVersion, answer.Fingerprint, asOf, validUntil(asOf, lifetime, answer.NextChange), capturedBy)
	}
	if err := tx.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("assurance: recording the viability receipts: %w", err)
	}
	return nil
}

func receiptOf(assessed *SubjectViability) SubjectViability {
	if assessed == nil {
		return SubjectViability{Outcome: viability.OutcomeUnknown}
	}
	return *assessed
}

// validUntil is the sooner of the next scheduled check and the moment the
// answer changes by the clock alone. Never before the assessment itself.
func validUntil(asOf time.Time, lifetime time.Duration, nextChange *time.Time) time.Time {
	until := asOf.Add(lifetime)
	if nextChange != nil && nextChange.Before(until) {
		until = *nextChange
	}
	if until.Before(asOf) {
		return asOf
	}
	return until
}

func reasonCodes(reasons []viability.Reason) []string {
	out := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		out = append(out, string(reason))
	}
	return out
}
