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
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/deadline"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
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
	receipts, _, err := r.RecentWithMore(ctx, since, limit)
	return receipts, err
}

// RecentWithMore is Recent, and whether either source holds more than it
// returned.
func (r attentionReceipts) RecentWithMore(ctx context.Context, since time.Time, limit int) ([]attention.Receipt, bool, error) {
	decided, more, err := recentReceipts(since, limit, func(cursor string, width int) ([]crmcontracts.Approval, storekit.Page, error) {
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
		decided = append(decided, correctionReceipt(correction))
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

// correctionReceipt renders one applied correction as a card.
//
// A changed date names both values; confidence-only changes must not claim
// the date moved. The recorded basis explains why the sweep acted.
func correctionReceipt(c deals.CorrectionReceipt) attention.Receipt {
	summary := fmt.Sprintf("Updated close-date confidence on %q", c.DealName)
	if slices.Contains(c.Fields, "expected_close_date") {
		summary = fmt.Sprintf("Changed the close date on %q: %s → %s", c.DealName, correctionDate(c.BeforeClose), correctionDate(c.AfterClose))
	}
	if slices.Contains(c.Fields, "forecast_category") {
		summary += ". Forecast category also changed"
	}
	if c.Basis != "" {
		summary += " — " + c.Basis
	}
	return attention.Receipt{
		ID:              c.AuditLogID,
		CloseDateChange: correctionValues(c),
		Kind:            deals.CloseDateCorrectionKind,
		Summary:         summary,
		OccurredAt:      c.AppliedAt,
		TargetType:      approvalTargetDeal,
		TargetID:        c.DealID.UUID,
		Actor:           attention.ReceiptActor{Type: c.ActorType, ID: c.ActorID, OnBehalfOf: c.OnBehalfOf},
		Undo: &attention.ReceiptUndo{
			AuditLogID: c.AuditLogID,
			Version:    c.Version,
			Reversed:   c.Reversed,
		},
	}
}

// approvalPageWidth is the widest page the approvals inbox serves. A wider
// ask is answered with its default page, so a longer read pages instead.
const approvalPageWidth = 200

// recentReceipts turns the store's rows into the lane's cards, paging the
// inbox until `limit` rows came back or the inbox said it had no more.
//
// The window belongs in SQL with the rest. The page is ordered by created_at
// and the window is about decided_at. A window applied afterwards could drop
// a whole page and hide a recent decision. The re-check in
// receiptsWithin is what makes the deref of DecidedAt safe in this package.
func recentReceipts(
	since time.Time, limit int,
	page func(cursor string, width int) ([]crmcontracts.Approval, storekit.Page, error),
) ([]attention.Receipt, bool, error) {
	var rows []crmcontracts.Approval
	cursor := ""
	for len(rows) < limit {
		got, next, err := page(cursor, min(approvalPageWidth, limit-len(rows)))
		if err != nil {
			return nil, false, err
		}
		rows = append(rows, got...)
		if !next.HasMore || next.NextCursor == "" {
			return receiptsWithin(rows, since), false, nil
		}
		cursor = next.NextCursor
	}
	return receiptsWithin(rows, since), true, nil
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
			ID:               ids.UUID(row.Id),
			Kind:             row.Kind,
			Summary:          summary,
			SummaryParts:     row.SummaryParts,
			OccurredAt:       *row.DecidedAt,
			Actor:            proposerOf(row),
			SourceActivityID: citedActivity(row.Evidence),
		}
		// Both or neither: a type with no id names nothing, and an id with no
		// type says where to look without saying at what.
		if row.TargetEntityType != nil && row.TargetEntityId != nil {
			receipt.TargetType = *row.TargetEntityType
			receipt.TargetID = ids.UUID(*row.TargetEntityId)
			receipt.TargetLabel = row.TargetLabel
		}
		out = append(out, receipt)
	}
	return out
}

func (r attentionReceipts) withChangeReviews(ctx context.Context, receipts []attention.Receipt) ([]attention.Receipt, error) {
	keys := make([]deals.AppliedChangeKey, 0, len(receipts))
	for _, receipt := range receipts {
		if receipt.TargetType == approvalTargetDeal && (receipt.Kind == deals.StageProgressionKind || receipt.Undo != nil) {
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

func correctionDate(value *string) string {
	if value == nil {
		return "not set"
	}
	return *value
}

// proposerOf is who staged a decision the system then applied: the member or
// agent whose work the receipt reports. An id outside the principal
// vocabulary names nobody, so the receipt says nothing about who.
func proposerOf(row crmcontracts.Approval) attention.ReceiptActor {
	kind, _, _ := strings.Cut(row.ProposedBy, ":")
	switch principal.PrincipalType(kind) {
	case principal.PrincipalHuman, principal.PrincipalAgent, principal.PrincipalConnector, principal.PrincipalSystem:
	default:
		return attention.ReceiptActor{}
	}
	actor := attention.ReceiptActor{Type: kind, ID: row.ProposedBy}
	if row.OnBehalfOf != nil {
		on := ids.UUID(*row.OnBehalfOf)
		actor.OnBehalfOf = &on
	}
	return actor
}

// citedActivity is the first message a decision's evidence was read from.
func citedActivity(evidence *[]crmcontracts.ApprovalEvidence) ids.UUID {
	if evidence == nil {
		return ids.Nil
	}
	for _, claim := range *evidence {
		if claim.SourceType != nil && *claim.SourceType == crmcontracts.ApprovalEvidenceSourceTypeActivity && claim.SourceId != nil {
			return ids.UUID(*claim.SourceId)
		}
	}
	return ids.Nil
}
