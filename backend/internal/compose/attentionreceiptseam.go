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
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/deadline"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
	svc   *approvals.Service
	deals *deals.Store
}

func (r attentionReceipts) Recent(ctx context.Context, since time.Time, limit int) ([]attention.Receipt, error) {
	decided, err := recentReceipts(since, limit, func(scan int) ([]crmcontracts.Approval, error) {
		status := approvalStatusApproved
		bySystem := true
		rows, _, err := r.svc.ListWire(ctx, approvals.ListInput{
			Status: &status, DecidedBySystem: &bySystem, DecidedAfter: &since, Limit: scan,
		})
		return rows, err
	})
	if err != nil {
		return nil, err
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
	corrections, err := r.deals.RecentCorrectionsOwnedBy(ctx, since, limit)
	if err != nil && !errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil, err
	}
	for _, correction := range corrections {
		decided = append(decided, correctionReceipt(correction))
	}
	// Newest first across BOTH sources: merged by appending, the corrections
	// would sit under every approval however recent, and the reader's most
	// recent news would be the furthest down the card.
	sort.SliceStable(decided, func(i, j int) bool {
		return decided[i].OccurredAt.After(decided[j].OccurredAt)
	})
	if len(decided) > limit {
		decided = decided[:limit]
	}
	return decided, nil
}

// correctionReceipt renders one applied correction as a card.
//
// The summary names the deal and the reason rather than the fields: a rep reads
// "because nobody has answered since June", and which columns moved is what the
// Undo restores rather than what the sentence is about.
func correctionReceipt(c deals.CorrectionReceipt) attention.Receipt {
	summary := fmt.Sprintf("Corrected the close date on %q", c.DealName)
	if c.Basis != "" {
		summary = fmt.Sprintf("Corrected the close date on %q — %s", c.DealName, c.Basis)
	}
	return attention.Receipt{
		ID:         c.AuditLogID,
		Kind:       deals.CloseDateCorrectionKind,
		Summary:    summary,
		OccurredAt: c.AppliedAt,
		TargetType: approvalTargetDeal,
		TargetID:   c.DealID.UUID,
		Undo: &attention.ReceiptUndo{
			AuditLogID: c.AuditLogID,
			Version:    c.Version,
			Reversed:   c.Reversed,
		},
	}
}

// recentReceipts turns the store's rows into the lane's cards.
//
// The read is bounded by the lane rather than widened past it: the store answers
// "approved, decided by the system, decided since" itself, so the limit applies
// to rows that qualify. The window belongs in SQL with the rest — the page is
// ordered by created_at while the window is about decided_at, so a window
// applied afterwards can discard a whole page and hide a decision made minutes
// ago beneath approvals staged more recently.
//
// The re-check below is not a second filter. It is what makes the deref of
// DecidedAt safe in this package, where the SQL guaranteeing it is elsewhere.
//
// The page reader is a parameter so a test can answer exactly the width it was
// asked for; nothing else varies it.
func recentReceipts(
	since time.Time, limit int, page func(scan int) ([]crmcontracts.Approval, error),
) ([]attention.Receipt, error) {
	rows, err := page(limit)
	if err != nil {
		return nil, err
	}
	return receiptsWithin(rows, since), nil
}

// receiptsWithin keeps the decided rows inside the lane's window.
func receiptsWithin(rows []crmcontracts.Approval, since time.Time) []attention.Receipt {
	out := make([]attention.Receipt, 0, len(rows))
	for _, row := range rows {
		// Inside the window, not before it: `since` is the receipt lane's own
		// horizon, and the same authority answers "is this behind that" here as
		// answers it for a task's due date.
		if row.DecidedAt == nil || deadline.Passed(row.DecidedAt, since) {
			continue
		}
		summary := ""
		if row.Summary != nil {
			summary = *row.Summary
		}
		receipt := attention.Receipt{
			ID:         ids.UUID(row.Id),
			Kind:       row.Kind,
			Summary:    summary,
			OccurredAt: *row.DecidedAt,
		}
		// Both or neither: a type with no id names nothing, and an id with no
		// type says where to look without saying at what.
		if row.TargetEntityType != nil && row.TargetEntityId != nil {
			receipt.TargetType = *row.TargetEntityType
			receipt.TargetID = ids.UUID(*row.TargetEntityId)
		}
		out = append(out, receipt)
	}
	return out
}
