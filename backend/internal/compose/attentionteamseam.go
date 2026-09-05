// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The readers behind the TEAM board, bound to the modules that own their rows.
//
// Their own file rather than beside the per-reader lanes, because they answer a
// different question: each is about somebody OTHER than the caller, counted
// under the caller's own visibility. The lane seam next door reads one person's
// day and never needs a roster.

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// attentionPromiseLoad counts each named person's commitments due, for the team
// board.
//
// ONE query over every owner rather than one per teammate. A board reaches a
// hundred people, and a hundred sequential round trips on a surface a lead
// opens every morning is a slow page for no reason — and each in its own
// transaction meant a person's records changing hands mid-loop could be counted
// twice or not at all.
//
// The store shares every arm of that count with the page a rep opens, so a
// lead's column and their colleague's own cards answer the same question.
type attentionPromiseLoad struct{ store *people.Store }

var _ attention.PromiseLoad = attentionPromiseLoad{}

func (p attentionPromiseLoad) DuePerOwner(
	ctx context.Context, owners []ids.UUID, by time.Time,
) (map[ids.UUID]int, error) {
	typed := make([]ids.UserID, 0, len(owners))
	for _, owner := range owners {
		typed = append(typed, ids.From[ids.UserKind](owner))
	}
	// Counted under the CALLER's own scopes, which is what the board promises:
	// this is how much of a teammate's load the reader can see, not how much of
	// it exists.
	due, err := p.store.CountOpenCommitmentsDueByOwner(ctx, typed, by)
	if err != nil {
		return nil, err
	}
	// A person the query did not name owes nothing. Said explicitly, because
	// the board draws a number for every member and an absent key would leave
	// their column reading zero for a reason nobody chose.
	for _, owner := range owners {
		if _, counted := due[owner]; !counted {
			due[owner] = 0
		}
	}
	return due, nil
}
