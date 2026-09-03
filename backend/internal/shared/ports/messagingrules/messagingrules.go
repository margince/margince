// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package messagingrules is the core-internal seam behind country messaging
// rules: the rule CONTRACT lives on the published surface
// backend/pkg/extension/messaging so extensions can declare it, and this
// package keeps the registry and re-exports the types as aliases so core call
// sites stay put.
//
// Core code never imports a pack and never contains a jurisdiction string
// (scripts/check-no-jurisdiction.sh). What the authorization engine asks this
// package is "what does the applicable jurisdiction require", and it gets data
// back — never a pack it calls into.
package messagingrules

import (
	"fmt"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	pub "github.com/margince/margince/backend/pkg/extension/messaging"
)

// The published types, aliased so a rule set declared by an extension and one
// registered by a core module are the same type.
type (
	// Rules is one jurisdiction's outbound-messaging rule set.
	Rules = pub.Rules
	// MarketingException is one route to advertising without consent.
	MarketingException = pub.MarketingException
	// Disclosure is one obligation a first message carries.
	Disclosure = pub.Disclosure
	// FrequencyCap bounds advertising to one address in a window.
	FrequencyCap = pub.FrequencyCap
	// ExceptionKind names a route to lawful advertising without consent.
	ExceptionKind = pub.ExceptionKind
	// DisclosureKind names something a first message must carry.
	DisclosureKind = pub.DisclosureKind
)

// The published constants, re-exported.
const (
	// ExistingCustomer is the sale-derived advertising exception.
	ExistingCustomer = pub.ExistingCustomer
	// ControllerIdentity is who is writing.
	ControllerIdentity = pub.ControllerIdentity
	// PrivacyContact is who answers about the data.
	PrivacyContact = pub.PrivacyContact
	// ObjectionRoute is how to say stop.
	ObjectionRoute = pub.ObjectionRoute
	// AdvertiserContact is the advertiser's own reachability.
	AdvertiserContact = pub.AdvertiserContact
)

var (
	mu    sync.RWMutex
	rules = map[jurisdiction.Code]Rules{}
)

// Register records one jurisdiction's rules. A duplicate or invalid set is a
// wiring defect and fails fast at boot, for the reason the jurisdiction
// registry does the same: two rule sets for one country are two answers to
// whether a message may go out, and whichever the engine happened to read
// would look like that country's law.
func Register(r Rules) {
	mu.Lock()
	defer mu.Unlock()
	if err := r.Validate(); err != nil {
		panic(fmt.Sprintf("messagingrules: %v", err))
	}
	if _, dup := rules[r.Jurisdiction]; dup {
		panic(fmt.Sprintf("messagingrules: rules for %q registered twice", r.Jurisdiction))
	}
	rules[r.Jurisdiction] = clone(r)
}

// clone deep-copies a rule set, so the registry and its callers never share
// backing arrays.
//
// Rules is a struct of slices and a pointer, so a plain copy aliases them. A
// caller that sorted, filtered or normalised the slice it got back — an
// ordinary thing to write — would rewrite the registered rules for every later
// send in the process, with no boot error and nothing failing. The sibling
// jurisdiction registry is safe from this by accident: it hands out an
// interface, so there is no reachable mutable state. This one has to say so.
func clone(r Rules) Rules {
	out := r
	out.MarketingExceptions = slices.Clone(r.MarketingExceptions)
	out.Disclosures = slices.Clone(r.Disclosures)
	if r.FrequencyCap != nil {
		copied := *r.FrequencyCap
		out.FrequencyCap = &copied
	}
	return out
}

// For returns the rules for a code; ok is false when the running binary was
// not compiled with them.
func For(code jurisdiction.Code) (Rules, bool) {
	mu.RLock()
	defer mu.RUnlock()
	r, ok := rules[code]
	if !ok {
		return Rules{}, false
	}
	return clone(r), true
}

// Strictest folds the applicable jurisdictions into the one rule set the
// engine applies, taking the STRICTEST answer for each obligation.
//
// Several jurisdictions can apply at once — the installation's own country and
// the recipient's — and the answer is not "pick one". A message lawful where
// the sender sits and unlawful where the recipient does is unlawful, so each
// obligation resolves independently to whichever position permits less:
// the shortest non-zero window, every declared disclosure, the tightest cap,
// and an exception only where EVERY applicable jurisdiction grants it.
//
// An unknown jurisdiction contributes nothing to fold, which is why the caller
// must treat "no rules at all" as the consent-only floor rather than as
// permission — Strictest cannot invent an obligation for a country it was
// never told about.
func Strictest(codes ...jurisdiction.Code) (Rules, []jurisdiction.Code, bool) {
	var out Rules
	var unknown []jurisdiction.Code
	found := false
	for _, code := range codes {
		r, ok := For(code)
		if !ok {
			unknown = append(unknown, code)
			continue
		}
		if !found {
			out, found = r, true
			continue
		}
		out = stricter(out, r)
	}
	return out, unknown, found
}

