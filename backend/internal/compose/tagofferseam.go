// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/platform/database"
)

// maxTagNameBytes is create_tag's own bound on a word, so an offer never proposes
// a name the coinage would refuse.
const maxTagNameBytes = 64

// tagOfferSeam answers what accepting a proposed word would take, through the
// store apply_tag resolves names with, so an offer and the apply that follows
// agree on what "the workspace already has this word" means.
func tagOfferSeam(db *database.DB) agents.TagOfferFor {
	store := collections.NewStore(db)
	taggable := tagAdapter{store: store}.TaggableTypes()
	return func(ctx context.Context, recordType, name string) (*agents.TagOffer, error) {
		word := strings.TrimSpace(name)
		if word == "" || len(word) > maxTagNameBytes || !slices.Contains(taggable, recordType) {
			return nil, nil
		}
		id, state, err := store.LookupTagName(ctx, word)
		if err != nil {
			return nil, err
		}
		if state.Archived() {
			return nil, nil
		}
		offer := &agents.TagOffer{Name: word, Exists: state.Live(), MayCreate: store.MayCreateTag(ctx)}
		if state.Live() {
			offer.TagID = &id
		}
		return offer, nil
	}
}
