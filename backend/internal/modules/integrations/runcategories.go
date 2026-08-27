// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package integrations

// What one run asks the provider for, and the two rules that bound it.
//
// An automatic run spends nothing: nobody weighed the purchase, so it takes
// only the categories the provider gives away, which is what lets enrichment
// run on every arrival without anybody deciding it was worth the money. A
// human pressing a button may buy a priced category for one named person, but
// never one the connection does not carry.

import (
	"errors"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// runCategories is what THIS run asks for: the connection's own selection, or
// the narrower set the caller named.
//
// A caller may only narrow. The connection is the ceiling an admin set, and a
// category outside it is refused rather than trimmed — buying less than was
// asked for while answering as though nothing was wrong is the failure a rep
// cannot see, and spending on a category an admin switched off is the one they
// must not be able to cause.
func runCategories(desc provider.Descriptor, conn admittedConnection, in provider.QueueInput) ([]provider.Category, error) {
	permitted := categoriesFrom(conn.categories)
	if in.Trigger.Automatic() {
		// Nobody weighed THIS purchase. An automatic run takes only what the
		// provider gives away, so enrichment can run on every arrival without
		// anybody deciding it was worth the money — a priced category is
		// bought by a human pressing a button for one named person.
		return intersect(permitted, desc.Free()), nil
	}
	requested := in.Categories
	if len(requested) == 0 {
		return permitted, nil
	}
	allowed := make(map[provider.Category]bool, len(permitted))
	for _, c := range permitted {
		allowed[c] = true
	}
	out := make([]provider.Category, 0, len(requested))
	for _, c := range requested {
		if !allowed[c] {
			return nil, fmt.Errorf("%w: %q is not one this connection buys", ErrCategoryNotPermitted, c)
		}
		out = append(out, c)
	}
	return out, nil
}

// intersect keeps the order of the first argument, so what a run asks for
// reads in the connection's own order rather than the descriptor's.
func intersect(permitted, allowed []provider.Category) []provider.Category {
	keep := make(map[provider.Category]bool, len(allowed))
	for _, c := range allowed {
		keep[c] = true
	}
	out := []provider.Category{}
	for _, c := range permitted {
		if keep[c] {
			out = append(out, c)
		}
	}
	return out
}

// ErrCategoryNotPermitted reports that a run asked for a category the
// connection does not carry. A caller's mistake, not a provider condition: the
// HTTP surface answers 422, because retrying it unchanged buys nothing.
var ErrCategoryNotPermitted = errors.New("integrations: this connection does not buy that category")
