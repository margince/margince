// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What ran without anybody being asked, and the way back from it.
//
// Two sources, one lane. An approval the system decided is a decision somebody
// could already revisit through the record it named. A close-date correction is
// not: it was applied when it was made, no card was ever staged, and this
// receipt is the only telling the owner gets — so it carries its own Undo.

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// approvalStatusApproved is the decided status a receipt is read from. Spelled
// once here because the receipt lane asks for it by name, and a typo would
// quietly return an empty lane rather than an error.
const approvalStatusApproved = "approved"

// attentionReceipts reads what ran without asking.
//
// The test is the decision's own decided_by_system marker. It used to be
// decided_by IS NULL, inferring "nobody decided" from an empty column, and that
// read the wrong thing twice over: no writer produces approved-with-no-decider,
// and deleting an app_user empties decided_by on every approval that contact
// decided — which would move their decisions into a lane headed "Done for you".
// Filtering on status alone would do the same thing to every reader's own
// approvals, which is the one claim this lane exists to make.
type attentionReceipts struct {
	tasks *activities.Store
	svc   *approvals.Service
	deals *deals.Store
}

func (r attentionReceipts) Recent(ctx context.Context, since time.Time, limit int) ([]attention.Receipt, error) {
	receipts, _, err := r.RecentWithMore(ctx, since, limit)
	return receipts, err
}

// RecentWithMore is Recent, and whether either source holds more than it
// returned.
func (r attentionReceipts) RecentWithMore(ctx context.Context, since time.Time, limit int) ([]attention.Receipt, bool, error) {
	decided, more, err := attention.RecentReceipts(since, limit, func(cursor string, width int) ([]crmcontracts.Approval, storekit.Page, error) {
		status := approvalStatusApproved
		bySystem := true
		return r.svc.ListWire(ctx, approvals.ListInput{
			Status: &status, DecidedBySystem: &bySystem, DecidedAfter: &since, Limit: width, Cursor: cursor,
		})
	})
	if err != nil {
		return nil, false, err
	}
	// The close-date sweep's own corrections, which no approval records.
	//
	// They are applied when they are made rather than staged, so a lane reading
	// only approvals would report a quiet night on a morning when every date in
	// the pipeline had moved. This is the sole telling, which is also why each
	// row carries the way back.
	// A reader with no deal grant loses the CORRECTIONS, not the panel.
	//
	// The two sources answer one question between them, and this one is
	// optional: a seat that may not read deals has no close-date corrections to
	// be told about, while it may well have approvals the system decided for it.
	// Propagating the refusal took the whole receipt surface away over a grant
	// that has nothing to do with the rows it was hiding.
	corrections, err := r.deals.RecentCorrectionsOwnedBy(ctx, since, limit+1)
	if err != nil && !errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil, false, err
	}
	if len(corrections) > limit {
		corrections, more = corrections[:limit], true
	}
	for _, correction := range corrections {
		decided = append(decided, attention.CorrectionReceipt(correction))
	}
	if r.tasks != nil {
		completions, err := r.tasks.RecentTaskCompletions(ctx, since, limit+1)
		if err != nil && !errors.Is(err, apperrors.ErrPermissionDenied) {
			return nil, false, err
		}
		if len(completions) > limit {
			completions, more = completions[:limit], true
		}
		for _, completed := range completions {
			var undo *attention.ReceiptUndo
			if completed.Writable {
				undo = &attention.ReceiptUndo{AuditLogID: completed.AuditID, Version: completed.Version}
			}
			said := approvalSummaryCopyFor(systemtext.Locale(ctx))
			contact := completed.Contact
			if contact == "" {
				contact = said.taskContact
			}
			reason := fmt.Sprintf(said.taskCompleted, contact, completed.WrittenAt.Format(time.DateOnly))
			decided = append(decided, attention.Receipt{Source: attention.ReceiptFromCorrection, ID: completed.AuditID, AuditLogID: completed.AuditID, Kind: "task_completed", Summary: reason, Headline: completed.Subject, Reason: reason, OccurredAt: completed.At, TargetType: string(recordTypeActivity), TargetID: completed.ID, Actor: attention.ReceiptActor{Type: systemActor}, Undo: undo})
		}
	}
	decided, err = r.withChangeReviews(ctx, decided)
	if err != nil {
		return nil, false, err
	}
	// Newest first across BOTH sources: merged by appending, the corrections
	// would sit under every approval however recent, and the reader's most
	// recent news would be the furthest down the card.
	// The id breaks a tie, so every read of one window pages it alike.
	sort.Slice(decided, func(i, j int) bool {
		if !decided[i].OccurredAt.Equal(decided[j].OccurredAt) {
			return decided[i].OccurredAt.After(decided[j].OccurredAt)
		}
		return decided[i].ID.String() > decided[j].ID.String()
	})
	if len(decided) > limit {
		decided, more = decided[:limit], true
	}
	return decided, more, nil
}

func (r attentionReceipts) withChangeReviews(ctx context.Context, receipts []attention.Receipt) ([]attention.Receipt, error) {
	keys := make([]deals.AppliedChangeKey, 0, len(receipts))
	for _, receipt := range receipts {
		if receipt.TargetType == approvalTargetDeal && receipt.Undo != nil {
			keys = append(keys, deals.AppliedChangeKey{DealID: ids.From[ids.DealKind](receipt.TargetID), ChangeID: receipt.ID})
		}
	}
	if len(keys) == 0 {
		return receipts, nil
	}
	reviews, err := r.deals.AppliedChangeReviews(ctx, keys)
	if err != nil && !errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil, err
	}
	for i := range receipts {
		key := deals.AppliedChangeKey{DealID: ids.From[ids.DealKind](receipts[i].TargetID), ChangeID: receipts[i].ID}
		if review, ok := reviews[key]; ok {
			receipts[i].Review = &review
		} else if receipts[i].TargetType == approvalTargetDeal {
			// The approval reader already authorized the receipt. A missing review
			// cannot erase that history or offer a restore against unverified state.
			receipts[i].Undo = nil
		}
	}
	return receipts, nil
}
