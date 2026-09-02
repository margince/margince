// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcore

// The other half of the correct-and-retry loop: CorrectOnce re-asks a draft
// that is well-formed and badly worded, and this re-asks one that is malformed.
//
// They are separate because they correct different things and only one of them
// can be spelled generically. A phrasing finding is this package's own
// judgement, so CorrectOnce composes the correction itself; a refused reply is
// the SURFACE's reading of its own schema, so the surface supplies it and the
// model lane feeds it back.

import (
	"context"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// Completer is the model lane every drafting surface holds.
type Completer interface {
	Complete(ctx context.Context, req model.Request) (model.Response, error)
}

// validatedCompleter is the same lane when it can re-ask — routerBrain in
// production, and not every test fake. Asserted for rather than required so a
// surface stays testable against a plain lane.
type validatedCompleter interface {
	CompleteValidated(ctx context.Context, req model.Request, validate ai.Validator) (model.Response, error)
}

// CompleteChecked sends one drafting request, giving the model one chance to
// fix a reply the surface's own reading refuses.
//
// It exists because the alternative is silent: a surface that parses a reply,
// refuses it and returns the error degrades to its deterministic floor without
// ever telling the model what was wrong. Every obedient model then fails the
// same way on every request, the reader gets the template, and nothing is red.
// Two drafting surfaces shipped exactly that, and the reply surface did not,
// because it happened to reach for the validated lane and they did not.
//
// read is the surface's reading of its own reply, not a schema check: it must
// be the SAME function the caller will parse with, or the lane accepts a reply
// the caller then refuses and the re-ask was spent on the wrong question.
func CompleteChecked(
	ctx context.Context, lane Completer, req model.Request, read ai.Validator,
) (model.Response, error) {
	if validated, ok := lane.(validatedCompleter); ok {
		return validated.CompleteValidated(ctx, req, read)
	}
	return lane.Complete(ctx, req)
}
