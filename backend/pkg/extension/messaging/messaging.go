// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package messaging carries the outbound-messaging contract of the published
// extension surface: a pack supplies country-specific RULES the core
// authorization engine consults — it is never an actor and never decides a
// send.
//
// The split is the same one the jurisdiction packs draw. A rule here is DATA:
// how long a reply stays a reply, whether the jurisdiction has an
// existing-customer exception and what that exception requires, what a first
// message must disclose, whether advertising carries a subject prefix, and how
// many advertising messages one address may receive in a window. The engine
// reads the data and answers; nothing in a pack is called back into.
//
// That matters more here than for retention. A pack that could decide a send
// would be country-specific code on the path of every outbound message, and the
// one thing this product cannot afford is two answers to "may we write to this
// person". So a pack states its jurisdiction's rules and the engine applies
// them, in one place, the same way for every country.
//
// These types are frozen published API from their first external consumer; they
// evolve additively or through versioned successors, never in place (EXT-P3).
//
//margince:extension-surface
package messaging

import (
	"fmt"
	"time"

	"github.com/margince/margince/backend/pkg/extension/jurisdiction"
)

// Rules is one jurisdiction's outbound-messaging rule set.
//
// Every field is optional in the sense that the zero value is a legal, STRICT
// answer: no exception, no disclosure obligation, no prefix, no cap, and the
// core's own default windows. A pack that declares nothing therefore permits
// nothing extra, which is the direction a missing declaration must fail in.
type Rules struct {
	// Jurisdiction is whose rules these are, lower-case ISO 3166-1 alpha-2.
	Jurisdiction jurisdiction.Code

	// Version is the rule set's own version, stamped onto every decision taken
	// under it. A decision a subject later asks about must be readable against
	// the rules that were live when it was taken, and those rules change — so
	// the number is recorded rather than re-derived from whatever the code says
	// today.
	Version int

	// ReplyWindow is how long an inbound message keeps making a reply a reply
	// when there is no thread to continue. Zero means the core default.
	//
	// It bounds an UNPROMPTED follow-up, never a same-thread reply: the subject
	// wrote to us and did not withdraw, and a rep answering a three-month-old
	// thread is doing the ordinary thing. A pack that shortened replies would
	// be refusing correspondence rather than restricting advertising.
	ReplyWindow time.Duration

	// DealFollowUpWindow is how long a live opportunity keeps supporting an
	// unprompted follow-up. Zero means the core default.
	DealFollowUpWindow time.Duration

	// MarketingExceptions are the routes to lawful advertising WITHOUT a
	// recorded consent. Empty means there are none, which is the strict
	// position and the right default: a jurisdiction whose pack forgets to
	// declare one requires consent rather than silently permitting a send.
	MarketingExceptions []MarketingException

	// Disclosures are what a first message to somebody must carry.
	Disclosures []Disclosure

	// SubjectPrefix is prepended to an advertising message's subject, exactly
	// once. Empty means none.
	SubjectPrefix string

	// FrequencyCap bounds advertising to one address in a window. Nil means
	// uncapped by this jurisdiction.
	FrequencyCap *FrequencyCap

	// OptOutAcknowledgement records whether an opt-out is owed a confirming
	// message. False means none is sent, which is the default.
	OptOutAcknowledgement bool
}

// ExceptionKind names a route to lawful advertising without consent. Closed:
// the engine implements each kind, so a pack naming one nothing implements
// would be an exception that looks declared and never applies.
type ExceptionKind string

// ExistingCustomer is the sale-derived exception several jurisdictions grant
// for advertising similar goods to an existing customer (UWG §7(3) in Germany).
// Its four conditions are declared on the exception rather than assumed,
// because they differ by country and the engine checks each one it is told to.
const ExistingCustomer ExceptionKind = "existing_customer"

// Validate enforces membership in the closed set.
func (k ExceptionKind) Validate() error {
	if k == ExistingCustomer {
		return nil
	}
	return fmt.Errorf("marketing exception %q is not one the engine implements", string(k))
}

// MarketingException is one route to advertising without a recorded consent,
// and the conditions that route requires.
//
// The conditions are stated positively and each defaults to FALSE, so a pack
// that declares an exception without naming its conditions gets an exception
// the engine checks nothing for — which is why the core refuses one at boot
// (Validate below). An exception with no conditions is not a lighter rule, it
// is an unguarded one.
type MarketingException struct {
	Kind ExceptionKind

	// RequiresSaleEvidence: there must be a recorded sale to this person.
	RequiresSaleEvidence bool

	// RequiresCollectionTimeOptOut: the address must have been collected with
	// a notice offering a free, easy objection at the time it was taken.
	RequiresCollectionTimeOptOut bool

	// RequiresSimilarity: the advertised goods must be similar to what was
	// bought. Checked per message, not once per person: a customer who bought
	// one product has not opened the door to every catalogue the seller has.
	RequiresSimilarity bool

	// RequiresNoObjection: no standing objection. Always true in practice —
	// an Art. 21 objection is absolute — and stated here so a pack cannot
	// declare an exception that reads as though it outranks one.
	RequiresNoObjection bool
}

