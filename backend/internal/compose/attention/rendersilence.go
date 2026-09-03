// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The two lanes that report a SILENCE, and what each one says it is worth.
//
// A deal nobody is moving and a person nobody is talking to. They rest on
// different records and warn about different things, but they render as one
// concept: neither is a deadline anybody agreed to, so neither carries a verb
// that decides, and both have to say how long the silence has run AND why that
// silence costs something. A card that could only say "quiet 63 days" left the
// reader to guess which of those it was.

import (
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

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
// It offers no verb that DECIDES. What to do about a quiet deal is a judgement,
// and a queue that answered it here would be deciding rather than warning. It
// does offer `open`, which decides nothing: naming a deal as drifting and then
// leaving the reader to go and find it by hand is a warning they cannot act on.
// The `deal` facts ride along so the card states value, stage and ownership
// without a second read per row.
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
		Deal:    dealFacts(deal),
		Actions: []crmcontracts.AttentionItemActions{actionOpen},
	}
	// The idle count travels TYPED, so the card can say the window the server
	// actually applied instead of implying one — and so `detail` stays the
	// supporting sentence it is everywhere else. It used to ride as the detail's
	// own decimal, which made one field mean a sentence on ten sources and an
	// integer on two, and any client rendering it faithfully printed a naked
	// "90" under the title.
	if deal.QuietDays > 0 {
		days := deal.QuietDays
		item.QuietDays = &days
	}
	if deal.ExpectedCloseDate != nil {
		due := *deal.ExpectedCloseDate
		item.DueAt = &due
	}
	return item
}

// lapsedItem renders one relationship this reader has stopped talking to.
//
// The contact's NAME travels as the title because a person's name is a
// sentence in every language, and the silence rides as a typed count, the way
// the risk card carries its idle days — so the client writes "quiet 63 days"
// and the server implies no window it did not apply.
//
// `occurred_at` is the last time they spoke. It is the fact the card dates
// itself from, and it lets the lane be read as a chronology rather than only
// as a list of numbers.
//
// It offers NO verb, exactly as the risk card does not. What to do about a
// lapsed relationship is a judgement about that person, and a queue that
// answered it here would be deciding rather than warning.
func lapsedItem(quiet QuietRelationship) crmcontracts.AttentionItem {
	name := quiet.Name
	days := quiet.QuietDays
	lastAt := quiet.LastAt
	funded := quiet.HasOpenDeal
	return crmcontracts.AttentionItem{
		Id:     quiet.PersonID.String(),
		Source: crmcontracts.AttentionItemSource("relationship_decay"),
		Title:  &name,
		// Typed, for the reason riskItem's is: `detail` is a sentence on every
		// other source, and the client writes "quiet N days" in the reader's
		// own language rather than rendering a decimal the server chose.
		QuietDays: &days,
		// What the relationship was worth before it lapsed. Typed for the same
		// reason: the client writes the phrase, and the queue READS the band —
		// a weak, unfunded silence is routine work and a strong or funded one
		// is not.
		Relationship: &crmcontracts.AttentionRelationshipFacts{
			Strength:    relationshipBand(quiet.Strength.Bucket),
			HasOpenDeal: &funded,
		},
		Subject:    subjectOf("person", quiet.PersonID),
		OccurredAt: &lastAt,
		Actions:    []crmcontracts.AttentionItemActions{},
	}
}

// relationshipBand carries the §4 bucket onto the wire, answering nothing for a
// word this contract does not declare.
//
// A mapping rather than a cast, because the two vocabularies are declared in
// different places: §4 owns the bands and the contract owns what a client is
// promised. A cast would widen the wire silently the day either grows a term,
// and the reader would get a band their client cannot translate. An unmapped
// bucket is ABSENT, which the client already draws the way it draws a lane that
// scored none.
func relationshipBand(bucket string) *crmcontracts.AttentionRelationshipFactsStrength {
	var band crmcontracts.AttentionRelationshipFactsStrength
	switch bucket {
	case relstrength.BucketNone:
		band = crmcontracts.AttentionRelationshipFactsStrengthNone
	case relstrength.BucketWeak:
		band = crmcontracts.AttentionRelationshipFactsStrengthWeak
	case relstrength.BucketModerate:
		band = crmcontracts.AttentionRelationshipFactsStrengthModerate
	case relstrength.BucketStrong:
		band = crmcontracts.AttentionRelationshipFactsStrengthStrong
	default:
		return nil
	}
	return &band
}

// dealFacts carries the at-risk deal's card facts onto the wire, or nothing:
// a facts object with every field empty says less than its absence.
func dealFacts(deal RiskyDeal) *crmcontracts.AttentionDealFacts {
	if deal.StageID == nil && deal.OwnerID == nil && deal.AmountMinor == nil && deal.Currency == nil {
		return nil
	}
	facts := &crmcontracts.AttentionDealFacts{
		AmountMinor: deal.AmountMinor,
		Currency:    deal.Currency,
	}
	if deal.StageID != nil {
		stage := openapi_types.UUID(*deal.StageID)
		facts.StageId = &stage
	}
	if deal.OwnerID != nil {
		owner := openapi_types.UUID(*deal.OwnerID)
		facts.OwnerId = &owner
	}
	return facts
}
