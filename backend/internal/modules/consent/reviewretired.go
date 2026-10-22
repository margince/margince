// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The two review endpoints that outlived the ask they served.
//
// A rep no longer hands a refusal to a colleague. A rep overrules a machine
// refusal with a reason of their own (POST /contacts/{id}/consent/allow), and an
// admin may direct a send from the review. The ask and the deciders' queue stay
// on the contract, deprecated, until outside clients have had time to move.

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RequestDecision refuses: asking a colleague to decide a refused send is not
// offered any more. The refusal names the moves that are.
func (s *Store) RequestDecision(ctx context.Context) error {
	if err := auth.RequireHuman(ctx); err != nil {
		return err
	}
	return &ReviewNotRoutableError{Why: askingRetired}
}

// askingRetired is what a caller of the retired ask is told instead.
const askingRetired = "asking a colleague to decide a refused send is no longer offered; " +
	"overrule the refusal with your reason (POST /contacts/{id}/consent/allow), " +
	"or ask an admin to send it from the review"

// ReviewNotRoutableError refuses a route that would put an unanswerable
// question in front of somebody.
type ReviewNotRoutableError struct {
	Why string
}

func (e *ReviewNotRoutableError) Error() string { return e.Why }

// FieldFault carries the refusal to every surface, not only HTTP. It names the
// review because that is what the caller would look at.
func (e *ReviewNotRoutableError) FieldFault() (field, code, message string) {
	return fieldReviewID, "review_not_routable", e.Why
}

// MayReadDecidersQueue answers whether this caller may read the deciders'
// queue. The queue holds no rows any more, so the answer is all a caller gets.
//
// The door is the one the queue had, so a caller it refused before is still
// refused.
func (s *Store) MayReadDecidersQueue(ctx context.Context) error {
	// A human at the keyboard: auth.RequireHuman admits connectors, which run
	// with the granting human's grants.
	if err := requireAHumanAtTheKeyboard(ctx); err != nil {
		return err
	}
	return auth.Require(ctx, entityCommunicationException, principal.ActionRead)
}