// Validate refuses an exception the engine could apply without checking
// anything.
func (e MarketingException) Validate() error {
	if err := e.Kind.Validate(); err != nil {
		return err
	}
	if !e.RequiresSaleEvidence && !e.RequiresCollectionTimeOptOut &&
		!e.RequiresSimilarity && !e.RequiresNoObjection {
		return fmt.Errorf("marketing exception %q names no condition — an exception the engine checks nothing for is not a lighter rule, it is an unguarded one", string(e.Kind))
	}
	return nil
}

// DisclosureKind names something a first message must carry. Closed for the
// reason ExceptionKind is: the engine renders each kind it knows, and one it
// does not know would be an obligation nothing discharges.
type DisclosureKind string

const (
	// ControllerIdentity is who is writing: the legal name and postal address
	// of the controller.
	ControllerIdentity DisclosureKind = "controller_identity"
	// PrivacyContact is how to reach whoever answers about the data.
	PrivacyContact DisclosureKind = "privacy_contact"
	// ObjectionRoute is how to say stop, free and without a barrier.
	ObjectionRoute DisclosureKind = "objection_route"
	// AdvertiserContact is the advertiser's own reachability — phone, email,
	// address, website — which some jurisdictions require on advertising
	// beyond the controller's identity.
	AdvertiserContact DisclosureKind = "advertiser_contact"
)

// Validate enforces membership in the closed set.
func (k DisclosureKind) Validate() error {
	switch k {
	case ControllerIdentity, PrivacyContact, ObjectionRoute, AdvertiserContact:
		return nil
	}
	return fmt.Errorf("disclosure %q is not one the engine renders", string(k))
}

// Disclosure is one obligation a message carries, and where it binds.
type Disclosure struct {
	Kind DisclosureKind

	// MarketingOnly limits the obligation to advertising. False means every
	// first message carries it.
	MarketingOnly bool
}

// Validate checks the kind.
func (d Disclosure) Validate() error { return d.Kind.Validate() }

// FrequencyCap bounds how many advertising messages one address may receive in
// a rolling window.
//
// It counts messages the recipient actually RECEIVED. A staged message that
// parked and a decision taken in observe mode both describe a message nobody
// got, and counting either would silently consume somebody's allowance — the
// engine holds that invariant, and this type only says what the bound is.
type FrequencyCap struct {
	// Messages is the most that may be received in Window.
	Messages int
	// Window is the rolling period the count covers.
	Window time.Duration
}

// Validate refuses a cap that cannot bind.
func (c FrequencyCap) Validate() error {
	if c.Messages <= 0 {
		return fmt.Errorf("frequency cap allows %d messages — a cap of zero or less would refuse every advertising message, which is a ban stated as a cap", c.Messages)
	}
	if c.Window <= 0 {
		return fmt.Errorf("frequency cap has a window of %s — a cap without a window never expires and would silence an address permanently", c.Window)
	}
	return nil
}

// Validate checks a whole rule set. The boot preflight refuses a pack whose
// rules it cannot apply, rather than composing an installation that believes it
// is following a country's law and is not.
func (r Rules) Validate() error {
	if err := r.Jurisdiction.Validate(); err != nil {
		return err
	}
	if r.Version <= 0 {
		return fmt.Errorf("messaging rules for %q carry version %d — a decision records the version it was taken under, and zero names nothing", string(r.Jurisdiction), r.Version)
	}
	if r.ReplyWindow < 0 || r.DealFollowUpWindow < 0 {
		return fmt.Errorf("messaging rules for %q carry a negative window — a window reaches back, never forward", string(r.Jurisdiction))
	}
	// One rule set may name a kind once. A second entry for the same kind is
	// refused rather than merged, for the reason a duplicate retention class
	// is: the fold would have to pick, and a weaker duplicate silently
	// replacing a stronger one is an exception applied on terms the pack never
	// declared.
	seen := map[ExceptionKind]bool{}
	for _, e := range r.MarketingExceptions {
		if err := e.Validate(); err != nil {
			return fmt.Errorf("messaging rules for %q: %w", string(r.Jurisdiction), err)
		}
		if seen[e.Kind] {
			return fmt.Errorf("messaging rules for %q declare exception %q twice — two sets of conditions for one route is two answers",
				string(r.Jurisdiction), string(e.Kind))
		}
		seen[e.Kind] = true
	}
	for _, d := range r.Disclosures {
		if err := d.Validate(); err != nil {
			return fmt.Errorf("messaging rules for %q: %w", string(r.Jurisdiction), err)
		}
	}
	if r.FrequencyCap != nil {
		if err := r.FrequencyCap.Validate(); err != nil {
			return fmt.Errorf("messaging rules for %q: %w", string(r.Jurisdiction), err)
		}
	}
	return nil
}
