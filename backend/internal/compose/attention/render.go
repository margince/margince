// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// One item's shape, per producer.
//
// The rule every renderer here keeps: the title is what a person would say
// happened, and the identifiers stay in `id` and `subject` where a client uses
// them to navigate. A card that printed `organization_id` at a reader was
// showing them the plumbing and calling it information.

// duplicateItem renders one open candidate pair.
//
// The title is left EMPTY and the kind carries the record type, because this
// sentence has no server-side author: the product ships three languages, and a
// string composed here would reach a German reader in English. The approval
// summary below is different — the server composed it at staging time out of
// the proposal's own facts, and it is what every other decision surface already
// shows.
func duplicateItem(pair DuplicatePair) crmcontracts.AttentionItem {
	kind := pair.EntityType
	confidence := float32(pair.Confidence)
	return crmcontracts.AttentionItem{
		Id:         pair.ID.String(),
		Source:     crmcontracts.AttentionItemSource("dedupe_candidate"),
		Kind:       &kind,
		Confidence: &confidence,
		Actions:    []crmcontracts.AttentionItemActions{"merge"},
	}
}

// approvalItem renders one staged proposal.
//
// The summary is the server's own sentence for the proposal and is already
// sanitized at staging time, so it is what a reader sees. A proposal whose
// summary is empty falls back to its kind rather than rendering a blank card —
// an unnamed decision is still a decision somebody has to make.
func approvalItem(approval crmcontracts.Approval) crmcontracts.AttentionItem {
	kind := approval.Kind
	item := crmcontracts.AttentionItem{
		Id:      approval.Id.String(),
		Source:  crmcontracts.AttentionItemSource("approval"),
		Kind:    &kind,
		Title:   approval.Summary,
		Actions: []crmcontracts.AttentionItemActions{"decide"},
	}
	if approval.ExpiresAt != nil {
		item.DueAt = approval.ExpiresAt
	}
	if approval.TargetEntityType != nil && approval.TargetEntityId != nil {
		item.Subject = subjectOf(*approval.TargetEntityType, ids.UUID(*approval.TargetEntityId))
	}
	return item
}

// taskItem renders one open task, resolving overdue server-side so the queue,
// the record page and the badge cannot disagree about where today ends.
func taskItem(task Task, asOf time.Time) crmcontracts.AttentionItem {
	subject := task.Subject
	item := crmcontracts.AttentionItem{
		Id:      task.ID.String(),
		Source:  crmcontracts.AttentionItemSource("task"),
		Title:   &subject,
		Actions: []crmcontracts.AttentionItemActions{"complete", "snooze"},
	}
	if task.DueAt != nil {
		due := *task.DueAt
		item.DueAt = &due
		past := due.Before(asOf)
		item.Overdue = &past
	}
	return item
}

// receiptItem renders one thing the system did on its own.
//
// Its only verb is `open`. A receipt reports a finished act, and offering a
// decision on it would ask the reader to answer a question that has already
// been answered.
func receiptItem(receipt Receipt) crmcontracts.AttentionItem {
	kind := receipt.Kind
	occurred := receipt.OccurredAt
	summary := receipt.Summary
	return crmcontracts.AttentionItem{
		Id:         receipt.ID.String(),
		Source:     crmcontracts.AttentionItemSource("approval"),
		Kind:       &kind,
		Title:      &summary,
		OccurredAt: &occurred,
		Actions:    []crmcontracts.AttentionItemActions{"open"},
	}
}

// subjectOf names the record an item concerns, when the producer named one.
//
// The label is deliberately absent here: resolving a display name is a read of
// that record, and this feed does not hold the grants to make it. A client
// showing the subject asks the record's own endpoint, which answers under the
// reader's scope rather than this assembler's.
func subjectOf(entityType string, id ids.UUID) *crmcontracts.AttentionSubject {
	kind, ok := subjectKinds[entityType]
	if !ok {
		return nil
	}
	return &crmcontracts.AttentionSubject{Type: kind, Id: openapi_types.UUID(id)}
}

// subjectKinds maps a staged target's entity type onto the contract's subject
// enum. A type absent here yields no subject rather than a guess: a card that
// pointed a reader at the wrong record would be worse than one that pointed
// nowhere.
var subjectKinds = map[string]crmcontracts.AttentionSubjectType{
	"organization": "organization",
	"person":       "person",
	"deal":         "deal",
	"lead":         "lead",
	"activity":     "activity",
	"project":      "project",
}
