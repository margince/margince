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
		free := intersect(permitted, desc.Free())
		if len(free) == 0 {
			// This connection buys nothing free, so an automatic run has
			// nothing to ask for. Dispatching it anyway sends a request with
			// no categories, which a provider answers with a refusal — and a
			// refusal flips the connection's status, breaking automatic
			// ingest for every later contact over a configuration choice.
			return nil, ErrNothingFreeToBuy
		}
		return free, nil
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
	asked := make(map[provider.Category]bool, len(requested))
	for _, c := range requested {
		if !allowed[c] {
			return nil, fmt.Errorf("%w: %q is not one this connection buys", ErrCategoryNotPermitted, c)
		}
		asked[c] = true
		out = append(out, c)
	}
	if err := requireCascadeTriggers(desc, asked); err != nil {
		return nil, err
	}
	return out, nil
}

// requireCascadeTriggers refuses a fallback asked for without the category it
// follows.
//
// A cascade fires only when its trigger comes back empty, so a request naming
// the fallback alone never issues it — and the platform prices it at nothing
// for exactly that reason, reserving no credits. The provider is not bound by
// our reasoning: Surfe's request still carries the email flag and charges for
// whatever it finds, so the run spends money against a hold of zero.
//
// Refused rather than silently widened. Adding the trigger would buy a
// category the caller did not ask for, and pricing the pair while sending one
// is the mismatch this exists to prevent.
func requireCascadeTriggers(desc provider.Descriptor, asked map[provider.Category]bool) error {
	for _, cascade := range desc.Cascades {
		if asked[cascade.Category] && !asked[cascade.After] {
			return fmt.Errorf("%w: %q is a fallback for %q and cannot be bought without it",
				ErrCategoryNotPermitted, cascade.Category, cascade.After)
		}
	}
	return nil
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

// ErrNothingFreeToBuy reports that an automatic trigger found no category it
// may spend on. A configuration state, not a fault: the event consumer
// swallows it exactly as it swallows a trigger the policy does not admit.
var ErrNothingFreeToBuy = errors.New("integrations: this connection buys nothing an automatic run may take")

// ErrCategoryNotPermitted reports that a run asked for a category the
// connection does not carry. A caller's mistake, not a provider condition: the
// HTTP surface answers 422, because retrying it unchanged buys nothing.
var ErrCategoryNotPermitted = errors.New("integrations: this connection does not buy that category")
