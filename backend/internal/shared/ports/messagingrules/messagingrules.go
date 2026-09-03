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
	rules[r.Jurisdiction] = r
}

// For returns the rules for a code; ok is false when the running binary was
// not compiled with them.
func For(code jurisdiction.Code) (Rules, bool) {
	mu.RLock()
	defer mu.RUnlock()
	r, ok := rules[code]
	return r, ok
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
func Strictest(codes ...jurisdiction.Code) (Rules, bool) {
	var out Rules
	found := false
	for _, code := range codes {
		r, ok := For(code)
		if !ok {
			continue
		}
		if !found {
			out, found = r, true
			continue
		}
		out = stricter(out, r)
	}
	return out, found
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
	// Compare rates rather than raw counts: three per 24h is tighter than ten
	// per 24h, and also tighter than four per hour.
	if float64(b.Messages)/b.Window.Seconds() < float64(a.Messages)/a.Window.Seconds() {
		return b
	}
	return a
}

// commonExceptions keeps only the exceptions BOTH sides grant, and a kept one
// carries the union of the conditions either side attaches.
func commonExceptions(a, b []MarketingException) []MarketingException {
	byKind := map[ExceptionKind]MarketingException{}
	for _, e := range a {
		byKind[e.Kind] = e
	}
	var out []MarketingException
	for _, e := range b {
		mine, both := byKind[e.Kind]
		if !both {
			continue
		}
		out = append(out, MarketingException{
			Kind: e.Kind,
			// A condition either jurisdiction attaches is checked. Dropping one
			// because the other does not require it would apply an exception on
			// looser terms than one of the two countries allows.
			RequiresSaleEvidence:         mine.RequiresSaleEvidence || e.RequiresSaleEvidence,
			RequiresCollectionTimeOptOut: mine.RequiresCollectionTimeOptOut || e.RequiresCollectionTimeOptOut,
			RequiresSimilarity:           mine.RequiresSimilarity || e.RequiresSimilarity,
			RequiresNoObjection:          mine.RequiresNoObjection || e.RequiresNoObjection,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Kind < out[j].Kind })
	return out
}
