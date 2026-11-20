// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Telling the seats that may answer a legal review that one is waiting. Who they
// are and how they are told belong to identity, notices and approvals, which this
// module may not import, so compose injects the desk. Who among them may not
// answer is the review's own rule and is decided here.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ReviewRequest is one open request as the desk announces it. Seats are the
// reviewers to tell, already without the requester and the deal's owner.
type ReviewRequest struct {
	ReviewID ids.UUID
	DealID   ids.UUID
	DealName string
	Seats    []ids.UUID
}

// LegalReviewDesk reaches the seats that answer reviews. All three calls run in
// the caller's transaction, so the request and the word to reviewers commit
// together or not at all.
type LegalReviewDesk interface {
	// Reviewers lists the live seats whose roles let them read and answer reviews.
	Reviewers(ctx context.Context, tx pgx.Tx) ([]ids.UUID, error)
	// Raise puts the request in front of each seat: an inbox entry and the notice
	// that announces it. A second call for the same review and seat adds nothing.
	Raise(ctx context.Context, tx pgx.Tx, request ReviewRequest) error
	// Settle takes the entry and the notice back from every seat, once the review
	// is answered or withdrawn.
	Settle(ctx context.Context, tx pgx.Tx, reviewID ids.UUID) error
}

// WithLegalReviewDesk wires the desk a request and its settling need.
func (h Handlers) WithLegalReviewDesk(desk LegalReviewDesk) Handlers {
	wired := *h.store
	wired.reviewDesk = desk
	h.store = &wired
	return h
}

// raiseLegalReview tells every seat that may answer the review. The requester
// and the deal's owner are left out by the same rule that refuses their answer.
func (s *Store) raiseLegalReview(ctx context.Context, tx pgx.Tx, reviewID ids.UUID, dealID ids.DealID, requester ids.UUID) error {
	if s.reviewDesk == nil {
		return errors.New("deals: legal review needs its desk, and it is not wired")
	}
	head := reviewHead{dealID: dealID.UUID, requestedBy: &requester}
	var dealName string
	if err := tx.QueryRow(ctx, `SELECT name, owner_id FROM deal WHERE id = $1`, dealID).
		Scan(&dealName, &head.dealOwner); err != nil {
		return fmt.Errorf("read the deal under review: %w", err)
	}
	reviewers, err := s.reviewDesk.Reviewers(ctx, tx)
	if err != nil {
		return fmt.Errorf("list the seats that answer legal reviews: %w", err)
	}
	seats := make([]ids.UUID, 0, len(reviewers))
	for _, seat := range reviewers {
		if refuseOwnSide(head, seat) == nil {
			seats = append(seats, seat)
		}
	}
	return s.reviewDesk.Raise(ctx, tx, ReviewRequest{ReviewID: reviewID, DealID: dealID.UUID, DealName: dealName, Seats: seats})
}

// settleLegalReview withdraws the word to reviewers once the review is closed.
func (s *Store) settleLegalReview(ctx context.Context, tx pgx.Tx, reviewID ids.UUID) error {
	if s.reviewDesk == nil {
		return errors.New("deals: legal review needs its desk, and it is not wired")
	}
	return s.reviewDesk.Settle(ctx, tx, reviewID)
}
