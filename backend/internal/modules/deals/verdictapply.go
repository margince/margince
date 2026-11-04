// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Margince setting a deal's close date, amount or probability for its owner,
// from the nightly reading of the deal's evidence. Each change is its own
// correction, so the owner can undo one and keep the others.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The corrections a verdict records. The amount and probability names are
// also the owner's switches for them; a close date answers to the
// close-date switch.
const (
	VerdictCloseDateCorrection   = "deal_close_date"
	VerdictAmountCorrection      = "deal_amount"
	VerdictProbabilityCorrection = "deal_probability"
	VerdictStageMoveCorrection   = "stage_move"
)

// VerdictSwitch is the owner's switch a verdict correction answers to.
func VerdictSwitch(correction string) string {
	switch correction {
	case VerdictCloseDateCorrection:
		return CloseDateCorrectionKind
	case VerdictStageMoveCorrection:
		return DealStageMoveKind
	}
	return correction
}

// Why a verdict change was not applied, as the verdict records it.
const (
	VerdictApplied         = "applied"
	VerdictDroppedStale    = "dropped:stale"
	VerdictDroppedSwitch   = "dropped:switch_off"
	VerdictDroppedHuman    = "dropped:human_edit"
	VerdictDroppedNoChange = "dropped:unchanged"
	// VerdictDroppedMoved is a deal whose evidence moved between the read and
	// the write. Nothing applies, and the deal is read again.
	VerdictDroppedMoved = "dropped:evidence_moved"
)

// VerdictValues are the deal fields a verdict may change, as one read saw them.
type VerdictValues struct {
	CloseDate *time.Time
	// Provisional says nobody confirmed CloseDate.
	Provisional bool
	AmountMinor *int64
	Currency    *string
	Probability *int
	// ProbabilitySource is who set Probability; nil while the deal follows
	// its stage.
	ProbabilitySource *string
	StageID           ids.UUID
}

// VerdictChange is one change a verdict decided on.
type VerdictChange struct {
	Correction  string
	CloseDate   time.Time
	AmountMinor int64
	// Currency is set only when the deal has none: the amount then keeps the
	// currency the customer named.
	Currency    string
	Probability int
	// ToStageID is where a stage move goes. CriteriaMet names the exit
	// criteria the move's evidence covers; the lock asks again that they are
	// every required one of each stage the deal leaves.
	ToStageID   ids.StageID
	CriteriaMet []ids.UUID
	// NewestCitedAt is the newest activity the change rests on. A human edit
	// of the field after it wins.
	NewestCitedAt time.Time
	// Evidence goes on the audit row: the reason, the cited activities, and
	// for an amount the quote and the conversion.
	Evidence map[string]any
}

// VerdictInput is one deal's changes, and the values they were decided on.
type VerdictInput struct {
	DealID ids.DealID
	Owner  ids.UUID
	Seen   VerdictValues
	// Changes holds at most one change per correction.
	Changes []VerdictChange
	// Allowed asks, under the deal row lock, whether the owner's switch for a
	// correction is still on. The switches are not this module's.
	Allowed func(ctx context.Context, tx pgx.Tx, correction string) (bool, error)
	// EvidenceHolds asks, under the deal row lock and before any write,
	// whether the evidence the verdict read still holds. That is the
	// checksum, the cited activities and the owner, none of them this
	// module's.
	EvidenceHolds func(ctx context.Context, tx pgx.Tx) (bool, error)
	// Record writes the timeline entry for what applied, in the same
	// transaction. It is not called when nothing applied.
	Record func(ctx context.Context, tx pgx.Tx, applied []VerdictChange) error
}

// VerdictResult is each change's outcome, in the order of the input, and
// whether the evidence still held under the lock.
type VerdictResult struct {
	Outcomes      []string
	EvidenceMoved bool
}

