// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/feedback"
)

// PreviewFeedbackCarryTx checks both subject records before exposing decisions that follow a merge.
func (s *FeedbackStore) PreviewFeedbackCarryTx(ctx context.Context, tx pgx.Tx, kind string, source, target ids.UUID) ([]feedback.CarriedDecision, error) {
	if err := admitSubject(kind); err != nil {
		return nil, err
	}
	if err := auth.Require(ctx, kind, principal.ActionRead); err != nil {
		return nil, err
	}
	for _, id := range []ids.UUID{source, target} {
		if err := auth.EnsureWritableLive(ctx, tx, kind, id); err != nil {
			return nil, err
		}
	}
	from, err := readCarriedDecisions(ctx, tx, kind, source, nil)
	if err != nil {
		return nil, err
	}
	if len(from) == 0 {
		return []feedback.CarriedDecision{}, nil
	}
	actor, ok := principal.Actor(ctx)
	// Hashed claim paths cannot prove which fields arbitrary notes or corrected values disclose.
	if !ok || len(auth.MaskedFields(actor, kind, true)) != 0 {
		return nil, apperrors.ErrPermissionDenied
	}
	to, err := readCarriedDecisions(ctx, tx, kind, target, &source)
	if err != nil {
		return nil, err
	}
	kept := map[string]feedback.StoredDecision{}
	for _, decision := range to {
		kept[VerdictLookupKey(decision.ClaimKind, decision.ClaimKey)] = decision
	}
	out := make([]feedback.CarriedDecision, 0, len(from))
	for _, decision := range from {
		effect := feedback.CarriedDecision{Source: decision}
		if survivor, found := kept[VerdictLookupKey(decision.ClaimKind, decision.ClaimKey)]; found {
			effect.Target = &survivor
		}
		out = append(out, effect)
	}
	return out, nil
}

func readCarriedDecisions(ctx context.Context, tx pgx.Tx, kind string, subject ids.UUID, source *ids.UUID) ([]feedback.StoredDecision, error) {
	// The native contacts merge matches the same two-key ledger identity; cross-module reads stay with this owner.
	rows, err := tx.Query(ctx, `SELECT id, claim_kind, claim_key, verdict, corrected_value, note,
 value_shown, value_captured_at, captured_by, updated_at, version FROM ai_feedback f
 WHERE subject_type = @kind AND subject_id = @subject AND (@source::uuid IS NULL OR EXISTS (
 SELECT 1 FROM ai_feedback origin WHERE origin.subject_type = @kind AND origin.subject_id = @source
 AND origin.claim_kind = f.claim_kind AND origin.claim_key = f.claim_key))
 ORDER BY claim_kind, claim_key LIMIT 501`, pgx.StrictNamedArgs{"kind": kind, "subject": subject, rateSourceField: source})
	if err != nil {
		return nil, err
	}
	decisions, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (feedback.StoredDecision, error) {
		var decision feedback.StoredDecision
		err := row.Scan(&decision.ID, &decision.ClaimKind, &decision.ClaimKey, &decision.Verdict,
			&decision.CorrectedValue, &decision.Note, &decision.ValueShown, &decision.ValueCapturedAt,
			&decision.CapturedBy, &decision.UpdatedAt, &decision.Version)
		return decision, err
	})
	if err != nil {
		return nil, err
	}
	if len(decisions) > 500 {
		return nil, apperrors.ErrInvalidArgument
	}
	return decisions, nil
}
