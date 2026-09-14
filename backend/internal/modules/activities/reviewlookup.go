// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The seams a refused message crosses to reach its review.
//
// A refusal freezes a message into a held row and consent opens a review for
// it. Two questions cross back the other way: which review stands over this
// message, and — when the message settles — how to end it. Both live here
// because this module may not import consent, so compose binds them (ADR-0054).
//
// ReviewCloser is in scheduledsend.go beside the hold it answers. This file
// holds the READ, which is the half a surface needs: a rep looking at their
// scheduled list sees a message that stopped, and without the id on the row
// there is nothing on it that leads to the work that would unstop it.

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ReviewLookup answers which review, if any, stands over each of these held
// messages.
//
// WHY A SEAM. The review lives in consent and this module may not import a
// sibling, so compose binds it — the same shape ReviewCloser above has, and for
// the same reason.
//
// BATCHED, taking every id a list read returned rather than one at a time. A
// per-row call would put one query behind every message in a rep's scheduled
// list, and the list is exactly where this is read.
//
// A POINTER, NOT PERMISSION. What comes back is an id; reading the review
// itself is gated on its own terms, so a caller who is handed one here can
// still be refused when they open it.
//
// Nil is a composition with no review surface, and a row then carries no review
// id — which reads correctly as "no review stands over this", the same answer
// every unrefused message gives.
type ReviewLookup interface {
	LiveReviewsForIntents(ctx context.Context, intents []ids.UUID) (map[ids.UUID]ids.UUID, error)
}

// WithReviewLookup wires the consent-side read a held row uses to find the work
// that would unstop it.
func (s *Store) WithReviewLookup(lookup ReviewLookup) *Store {
	clone := *s
	clone.reviewLookup = lookup
	return &clone
}
