// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// Rendering a completed act as a card.
//
// Its own file because a receipt is the one card on this surface that reports
// rather than asks, and the one that may carry a verb without the reader having
// been asked anything: a change nobody agreed to has to travel with its way
// back, because no approval row exists to go and reject.

import (
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// receiptItem renders one thing the system did on its own.
//
// It offers no decision: a receipt reports a finished act, and asking the reader
// to answer a question already answered is not a verb this lane has.
//
// It offers `open` only when the decision named a record. Not every approval is
// about one, and a card that advertised the verb regardless would send a client
// that trusts it to a destination the card never carried.
func receiptItem(receipt Receipt) crmcontracts.AttentionItem {
	kind := receipt.Kind
	occurred := receipt.OccurredAt
	summary := receipt.Summary
	subject := subjectOf(receipt.TargetType, receipt.TargetID)
	actions := []crmcontracts.AttentionItemActions{}
	if openableSubject(subject) {
		actions = append(actions, actionOpen)
	}
	item := crmcontracts.AttentionItem{
		Id:         receipt.ID.String(),
		Source:     crmcontracts.AttentionItemSource("approval"),
		Kind:       &kind,
		Title:      &summary,
		Subject:    subject,
		OccurredAt: &occurred,
		Actions:    actions,
	}
	// A change made with nobody asked carries the way back.
	//
	// Offered only while it is still there to take: a correction somebody has
	// already reversed keeps the payload — the row says so rather than
	// vanishing — but the verb goes, because a button that would put back what
	// is already back is one press that reports success and does nothing.
	if undo := receipt.Undo; undo != nil {
		version := undo.Version
		item.Version = &version
		item.Undo = &crmcontracts.AppliedUndo{
			AuditLogId: openapi_types.UUID(undo.AuditLogID),
			// The same version the item carries. Both, not one: the restore
			// route REQUIRES If-Match, and a client reading the version off the
			// undo it is about to press would otherwise send zero and be
			// refused for a conflict that is not one.
			Version:  undo.Version,
			Reversed: undo.Reversed,
		}
		if !undo.Reversed {
			item.Actions = append(item.Actions, actionUndo)
		}
	}
	return item
}