// ApplyVerdict applies a verdict's changes for the deal's owner and reports
// each change's outcome, in the order of in.Changes. A deal that closed,
// changed owner or went on legal hold since the decision changes nothing.
func (s *Store) ApplyVerdict(ctx context.Context, in VerdictInput) (VerdictResult, error) {
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return VerdictResult{}, err
	}
	active, err := s.activeColumns(ctx)
	if err != nil {
		return VerdictResult{}, err
	}
	result := VerdictResult{Outcomes: make([]string, len(in.Changes))}
	outcomes := result.Outcomes
	for i := range outcomes {
		outcomes[i] = VerdictDroppedStale
	}
	err = s.Tx(ctx, func(tx pgx.Tx) error {
		lock, current, ok, err := lockVerdictDeal(ctx, tx, in)
		if err != nil || !ok {
			return err
		}
		holds, err := in.EvidenceHolds(ctx, tx)
		if err != nil {
			return err
		}
		if !holds {
			for i := range outcomes {
				outcomes[i] = VerdictDroppedMoved
			}
			result.EvidenceMoved = true
			return nil
		}
		var applied []VerdictChange
		// Asked of the deal as locked, before a stage move resets its
		// probability: that reset is ours, not a colleague's edit.
		heldStill := make([]bool, len(in.Changes))
		for i, change := range in.Changes {
			_, heldStill[i] = verdictColumns(change.Correction, in.Seen, current)
		}
		for _, i := range stageMoveFirst(in.Changes) {
			change := in.Changes[i]
			var outcome string
			switch {
			case !heldStill[i]:
				outcome = VerdictDroppedStale
			case change.Correction == VerdictStageMoveCorrection:
				outcome, current, err = s.applyVerdictStageMove(ctx, tx, in, current, change, active)
			default:
				now := in
				now.Seen = current
				outcome, err = applyVerdictChange(ctx, tx, lock, now, current, change)
			}
			if err != nil {
				return err
			}
			outcomes[i] = outcome
			if outcome == VerdictApplied {
				applied = append(applied, change)
			}
		}
		if len(applied) == 0 {
			return nil
		}
		return in.Record(ctx, tx, applied)
	})
	return result, err
}

// lockVerdictDeal locks the deal and re-asks what the decision rested on:
// open, not on hold, the same owner, and maintenance writes switched on.
func lockVerdictDeal(ctx context.Context, tx pgx.Tx, in VerdictInput) (storekit.RowLock, VerdictValues, bool, error) {
	lock, err := storekit.LockRow(ctx, tx, dealTable, in.DealID.UUID, storekit.LiveOnly)
	if errors.Is(err, apperrors.ErrNotFound) {
		return lock, VerdictValues{}, false, nil
	}
	if err != nil {
		return lock, VerdictValues{}, false, err
	}
	// The owner's agent writes only what the owner may write.
	if err := auth.EnsureWritable(ctx, tx, dealTable, in.DealID.UUID); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) {
			return lock, VerdictValues{}, false, nil
		}
		return lock, VerdictValues{}, false, err
	}
	var status string
	var owner *ids.UUID
	var legalHold bool
	var current VerdictValues
	if err := tx.QueryRow(ctx, `
		SELECT status, owner_id, legal_hold, expected_close_date, close_date_provisional, amount_minor, currency::text,
		       win_probability_override, win_probability_set_by, stage_id
		  FROM deal WHERE id = $1`, in.DealID).
		Scan(&status, &owner, &legalHold, &current.CloseDate, &current.Provisional, &current.AmountMinor, &current.Currency,
			&current.Probability, &current.ProbabilitySource, &current.StageID); err != nil {
		return lock, VerdictValues{}, false, fmt.Errorf("deals: reading the deal a verdict changes: %w", err)
	}
	if DealStatus(status) != DealOpen || legalHold || owner == nil || *owner != in.Owner {
		return lock, current, false, nil
	}
	on, err := config.StoredApplyTx(ctx, tx, config.Bound(), MaintenanceWritesEnabled)
	if err != nil || !on {
		return lock, current, false, err
	}
	return lock, current, true, nil
}

