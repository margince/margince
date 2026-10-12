// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// withoutScoreOverrideClears takes the score pair out of the named clears and
// reports whether either was there. A null on either field ends an override.
// Ending one restores the machine score rather than writing NULL to a column,
// so applyScoreOverride owns it, not ApplyClears.
func withoutScoreOverrideClears(fields []string) ([]string, bool) {
	kept := make([]string, 0, len(fields))
	named := false
	for _, field := range fields {
		if field == leadScoreField || field == leadScoreOverrideReasonField {
			named = true
			continue
		}
		kept = append(kept, field)
	}
	return kept, named
}

// applyScoreOverride folds the sticky score override into the patch and
// reports whether the caller must resume recompute (an override was cleared).
// Setting `score` needs a non-empty reason and keeps the machine value in
// score_computed. A null on score or the reason clears the override. A reason
// with no score amends the note on an override in force; "" is invalid input.
func applyScoreOverride(p *storekit.Patch, current crmcontracts.Lead, in UpdateLeadInput) (resumeRecompute bool, err error) {
	overrideInForce := current.ScoreOverrideReason != nil

	switch {
	case in.Score != nil:
		reason := ""
		if in.ScoreOverrideReason != nil {
			reason = strings.TrimSpace(*in.ScoreOverrideReason)
		}
		if reason == "" {
			return false, &ScoreOverrideReasonRequiredError{}
		}
		p.Set("score", current.Score, *in.Score)
		p.Set("score_override_reason", current.ScoreOverrideReason, reason)
		// Keep the machine value only when the override first takes hold.
		// Under an override already in force, score_computed holds it and
		// recompute keeps it fresh, so a human number must not replace it.
		if !overrideInForce {
			p.Set("score_computed", current.ScoreComputed, current.Score)
		}
		return false, nil

	case in.ClearScoreOverride:
		if in.ScoreOverrideReason != nil {
			return false, &ScoreOverrideClearConflictError{}
		}
		if !overrideInForce {
			return false, nil // no override to clear: a no-op
		}
		p.Set("score_override_reason", current.ScoreOverrideReason, nil)
		// Resume: score tracks the retained machine value, then recompute
		// refines it from current signals.
		if current.ScoreComputed != nil {
			p.Set("score", current.Score, *current.ScoreComputed)
		}
		p.Set("score_computed", current.ScoreComputed, nil)
		return true, nil

	case in.ScoreOverrideReason != nil:
		if strings.TrimSpace(*in.ScoreOverrideReason) == "" {
			return false, &ScoreOverrideReasonEmptyError{}
		}
		if !overrideInForce {
			return false, &ScoreOverrideWithoutScoreError{}
		}
		p.Set("score_override_reason", current.ScoreOverrideReason, strings.TrimSpace(*in.ScoreOverrideReason))
		return false, nil
	}
	return false, nil
}
