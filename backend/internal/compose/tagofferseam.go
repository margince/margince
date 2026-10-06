// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/collections"
)

// tagOfferSeam answers what accepting a proposed word would take, through the
// store apply_tag resolves names with, so an offer and the apply that follows
// agree on what "the workspace already has this word" means.
func tagOfferSeam(pool *pgxpool.Pool) agents.TagOfferFor {
	store := collections.NewStore(InstallationDB(pool))
	return func(ctx context.Context, name string) (*agents.TagOffer, error) {
		word := strings.TrimSpace(name)
		if word == "" {
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
