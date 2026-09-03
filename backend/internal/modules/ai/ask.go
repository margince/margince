// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// Completer is one model lane as a calling site sees it: somewhere to send a
// request and get one reply. A site declares its own one-method interface for
// injection; the same values satisfy this one.
//
// Named for what it does rather than for the lane, because Lane in this package
// is already the pricing vocabulary and two meanings of one word in one package
// is a reader's problem before it is a compiler's.
type Completer interface {
	Complete(ctx context.Context, req model.Request) (model.Response, error)
}

// validatedLane is a lane that can re-ask when the reply is refused. Not every
// lane can — the offline fake, the certification recorder and the test doubles
// cannot — so Ask asks rather than requires.
type validatedLane interface {
	CompleteValidated(ctx context.Context, req model.Request, validate Validator) (model.Response, error)
}

// Ask sends one request, re-asking when the reply is one the site would refuse.
//
// validate is the site's OWN read of the reply, not a looser shape check: a
// reply that is valid JSON and still names nobody is exactly what a retry can
// fix, and the refusal message is what tells the model why. A shape-only check
// would spend the attempt without naming the fault.
//
// This is the ONE place the "can this lane re-ask?" question is asked. It lives
// here, beside Validator and the §5.2 policy it dispatches to, rather than
// beside each site, because the sites that spelled it themselves all spelled it
// identically — and a tree that answers one question in thirty places is a tree
// where twenty-nine of them are one refactor away from disagreeing. A site that
// sends bare deliberately — a certification recorder measuring an unassisted
// model — calls Complete directly and says why, which is visible.
func Ask(ctx context.Context, lane Completer, req model.Request, validate Validator) (model.Response, error) {
	if structured, canReAsk := lane.(validatedLane); canReAsk {
		return structured.CompleteValidated(ctx, req, validate)
	}
	return lane.Complete(ctx, req)
}
