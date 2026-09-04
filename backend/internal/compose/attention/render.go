// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"errors"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/deadline"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// One item's shape, per producer.
//
// The rule every renderer here keeps: the title is what a person would say
// happened, and the identifiers stay in `id` and `subject` where a client uses
// them to navigate. A card that printed `organization_id` at a reader was
// showing them the plumbing and calling it information.

// actionOpen sends the reader to the record named in the item's `subject`, so
// only a card carrying a subject that IS a record may offer it —
// lanewiring_test.go refuses the rest.
const actionOpen crmcontracts.AttentionItemActions = "open"

// actionDismiss puts a lapsed contact aside for a while.
//
// Offered ONLY where a dismissal endpoint takes the row's own id, which today
// is the relationship-decay lane: its rows carry the person's id, and
// /people/{id}/nudge-dismissal is keyed on exactly that. A verb on a row whose
// id the endpoint cannot take would be a control that 404s.
//
// A reader's own judgement rather than a change to the record — the contact is
// no less quiet for being set aside — which is why the dismissal is per-reader
// and expires rather than resolving anything.
const actionDismiss crmcontracts.AttentionItemActions = "dismiss"

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
//
// The verb needs MORE than that read, and this is where the two questions part.
// Settling a pair archives one record and rewrites the other, so it is offered
// only to a reader who could change BOTH — the records' owner, or a seat that
// writes the whole workspace. Everyone else sees the pair and no verb, because
// they are the ones who cannot act on it. Offered on visibility alone, the
// button refused every press on a pair whose two records had different owners,
// and the surface then advised trying again.
func (s *Service) duplicateItem(
	pair DuplicatePair, known pairFaces, decidable pairAuthority, settleable map[ids.UUID]bool,
) crmcontracts.AttentionItem {
	kind := pair.EntityType
	confidence := float32(pair.Confidence)
	item := crmcontracts.AttentionItem{
		Id:         pair.ID.String(),
		Source:     crmcontracts.AttentionItemSource("dedupe_candidate"),
		Kind:       &kind,
		Confidence: &confidence,
		Actions:    []crmcontracts.AttentionItemActions{},
	}
	left, leftKnown := known.side(pair.EntityType, pair.LeftID)
	right, rightKnown := known.side(pair.EntityType, pair.RightID)
	if !leftKnown || !rightKnown {
		return item
	}
	item.Pair = &crmcontracts.AttentionPair{
		Left:     left,
		Right:    right,
		Evidence: evidenceRows(pair.Evidence),
	}
	// Both questions, and they are different. Authority asks whether this
	// reader could change these records; settleable asks whether the merge
	// would be accepted from anyone. A button offered on either alone is one
	// that refuses after the press.
	if decidable.both(pair.EntityType, pair.LeftID, pair.RightID) && settleable[pair.ID] {
		item.Actions = []crmcontracts.AttentionItemActions{"merge"}
	}
	return item
}

// pairFaces holds the records a page names, keyed by entity type then by id. An
// id with no entry is one this reader may not see.
type pairFaces map[string]map[ids.UUID]RecordFace

// pairAuthority holds which records the reader could change, keyed the same way
// pairFaces is. An id with no entry is one they could not.
type pairAuthority map[string]map[ids.UUID]bool

// both is the pair's own question: a merge touches two records, so authority
// over one of them is not authority to decide it.
func (a pairAuthority) both(entityType string, left, right ids.UUID) bool {
	rows := a[entityType]
	return rows[left] && rows[right]
}

func (f pairFaces) side(entityType string, id ids.UUID) (crmcontracts.AttentionPairSide, bool) {
	face, ok := f[entityType][id]
	if !ok {
		return crmcontracts.AttentionPairSide{}, false
	}
	return pairSide(id, face), true
}

// namePairs reads every record the page will name, one query per entity type
// rather than one per record.
//
// A REFUSAL is not an error to return: the rest of the day is still readable,
// and records this caller may not see must not empty the whole lane. A refusal
// is `ErrPermissionDenied` (the object grant, refused whole) or `ErrNotFound`
// (a row-scope miss, which the batched read reports as an absence — existence
// stays hidden either way).
//
// Any OTHER error is returned. A database that will not answer is not a record
// the reader lacks the grants for, and rendering it as withheld would tell them
// a pair is hidden from their account when nothing of the kind is true — the
// same lie the lane assembler refuses to tell when a whole lane fails.
func (s *Service) namePairs(ctx context.Context, pairs []DuplicatePair) (pairFaces, error) {
	wanted := map[string][]ids.UUID{}
	for _, pair := range pairs {
		wanted[pair.EntityType] = append(wanted[pair.EntityType], pair.LeftID, pair.RightID)
	}
	named := make(pairFaces, len(wanted))
	for entityType, rowIDs := range wanted {
		faces, err := s.duplicates.DescribeMany(ctx, entityType, rowIDs)
		switch {
		case errors.Is(err, apperrors.ErrPermissionDenied), errors.Is(err, apperrors.ErrNotFound):
			continue
		case err != nil:
			return nil, err
		}
		named[entityType] = faces
	}
	return named, nil
}

