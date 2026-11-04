// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/viability"
)

// DealVerdict is Margince's last reading of one deal's evidence, kept on the
// deal's receipt so a deal whose evidence has not moved is not read again.
type DealVerdict struct {
	DealID ids.UUID
	// Fingerprint is the deal's viability fingerprint now. It is stored only
	// when the deal has no receipt yet, so the input check reads it as moved.
	Fingerprint   string
	InputChecksum string
	BasisDigest   string
	PromptVersion string
	AssessedAt    time.Time
	// NextDueAt is when the clock alone makes the deal worth reading again.
	NextDueAt *time.Time
	Actions   json.RawMessage
}

// RecordVerdict stores a deal's verdict on its receipt. A deal the input check
// has not walked gets an unknown receipt that names no run; the check's next
// pass replaces everything but the verdict.
func (s *Store) RecordVerdict(ctx context.Context, tx pgx.Tx, v DealVerdict) error {
	if err := auth.Require(ctx, "forecast", principal.ActionCreate); err != nil {
		return err
	}
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO assurance_deal_receipt
		    (deal_id, run_id, outcome, reasons, silent_days, policy_version, input_fingerprint,
		     assessed_at, valid_until, captured_by,
		     verdict_input_checksum, verdict_basis_digest, verdict_prompt_version,
		     verdict_assessed_at, verdict_next_due_at, verdict_actions)
		VALUES ($1, NULL, $2, '{}', 0, $3, $4, $5, $5, $6, $7, $8, $9, $5, $10, $11)
		ON CONFLICT (deal_id) DO UPDATE
		SET verdict_input_checksum = EXCLUDED.verdict_input_checksum,
		    verdict_basis_digest = EXCLUDED.verdict_basis_digest,
		    verdict_prompt_version = EXCLUDED.verdict_prompt_version,
		    verdict_assessed_at = EXCLUDED.verdict_assessed_at,
		    verdict_next_due_at = EXCLUDED.verdict_next_due_at,
		    verdict_actions = EXCLUDED.verdict_actions,
		    updated_at = now()`,
		v.DealID, string(viability.OutcomeUnknown), viability.PolicyVersion, v.Fingerprint, v.AssessedAt,
		capturedBy, v.InputChecksum, v.BasisDigest, v.PromptVersion, v.NextDueAt, v.Actions)
	if err != nil {
		return fmt.Errorf("assurance: recording the deal verdict: %w", err)
	}
	return nil
}
