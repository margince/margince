// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// SuggestionRepairVersion binds reviewed plans to the owner’s retirement rules.
const SuggestionRepairVersion = "1"

// RepairReasonCurrentWork and its peers explain why reviewed work is kept or retired.
const (
	RepairReasonCurrentWork     = "current_work"
	RepairReasonUnverified      = "source_rule_not_verified"
	RepairReasonHumanAdoption   = "human_adoption_not_excluded"
	RepairReasonCompanyHasDeal  = "company_has_deal"
	RepairReasonSourceChanged   = "suggestion_source_changed"
	RepairReasonBuyerUnverified = "buyer_no_longer_verified"
)

// SuggestionBuyerVerifier reuses current buyer admission at review and execution.
type SuggestionBuyerVerifier func(context.Context, pgx.Tx, Suggestion, time.Time) (bool, error)

// SuggestionRepairProvenance ties an automatic retirement to the reviewed run.
type SuggestionRepairProvenance struct {
	RuleVersion string
	RunID       ids.UUID
}

// SuggestionRepair binds the proposed outcome to the version actually reviewed.
type SuggestionRepair struct {
	ID        ids.UUID
	Version   int64
	Reason    string
	Supersede bool
}

// SuggestionRepairPageTx applies the ordinary evidence visibility boundary before classification.
func SuggestionRepairPageTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int, asOf time.Time, buyerVerified SuggestionBuyerVerifier) ([]SuggestionRepair, error) {
	if limit < 1 {
		return nil, apperrors.ErrInvalidArgument
	}
	limit = min(limit, suggestionMaxLimit)
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	visible, err := suggestionVisibleClause(ctx, arg, "")
	if err != nil {
		return nil, err
	}
	query := `SELECT ` + suggestionColumns + ` FROM deal_suggestion s JOIN company c ON c.id=s.company_id
 WHERE s.state='open' AND ` + visible +
		fmt.Sprintf(` AND s.id>$%d ORDER BY s.id LIMIT $%d`, arg(after), arg(limit))
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	candidates, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Suggestion, error) { return scanSuggestion(row) })
	if err != nil {
		return nil, err
	}
	if err := attachEvidence(ctx, tx, candidates); err != nil {
		return nil, err
	}
	out := make([]SuggestionRepair, 0, len(candidates))
	for _, candidate := range candidates {
		repair, err := suggestionRepairTx(ctx, tx, candidate, asOf, false, buyerVerified)
		if err != nil {
			return nil, err
		}
		out = append(out, repair)
	}
	return out, nil
}

// PreviewSuggestionRepairTx keeps inspection read-only while enforcing ordinary visibility.
func PreviewSuggestionRepairTx(ctx context.Context, tx pgx.Tx, id ids.UUID, asOf time.Time, buyerVerified SuggestionBuyerVerifier) (SuggestionRepair, error) {
	current, err := readVisibleSuggestionTx(ctx, tx, id, false)
	if err != nil {
		return SuggestionRepair{}, err
	}
	return suggestionRepairTx(ctx, tx, current, asOf, false, buyerVerified)
}

// SupersedeReviewedSuggestionTx never turns an automatic retirement into a human dismissal.
func SupersedeReviewedSuggestionTx(ctx context.Context, tx pgx.Tx, expected SuggestionRepair, provenance SuggestionRepairProvenance, asOf time.Time, buyerVerified SuggestionBuyerVerifier) (bool, error) {
	if provenance.RuleVersion == "" || provenance.RunID == ids.Nil {
		return false, apperrors.ErrInvalidArgument
	}
	if err := RequireSuggestionDecision(ctx, false); err != nil {
		return false, err
	}
	current, err := readVisibleSuggestionTx(ctx, tx, expected.ID, true)
	if err != nil {
		return false, err
	}
	actual, err := suggestionRepairTx(ctx, tx, current, asOf, true, buyerVerified)
	if err != nil {
		return false, err
	}
	if actual != expected {
		return false, apperrors.ErrConflict
	}
	if !actual.Supersede {
		return false, nil
	}
	if err := supersedeTx(ctx, tx, current.ID, map[string]any{"reason": actual.Reason, "rule_version": provenance.RuleVersion, "run_id": provenance.RunID}); err != nil {
		return false, err
	}
	return true, nil
}

func suggestionRepairTx(ctx context.Context, tx pgx.Tx, current Suggestion, asOf time.Time, lock bool, buyerVerified SuggestionBuyerVerifier) (SuggestionRepair, error) {
	out := SuggestionRepair{ID: current.ID, Version: current.Version, Reason: RepairReasonCurrentWork}
	if current.State != SuggestionOpen {
		return out, apperrors.ErrConflict
	}
	if current.Kind != "open_deal" {
		out.Reason = RepairReasonUnverified
		return out, nil
	}
	pristine, err := storekit.MachineCreatedUntouched(ctx, tx, suggestionEntity, current.ID)
	if err != nil {
		return out, err
	}
	if !pristine {
		out.Reason = RepairReasonHumanAdoption
		return out, nil
	}
	if buyerVerified == nil {
		return out, errors.New("deals: reviewing a suggestion needs its effects wired")
	}
	free, err := suggestionCompanyFreeTx(ctx, tx, current, lock)
	if err != nil {
		return out, err
	}
	evidence, err := suggestionEvidenceCurrentTx(ctx, tx, current)
	if err != nil {
		return out, err
	}
	buyer, err := buyerVerified(ctx, tx, current, asOf)
	if err != nil {
		return out, err
	}
	switch {
	case !free:
		out.Reason = RepairReasonCompanyHasDeal
	case !evidence:
		out.Reason = RepairReasonSourceChanged
	case !buyer:
		out.Reason = RepairReasonBuyerUnverified
	default:
		return out, nil
	}
	out.Supersede = true
	return out, nil
}