// stricter folds one rule set into another, obligation by obligation.
func stricter(a, b Rules) Rules {
	out := a
	// The fold is no longer any one jurisdiction's, and stamping it with one
	// country's code would misname the rules a decision was taken under.
	out.Jurisdiction = ""
	// A version is only meaningful for a single jurisdiction's set; a folded
	// one carries none, and the decision records the codes it folded instead.
	out.Version = 0
	out.ReplyWindow = shorterWindow(a.ReplyWindow, b.ReplyWindow)
	out.DealFollowUpWindow = shorterWindow(a.DealFollowUpWindow, b.DealFollowUpWindow)
	// Every disclosure either side requires. An obligation one country imposes
	// is not lifted by another that does not.
	out.Disclosures = append(append([]Disclosure{}, a.Disclosures...), b.Disclosures...)
	// A prefix either side requires. Two different prefixes cannot both be
	// satisfied by one subject line, so the first declared wins and the caller
	// is expected not to compose two prefixing jurisdictions — a case no
	// shipped pack creates today.
	if out.SubjectPrefix == "" {
		out.SubjectPrefix = b.SubjectPrefix
	}
	out.FrequencyCap = tighterCap(a.FrequencyCap, b.FrequencyCap)
	out.OptOutAcknowledgement = a.OptOutAcknowledgement || b.OptOutAcknowledgement
	// An exception only where BOTH grant it: a route to advertising without
	// consent that one jurisdiction does not recognise is not available in a
	// send both govern.
	out.MarketingExceptions = commonExceptions(a.MarketingExceptions, b.MarketingExceptions)
	return out
}

// shorterWindow picks the tighter of two windows, treating zero as "this
// jurisdiction says nothing" rather than as an instant expiry.
func shorterWindow(a, b time.Duration) time.Duration {
	switch {
	case a == 0:
		return b
	case b == 0:
		return a
	case b < a:
		return b
	}
	return a
}

// tighterCap picks the cap that permits fewer messages per unit of time. Nil
// means uncapped, so a nil on either side yields the other's cap.
func tighterCap(a, b *FrequencyCap) *FrequencyCap {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	}
	// A degenerate cap is the strictest thing there is, and it is handled
	// FIRST rather than left to the comparison. Validate refuses one on the
	// extension path, but Register is exported and a core-built value never
	// sees Validate — and a zero window divided by a zero count is NaN, which
	// loses every comparison and would hand back whichever cap happened to sit
	// on the left. That made the fold's answer depend on the order the caller
	// listed the countries in.
	if degenerate(a) {
		return a
	}
	if degenerate(b) {
		return b
	}
	// Cross-multiplied rather than divided: exact, and it cannot produce NaN.
	// Three per 24h is tighter than ten per 24h, and also tighter than four per
	// hour. int64 nanoseconds against a message count stays far from overflow
	// for any cap a statute could state.
	left := int64(b.Messages) * int64(a.Window)
	right := int64(a.Messages) * int64(b.Window)
	if left != right {
		if left < right {
			return b
		}
		return a
	}
	// Equal rates are not equal burst: {1, 1h} and {24, 24h} permit the same
	// average and a 24-message burst is not the same restriction as one an
	// hour. The smaller allowance wins.
	if b.Messages < a.Messages {
		return b
	}
	return a
}

// degenerate reports a cap that cannot bind as written.
func degenerate(c *FrequencyCap) bool { return c.Messages <= 0 || c.Window <= 0 }

// commonExceptions keeps only the exceptions BOTH sides grant, and a kept one
// carries the union of the conditions either side attaches.
func commonExceptions(a, b []MarketingException) []MarketingException {
	// OR-folded rather than last-write-wins. A set declaring one kind twice is
	// refused by Validate, but Register is exported and the fold must be total
	// over its own type: overwriting let a weak duplicate erase a strong
	// entry's conditions, and the fold then permitted MORE than the input it
	// was folding — the one thing a strictest-wins fold may never do.
	byKind := map[ExceptionKind]MarketingException{}
	for _, e := range a {
		byKind[e.Kind] = unionConditions(byKind[e.Kind], e)
	}
	var out []MarketingException
	for _, e := range b {
		mine, both := byKind[e.Kind]
		if !both {
			continue
		}
		// A condition either jurisdiction attaches is checked. Dropping one
		// because the other does not require it would apply an exception on
		// looser terms than one of the two countries allows.
		out = append(out, unionConditions(mine, e))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Kind < out[j].Kind })
	return out
}

// unionConditions keeps every condition either side attaches to one kind.
func unionConditions(a, b MarketingException) MarketingException {
	return MarketingException{
		Kind:                         b.Kind,
		RequiresSaleEvidence:         a.RequiresSaleEvidence || b.RequiresSaleEvidence,
		RequiresCollectionTimeOptOut: a.RequiresCollectionTimeOptOut || b.RequiresCollectionTimeOptOut,
		RequiresSimilarity:           a.RequiresSimilarity || b.RequiresSimilarity,
		RequiresNoObjection:          a.RequiresNoObjection || b.RequiresNoObjection,
	}
}
