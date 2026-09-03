// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package aitest is the shared lane double for a site that asks through ai.Ask.
//
// It lives beside the dispatch it stands in for, because what it proves belongs
// to that dispatch and not to any one site: that the validator a site hands the
// lane is the site's OWN read of the reply. A double per package would be a
// dozen spellings of one behaviour — the same accident ai.Ask exists to undo,
// repeated in the tests that prove ai.Ask is used.
//
// A site's ordinary fake answers once and cannot re-ask, so under it the
// validator is never called and a site that passed a looser check than its own
// parse would look identical to one that passed the right thing.
package aitest

import (
	"context"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// ReAsking answers First, and when the site's validator refuses it, answers
// Second — which is what the §5.2 policy does with the refusal message in hand.
//
// Refused is the site's own refusal of First, and it is the field a test
// asserts on: a nil there means the site accepted a reply its answer path would
// have thrown away, which is the defect ai.Ask exists to prevent and the one an
// ordinary fake cannot see.
type ReAsking struct {
	First   string
	Second  string
	Refused error
	Asked   int
	Bare    int
}

// Complete is the unvalidated path. A site reaching it has lost the retry —
// Bare counts that rather than failing here, so a test can say which happened.
func (l *ReAsking) Complete(context.Context, model.Request) (model.Response, error) {
	l.Bare++
	return model.Response{Text: l.First}, nil
}

// CompleteValidated hands the site's validator its first answer and, when that
// is refused, answers again.
func (l *ReAsking) CompleteValidated(
	_ context.Context, _ model.Request, validate ai.Validator,
) (model.Response, error) {
	l.Asked++
	l.Refused = validate(l.First)
	if l.Refused == nil {
		return model.Response{Text: l.First}, nil
	}
	return model.Response{Text: l.Second}, nil
}