// decidablePairs asks which of the page's records the reader could change, one
// query per entity type, exactly as namePairs asks who they may see.
//
// A refusal is treated the same way and for the same reason: it means "no verb
// on these", which is a complete answer, and letting it empty the whole lane
// would cost the reader pairs they are entitled to look at. Any other error is
// returned, because a database that will not answer is not a permission
// verdict.
func (s *Service) decidablePairs(ctx context.Context, pairs []DuplicatePair) (pairAuthority, error) {
	wanted := map[string][]ids.UUID{}
	for _, pair := range pairs {
		wanted[pair.EntityType] = append(wanted[pair.EntityType], pair.LeftID, pair.RightID)
	}
	authority := make(pairAuthority, len(wanted))
	for entityType, rowIDs := range wanted {
		writable, err := s.duplicates.DecidableSubset(ctx, entityType, rowIDs)
		switch {
		case errors.Is(err, apperrors.ErrPermissionDenied), errors.Is(err, apperrors.ErrNotFound):
			continue
		case err != nil:
			return nil, err
		}
		authority[entityType] = writable
	}
	return authority, nil
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
// backend/gates/dedupeevidencefields_test.go holds the detector's list against the
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
func approvalItem(approval crmcontracts.Approval, machine MachineSender) crmcontracts.AttentionItem {
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
	if staged, ok := stagedFacts(approval, machine); ok {
		item.Staged = &staged
	}
	return item
}

// stagedFacts says what a routine contact decision is ABOUT, for the group it
// will join.
//
// Read from the payload the verdict engine staged — the address it is asking
// about, and whether that address's domain already names a company here. Both
// are facts the engine had; re-deriving them in the queue would be a second
// opinion, and one decision would land in two groups across two reads.
//
// TYPED on the item rather than written into `detail` as marker words. They were
// words because `detail` was the only line on the wire and this was all they
// needed to carry — but that made a supporting line undrawable on this source,
// and any client rendering it faithfully showed a rep "machine_sender".
func stagedFacts(
	approval crmcontracts.Approval, machine MachineSender,
) (crmcontracts.AttentionStagedFacts, bool) {
	if approval.Kind != "capture_counterparty" || approval.ProposedChange == nil {
		return crmcontracts.AttentionStagedFacts{}, false
	}
	change := *approval.ProposedChange
	facts := crmcontracts.AttentionStagedFacts{}
	if address, ok := change["email"].(string); ok && machine != nil && machine(address) {
		sender := true
		facts.MachineSender = &sender
	}
	// KnownCompany is left UNSET here, and the reason is worth stating: the
	// only company-ish field the staged payload carries is `display_name`,
	// which capture labels "untrusted header text — for display, never
	// matching" (modules/capture/pending.go). A sender types it, so
	// `Alice <alice@gmail.com>` would have read as a company we know.
	//
	// A real match needs a lookup against the organizations this workspace has,
	// which is a read this assembler does not make. Until it does, a contact
	// question is either from a machine or is the honest remainder.
	return facts, true
}

// taskItem renders one open task.
//
// Overdue is resolved here rather than in the browser, through deadline.Passed
// — the one place that decides whether a due moment is behind now.
// Held by: TestOnlyOnePlaceDecidesWhetherSomethingIsLate
// (backend/gates/overdueboundary_test.go).
func taskItem(task Task, asOf time.Time) crmcontracts.AttentionItem {
	subject := task.Subject
	item := crmcontracts.AttentionItem{
		Id:      task.ID.String(),
		Source:  crmcontracts.AttentionItemSource("task"),
		Title:   &subject,
		Subject: subjectOf(task.LinkType, task.LinkID),
		Actions: []crmcontracts.AttentionItemActions{"complete", "snooze"},
	}
	if task.DueAt != nil {
		due := *task.DueAt
		item.DueAt = &due
		past := deadline.Passed(task.DueAt, asOf)
		item.Overdue = &past
	}
	// Who holds it. Absent means nobody has taken it, which the unassigned
	// scope exists to surface and which the row could not say before.
	if task.AssigneeID != nil {
		assignee := openapi_types.UUID(*task.AssigneeID)
		item.AssigneeId = &assignee
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
// Marking a promise kept is the claim's own endpoint's job, and this feed adds
// no authority the record does not already have. What it does offer is `open`:
// the person the promise was made to is named on the card, and a reader who
// cannot reach them has been told about a debt and denied the way to pay it.
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
		Actions: []crmcontracts.AttentionItemActions{actionOpen},
	}
	if promise.SourceLabel != "" {
		label := promise.SourceLabel
		item.Kind = &label
	}
	return item
}

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
	return crmcontracts.AttentionItem{
		Id:         receipt.ID.String(),
		Source:     crmcontracts.AttentionItemSource("approval"),
		Kind:       &kind,
		Title:      &summary,
		Subject:    subject,
		OccurredAt: &occurred,
		Actions:    actions,
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

// openableSubject reports whether a subject names a record with a page of its
// own — the only kind `open` may be offered on. An activity is a timeline
// entry: naming it on the card is honest, promising navigation to it is not,
// and the client's router answers exactly this set.
func openableSubject(subject *crmcontracts.AttentionSubject) bool {
	if subject == nil {
		return false
	}
	switch subject.Type {
	case "organization", "person", "deal", "lead", "project":
		return true
	}
	return false
}
