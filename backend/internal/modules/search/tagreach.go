// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// hitTypeTag is the `type` a tag hit carries on the wire, and the same word
// the tag branch declares in searchBranches. Named because the count path
// tests it twice and a typo in either would silently count nothing.
const hitTypeTag = "tag"

// TagReachCounter counts, inside the caller's own transaction, how many records
// carry each of these tags under that caller's row scope. It takes the whole
// page's tags at once: a per-hit counter cost a round trip per record type per
// hit, which a page of 200 turns into hundreds.
//
// A tag missing from the returned map is one nothing visible carries.
type TagReachCounter func(ctx context.Context, tx pgx.Tx, tagIDs []ids.TagID) (map[ids.TagID]int, error)

// countTagReach fills CarriedBy on the tag hits of one page.
//
// It runs in the ranking query's transaction so the counts are taken under the
// same connection and the same actor, not because that freezes them: the
// handle runs READ COMMITTED, so each count still sees its own committed
// snapshot. A number one write out of date is the right trade here — this is
// a hint about whether a word is worth opening, and the tag page behind it
// re-counts on arrival.
//
// A counting failure fails the search rather than answering with a silent
// gap. The counter runs the caller's own row scope, so an error here is that
// scope refusing to render — and a page that quietly drops the number in that
// case would show a searcher a smaller world without saying so.
func (s *Store) countTagReach(ctx context.Context, tx pgx.Tx, hits []Hit) error {
	if s.carriedBy == nil {
		return nil
	}
	var tagIDs []ids.TagID
	for i := range hits {
		if hits[i].Type == hitTypeTag {
			tagIDs = append(tagIDs, ids.From[ids.TagKind](hits[i].ID))
		}
	}
	if len(tagIDs) == 0 {
		return nil
	}
	counts, err := s.carriedBy(ctx, tx, tagIDs)
	if err != nil {
		return fmt.Errorf("search: counting what this page's tags are on: %w", err)
	}
	for i := range hits {
		if hits[i].Type != hitTypeTag {
			continue
		}
		// Absent from the map means nothing VISIBLE carries the word, which is
		// a count of zero rather than a count nobody took — the counter ran and
		// this tag matched no row the caller may see.
		n := counts[ids.From[ids.TagKind](hits[i].ID)]
		hits[i].CarriedBy = &n
	}
	return nil
}
