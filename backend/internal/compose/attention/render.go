// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"errors"
	"strconv"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/deadline"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// One item's shape, per producer.
//
// The rule every renderer here keeps: the title is what a person would say
// happened, and the identifiers stay in `id` and `subject` where a client uses
// them to navigate. A card that printed `organization_id` at a reader was
// showing them the plumbing and calling it information.

// duplicateItem renders one open candidate pair, with both records named.
//
// The title is left EMPTY and the kind carries the record type, because this
// sentence has no server-side author: the product ships three languages, and a
// string composed here would reach a German reader in English. The approval
// summary below is different — the server composed it at staging time out of
// the proposal's own facts, and it is what every other decision surface already
// shows.
//
// Naming the two sides is a READ of each record, so a side this reader may not
// see costs the item its pair — and an item with no pair offers no merge. The
// reader is told a duplicate is waiting and cannot act on it here, which is the
// honest answer: the alternative is a merge button over a record they cannot
// read.
func (s *Service) duplicateItem(
	ctx context.Context, pair DuplicatePair,
) (crmcontracts.AttentionItem, error) {
	kind := pair.EntityType
	confidence := float32(pair.Confidence)
	item := crmcontracts.AttentionItem{
		Id:         pair.ID.String(),
		Source:     crmcontracts.AttentionItemSource("dedupe_candidate"),
		Kind:       &kind,
		Confidence: &confidence,
		Actions:    []crmcontracts.AttentionItemActions{},
	}
	left, right, ok, err := s.faces(ctx, pair)
	if err != nil {
		return crmcontracts.AttentionItem{}, err
	}
	if !ok {
		return item, nil
	}
	item.Pair = &crmcontracts.AttentionPair{
		Left:     left,
		Right:    right,
		Evidence: evidenceRows(pair.Evidence),
	}
	item.Actions = []crmcontracts.AttentionItemActions{"merge"}
	return item, nil
}

// faces names both sides, or neither.
//
// A REFUSAL on one side is not an error to return: the rest of the day is still
// readable, and one record this caller may not see must not empty the whole
// lane. A refusal is `ErrPermissionDenied` (the object grant) or `ErrNotFound`
// (a row-scope miss, which answers not-found so existence stays hidden).
//
// Any OTHER error is returned. A database that will not answer is not a record
// the reader lacks the grants for, and rendering it as withheld would tell them
// a pair is hidden from their account when nothing of the kind is true — the
// same lie the lane assembler refuses to tell when a whole lane fails.
func (s *Service) faces(
	ctx context.Context, pair DuplicatePair,
) (crmcontracts.AttentionPairSide, crmcontracts.AttentionPairSide, bool, error) {
	left, ok, err := s.face(ctx, pair.EntityType, pair.LeftID)
	if err != nil || !ok {
		return crmcontracts.AttentionPairSide{}, crmcontracts.AttentionPairSide{}, false, err
	}
	right, ok, err := s.face(ctx, pair.EntityType, pair.RightID)
	if err != nil || !ok {
		return crmcontracts.AttentionPairSide{}, crmcontracts.AttentionPairSide{}, false, err
	}
	return left, right, true, nil
}

// face names one side: the record, whether the reader may see it, and whether
// the read itself failed.
func (s *Service) face(
	ctx context.Context, entityType string, id ids.UUID,
) (crmcontracts.AttentionPairSide, bool, error) {
	record, err := s.duplicates.Describe(ctx, entityType, id)
	switch {
	case errors.Is(err, apperrors.ErrPermissionDenied), errors.Is(err, apperrors.ErrNotFound):
		return crmcontracts.AttentionPairSide{}, false, nil
	case err != nil:
		return crmcontracts.AttentionPairSide{}, false, err
	default:
		return pairSide(id, record), true, nil
	}
}

func pairSide(id ids.UUID, face RecordFace) crmcontracts.AttentionPairSide {
	side := crmcontracts.AttentionPairSide{
		Id:           openapi_types.UUID(id),
		Label:        face.Label,
		CreatedAt:    face.CreatedAt,
		RelatedCount: face.RelatedCount,
	}
	if face.Detail != "" {
		side.Detail = &face.Detail
	}
	return side
}

