// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// A create can carry an offer of a tag word: the user said where they met the
// contact, the assistant passes that word, and the answer says what accepting it
// would take. Nothing is applied here. The user's yes is a click on the card, or
// a sentence the assistant then acts on with apply_tag.

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// TagOffer is a proposed tag word and what accepting it takes.
type TagOffer struct {
	// Name is the word as the caller proposed it.
	Name string `json:"name"`
	// TagID is the live tag of that name, absent when the workspace has none.
	TagID *ids.UUID `json:"tag_id,omitempty"`
	// Exists says a live tag of that name is in the vocabulary.
	Exists bool `json:"exists"`
	// MayCreate says this caller could coin the word when it does not exist, by
	// the grant the vocabulary requires, so the card never offers a refusal.
	MayCreate bool `json:"may_create"`
}

// TagOfferFor answers what accepting one proposed word would take, or nil when
// the word cannot be offered: it is blank, or a retired word holds the name.
type TagOfferFor func(ctx context.Context, name string) (*TagOffer, error)

// offerTag turns the proposed word into an offer. The record is already created
// by the time this runs, so a failure is carried as a warning and never fails
// the call, for the reason reportDuplicates gives.
func (t createRecord) offerTag(ctx context.Context, word string) *TagOffer {
	if word == "" || t.tagOffer == nil {
		return nil
	}
	offer, err := t.tagOffer(ctx, word)
	if err != nil {
		noteWarning(ctx, CodeTagOfferUnavailable,
			"The record was created. Whether the tag word "+word+" could be offered could not be checked, "+
				"so say so rather than claiming it was offered.")
		return nil
	}
	return offer
}

// CodeTagOfferUnavailable is the warning a create carries when the tag word it
// was given could not be checked.
const CodeTagOfferUnavailable = "tag_offer_unavailable"
