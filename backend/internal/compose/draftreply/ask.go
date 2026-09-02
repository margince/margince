// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftreply

import (
	"context"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// Lane is the model lane a drafting site sends through. A site declares its own
// one-method interface for injection; this one is satisfied by the same values.
type Lane interface {
	Complete(ctx context.Context, req model.Request) (model.Response, error)
}

// validatedLane is a lane that can re-ask when the reply is refused. Not every
// lane can — the offline fake and the test doubles cannot — so the dispatch
// below asks rather than requires.
type validatedLane interface {
	CompleteValidated(ctx context.Context, req model.Request, validate ai.Validator) (model.Response, error)
}

// Ask sends the request, re-asking when the reply is one the site would refuse.
//
// validate is the site's OWN read of the reply, not a looser shape check: a
// reply that is valid JSON and still names nobody is exactly what a retry can
// fix, and the refusal message is what tells the model why. A shape-only check
// would spend the attempt without naming the fault.
//
// This is the dispatch, and it is here rather than beside each site because
// both wrote it identically — which is the same accident that put two readers
// on one reply and let one defect become two.
func Ask(ctx context.Context, lane Lane, req model.Request, validate ai.Validator) (model.Response, error) {
	if structured, ok := lane.(validatedLane); ok {
		return structured.CompleteValidated(ctx, req, validate)
	}
	return lane.Complete(ctx, req)
}
