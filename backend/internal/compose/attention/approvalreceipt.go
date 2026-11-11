// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"strings"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/deadline"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// approvalPageWidth is the widest page the approvals inbox serves. A wider
// ask is answered with its default page, so a longer read pages instead.
const approvalPageWidth = 200

// RecentReceipts turns the store's rows into the lane's cards, paging the
// inbox until `limit` rows came back or the inbox said it had no more.
//
// The window belongs in SQL with the rest. The page is ordered by created_at
// and the window is about decided_at. A window applied afterwards could drop
// a whole page and hide a recent decision. The re-check in
// ReceiptsWithin is what makes the deref of DecidedAt safe in this package.
func RecentReceipts(
	since time.Time, limit int,
	page func(cursor string, width int) ([]crmcontracts.Approval, storekit.Page, error),
) ([]Receipt, bool, error) {
	var rows []crmcontracts.Approval
	cursor := ""
	for len(rows) < limit {
		got, next, err := page(cursor, min(approvalPageWidth, limit-len(rows)))
		if err != nil {
			return nil, false, err
		}
		rows = append(rows, got...)
		if !next.HasMore || next.NextCursor == "" {
			return ReceiptsWithin(rows, since), false, nil
		}
		cursor = next.NextCursor
	}
	return ReceiptsWithin(rows, since), true, nil
}

// ReceiptsWithin keeps the decided rows inside the lane's window.
func ReceiptsWithin(rows []crmcontracts.Approval, since time.Time) []Receipt {
	out := make([]Receipt, 0, len(rows))
	for _, row := range rows {
		// Inside the window, not before it. `since` is the lane's horizon, and
		// deadline.Passed answers it here as it does for a task's due date.
		if row.DecidedAt == nil || deadline.Passed(row.DecidedAt, since) {
			continue
		}
		summary := ""
		if row.Summary != nil {
			summary = *row.Summary
		}
		receipt := Receipt{
			Source:           ReceiptFromApproval,
			ID:               ids.UUID(row.Id),
			Kind:             row.Kind,
			Summary:          summary,
			SummaryParts:     row.SummaryParts,
			SystemDisplay:    row.SystemDisplay,
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

// proposerOf is who staged a decision the system then applied: the member or
// agent whose work the receipt reports. An id outside the principal
// vocabulary names nobody, so the receipt says nothing about who.
func proposerOf(row crmcontracts.Approval) ReceiptActor {
	kind, _, _ := strings.Cut(row.ProposedBy, ":")
	switch principal.PrincipalType(kind) {
	case principal.PrincipalHuman, principal.PrincipalAgent, principal.PrincipalConnector, principal.PrincipalSystem:
	default:
		return ReceiptActor{}
	}
	actor := ReceiptActor{Type: kind, ID: row.ProposedBy}
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
