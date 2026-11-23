// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// amountChange is the verdict's amount correction and the audit entry it wrote.
func (v *dealVerdictEnv) amountChange(t *testing.T) (ids.UUID, ids.UUID) {
	t.Helper()
	correction := v.WsScalar(t, `SELECT id::text FROM deal_correction WHERE deal_id = $1 AND correction = $2`, v.deal, deals.VerdictAmountCorrection)
	audit := v.WsScalar(t, `SELECT audit_log_id::text FROM deal_correction WHERE deal_id = $1 AND correction = $2`, v.deal, deals.VerdictAmountCorrection)
	return ids.MustParse(correction), ids.MustParse(audit)
}

func (v *dealVerdictEnv) setAmount(t *testing.T, minor int64) {
	t.Helper()
	if _, err := v.Deals.UpdateDeal(v.owner(t), ids.From[ids.DealKind](v.deal), deals.UpdateDealInput{AmountMinor: &minor}); err != nil {
		t.Fatal(err)
	}
}

// A person sets the amount away from Margince's value and back to it. That is
// their own choice, so Undo must not put the earlier value over it. The values
// alone cannot tell this from an untouched amount.
func TestAnAmountEditedAwayAndBackIsNoLongerUndoable(t *testing.T) {
	v := setupDealVerdict(t)
	v.run(t)
	correction, audit := v.amountChange(t)
	review, err := v.Deals.AppliedChangeReview(v.owner(t), ids.From[ids.DealKind](v.deal), audit)
	if err != nil || !review.CanUndo {
		t.Fatalf("before any edit the amount review is %+v (%v), want Undo open", review, err)
	}

	v.setAmount(t, 5_000_000)
	v.setAmount(t, 4_500_000)

	review, err = v.Deals.AppliedChangeReview(v.owner(t), ids.From[ids.DealKind](v.deal), audit)
	if err != nil || review.CanUndo {
		t.Fatalf("after 45,000 to 50,000 and back the review is %+v (%v), want no Undo", review, err)
	}
	if _, err := v.Deals.RevertCorrection(v.owner(t), correction, nil, nil); err == nil {
		t.Fatal("Undo put Margince's earlier amount over the person's final choice")
	} else if _, ok := errors.AsType[*deals.CorrectionReversalError](err); !ok {
		t.Fatalf("Undo refused with %v, want a reversal refusal", err)
	}
}

// A later write to a field the change never touched leaves Undo open.
func TestALaterWriteToAnotherFieldKeepsUndo(t *testing.T) {
	v := setupDealVerdict(t)
	v.run(t)
	correction, audit := v.amountChange(t)
	name := "Renamed by a person"
	if _, err := v.Deals.UpdateDeal(v.owner(t), ids.From[ids.DealKind](v.deal), deals.UpdateDealInput{Name: &name}); err != nil {
		t.Fatal(err)
	}
	review, err := v.Deals.AppliedChangeReview(v.owner(t), ids.From[ids.DealKind](v.deal), audit)
	if err != nil || !review.CanUndo {
		t.Fatalf("after an unrelated edit the review is %+v (%v), want Undo open", review, err)
	}
	if _, err := v.Deals.RevertCorrection(v.owner(t), correction, nil, nil); err != nil {
		t.Fatalf("Undo after an unrelated edit: %v", err)
	}
}

// A text field is compared as the exact text it is. Two names that read as one
// number or one instant stay two names, and a value shaped like a bad date
// cannot fail the review.
func TestTextThatLooksLikeANumberOrADateIsNotReadAsOne(t *testing.T) {
	v := setupDealVerdict(t)
	v.run(t)
	_, audit := v.amountChange(t)
	for _, name := range []string{"1", "1.0", "2026-99-99T99:99 A", "2026-11-23T10:00Z"} {
		if _, err := v.Deals.UpdateDeal(v.owner(t), ids.From[ids.DealKind](v.deal), deals.UpdateDealInput{Name: &name}); err != nil {
			t.Fatal(err)
		}
		review, err := v.Deals.AppliedChangeReview(v.owner(t), ids.From[ids.DealKind](v.deal), audit)
		if err != nil || !review.CanUndo {
			t.Fatalf("with the deal named %q the review is %+v (%v), want it read and Undo open", name, review, err)
		}
	}
}