// evidenceRows keeps the comparisons a client can name.
//
// A field or a verdict outside the contract's own enums is DROPPED rather than
// passed through: the client turns each key into a phrase in the reader's
// language, and a key it does not know would reach the screen as the database's
// own column name. That is the leak this rendering exists to close, so it is
// closed by the sender.
//
// Recognition asks the GENERATED enum whether it knows the value, rather than a
// map kept beside it. A map here would be a third copy of one vocabulary — the
// detector writes it, the contract publishes it, and a hand-maintained list in
// the middle can drift from both while every gate stays green.
// backend/dedupeevidencefields_test.go holds the detector's list against the
// contract in both directions; this reads the contract directly, so the two
// ends are all there is.
func evidenceRows(rows []FieldComparison) []crmcontracts.AttentionPairEvidence {
	out := make([]crmcontracts.AttentionPairEvidence, 0, len(rows))
	for _, row := range rows {
		field := crmcontracts.AttentionPairEvidenceField(row.Field)
		signal := crmcontracts.AttentionPairEvidenceSignal(row.Signal)
		if !field.Valid() || !signal.Valid() {
			continue
		}
		out = append(out, crmcontracts.AttentionPairEvidence{
			Field:      field,
			Signal:     signal,
			LeftValue:  row.Left,
			RightValue: row.Right,
		})
	}
	return out
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

// taskItem renders one open task.
//
// Overdue is resolved here rather than in the browser, through deadline.Passed
// — the one place that decides whether a due moment is behind now.
// Held by: TestOnlyOnePlaceDecidesWhetherSomethingIsLate
// (backend/overdueboundary_test.go).
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
		past := deadline.Passed(task.DueAt, asOf)
		item.Overdue = &past
	}
	return item
}

// briefItem renders one entry of the overnight brief's queue.
//
// No title, for the reason a duplicate pair carries none (see duplicateItem):
// the sentence would have to be composed here, and the product ships three
// languages. What the client gets is the deal as a typed subject and the rank,
// which is enough to draw the row in the reader's own language and to fetch the
// full card from the brief's own endpoint.
//
// The three verbs route to /brief/items/{itemId}/… — the same endpoints Home
// already calls, so this lane adds no second way to answer a brief item.
func briefItem(entry BriefEntry) crmcontracts.AttentionItem {
	rank := entry.Rank
	return crmcontracts.AttentionItem{
		Id:      entry.ID.String(),
		Source:  crmcontracts.AttentionItemSource("brief_item"),
		Rank:    &rank,
		Subject: subjectOf("deal", entry.DealID),
		Actions: []crmcontracts.AttentionItemActions{"act", "set_aside", "dismiss"},
	}
}

// commitmentItem renders one promise this rep made.
//
// It carries a title where a brief item and a duplicate pair do not, and the
// difference is who wrote the words. Those two would need a sentence composed
// here, in a product that ships three languages. A commitment already HAS its
// sentence — the claim body, in the language it was captured in — so sending it
// states what was promised rather than inventing a phrasing.
//
// The evidence rides in `detail` for the same reason the claim contract keeps
// `source_quote` beside `body`: a reader has to be able to check the promise
// against what was actually written, and a card showing only the paraphrase
// asks them to trust the extractor instead.
//
// Its only verb is `open`. Marking a promise kept is the claim's own endpoint's
// job, and this feed adds no authority the record does not already have.
func commitmentItem(promise Commitment, asOf time.Time) crmcontracts.AttentionItem {
	body := promise.Body
	quote := promise.Quote
	due := promise.DueAt
	past := deadline.Passed(&due, asOf)
	item := crmcontracts.AttentionItem{
		Id:      promise.ID.String(),
		Source:  crmcontracts.AttentionItemSource("conversation_claim"),
		Title:   &body,
		Detail:  &quote,
		Subject: subjectOf("person", promise.PersonID),
		DueAt:   &due,
		Overdue: &past,
		Actions: []crmcontracts.AttentionItemActions{"open"},
	}
	if promise.SourceLabel != "" {
		label := promise.SourceLabel
		item.Kind = &label
	}
	return item
}

// riskItem renders one deal the pipeline should worry about.
//
// The title is the deal's own name, which the reader recognises. The card's
// SENTENCE is the client's to write, because the two grounds read differently
// in every language and the server has none — so what travels is the deal, the
// idle days, and whether the close date has passed, and the client says it.
//
// `kind` carries the ground rather than a label: `quiet` when only the idle
// clock admitted it, `close_overdue` when the date has passed. A deal that is
// both is reported as overdue, because a date the customer agreed to outranks
// a silence nobody agreed to.
//
// Its only verb is `open`. What to DO about a quiet deal is a judgement, and a
// queue that offered to answer it here would be deciding rather than warning.
func riskItem(deal RiskyDeal) crmcontracts.AttentionItem {
	name := deal.Name
	ground := "quiet"
	if deal.CloseOverdue {
		ground = "close_overdue"
	}
	item := crmcontracts.AttentionItem{
		Id:      deal.DealID.String(),
		Source:  crmcontracts.AttentionItemSource("deal_at_risk"),
		Kind:    &ground,
		Title:   &name,
		Subject: subjectOf("deal", deal.DealID),
		Overdue: &deal.CloseOverdue,
		Actions: []crmcontracts.AttentionItemActions{"open"},
	}
	// The idle count rides as the detail's own number, so the card can say the
	// window the server actually applied instead of implying one.
	if deal.QuietDays > 0 {
		days := strconv.Itoa(deal.QuietDays)
		item.Detail = &days
	}
	if deal.ExpectedCloseDate != nil {
		due := *deal.ExpectedCloseDate
		item.DueAt = &due
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