// applyVerdictChange writes one change, unless its switch is off, the field
// moved since the decision, or a colleague edited it after the evidence.
func applyVerdictChange(
	ctx context.Context, tx pgx.Tx, lock storekit.RowLock, in VerdictInput, current VerdictValues, change VerdictChange,
) (string, error) {
	allowed, err := in.Allowed(ctx, tx, change.Correction)
	if err != nil {
		return "", err
	}
	if !allowed {
		return VerdictDroppedSwitch, nil
	}
	columns, unchanged := verdictColumns(change.Correction, in.Seen, current)
	if columns == nil || !unchanged {
		return VerdictDroppedStale, nil
	}
	edited, err := humanEditedAfter(ctx, tx, in.DealID, columns, change.NewestCitedAt)
	if err != nil {
		return "", err
	}
	if edited {
		return VerdictDroppedHuman, nil
	}
	patch := storekit.NewPatch()
	if !verdictPatch(patch, change, current) {
		return VerdictDroppedStale, nil
	}
	if patch.Empty() {
		return VerdictDroppedNoChange, nil
	}
	evidence := map[string]any{CorrectionKindEvidenceKey: change.Correction, "basis_owner": in.Owner.String()}
	for key, value := range change.Evidence {
		evidence[key] = value
	}
	_, err = commitCorrection(ctx, tx, lock, correctionWrite{
		deal: in.DealID, correction: change.Correction, patch: patch, auditEvidence: evidence,
		changedFields: map[string]any{CorrectionKindEvidenceKey: change.Correction},
	})
	if err != nil {
		return "", err
	}
	return VerdictApplied, nil
}

// verdictColumns names the columns a correction writes, and whether they still
// hold what the decision saw.
func verdictColumns(correction string, seen, current VerdictValues) ([]string, bool) {
	switch correction {
	case VerdictCloseDateCorrection:
		return []string{closeDateField, provisionalField},
			sameDate(seen.CloseDate, current.CloseDate) && seen.Provisional == current.Provisional
	case VerdictAmountCorrection:
		return []string{"amount_minor", "currency"},
			sameInt64(seen.AmountMinor, current.AmountMinor) && sameString(seen.Currency, current.Currency)
	case VerdictProbabilityCorrection:
		return []string{winProbabilityOverrideColumn},
			sameInt(seen.Probability, current.Probability) && sameString(seen.ProbabilitySource, current.ProbabilitySource)
	case VerdictStageMoveCorrection:
		return []string{"stage_id"}, seen.StageID == current.StageID
	}
	return nil, false
}

// verdictPatch sets the change's value. It reports false for a change the
// deal can no longer take. That is a probability a colleague set, or an
// amount with no currency on either side.
func verdictPatch(patch *storekit.Patch, change VerdictChange, current VerdictValues) bool {
	switch change.Correction {
	case VerdictCloseDateCorrection:
		// The customer named this date, so it is no longer an estimate.
		date := change.CloseDate
		patch.SetDate(closeDateField, current.CloseDate, &date)
		if current.Provisional {
			patch.Set(provisionalField, true, false)
		}
	case VerdictAmountCorrection:
		if current.Currency == nil && change.Currency == "" {
			return false
		}
		if current.Currency == nil {
			patch.Set("currency", nil, change.Currency)
		}
		if current.AmountMinor == nil || *current.AmountMinor != change.AmountMinor {
			patch.Set("amount_minor", current.AmountMinor, change.AmountMinor)
		}
	case VerdictProbabilityCorrection:
		if current.ProbabilitySource != nil && Source(*current.ProbabilitySource) == SourceHuman {
			return false
		}
		pct := change.Probability
		setWinProbability(patch, current.Probability, current.ProbabilitySource, &pct, SourceMargince)
	}
	return true
}

// humanEditedAfter reports whether a colleague wrote one of the columns after
// the newest evidence a change rests on.
func humanEditedAfter(ctx context.Context, tx pgx.Tx, deal ids.DealID, columns []string, after time.Time) (bool, error) {
	var edited bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM audit_log
		                WHERE entity_type = 'deal' AND entity_id = $1 AND actor_type = $2
		                  AND after ?| $3 AND occurred_at > $4)`,
		deal, string(principal.PrincipalHuman), columns, after).Scan(&edited)
	if err != nil {
		return false, fmt.Errorf("deals: reading who last edited the deal: %w", err)
	}
	return edited, nil
}

func sameDate(a, b *time.Time) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && a.Format(time.DateOnly) == b.Format(time.DateOnly))
}

func sameInt64(a, b *int64) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}
