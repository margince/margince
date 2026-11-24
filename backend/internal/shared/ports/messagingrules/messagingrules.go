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
	"strings"
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
	// Instrument is one law a rule set states, and when it took effect.
	Instrument = pub.Instrument
	// Provenance is who reviewed a rule set, and when.
	Provenance = pub.Provenance
	// Citation names the law behind one engine finding or refusal.
	Citation = pub.Citation
	// Territory narrows a citation to where the sender is established.
	Territory = pub.Territory
	// ExceptionKind names a route to lawful advertising without consent.
	ExceptionKind = pub.ExceptionKind
	// DisclosureKind names something a first message must carry.
	DisclosureKind = pub.DisclosureKind
	// ContactMeans names one way of reaching someone a disclosure must name.
	ContactMeans = pub.ContactMeans
	// PurposeClass names a class of consent purpose.
	PurposeClass = pub.PurposeClass
	// MarketingMode says whether advertising needs consent or is allowed until a refusal.
	MarketingMode = pub.MarketingMode
)

// The published constants, re-exported.
const (
	// ExistingCustomer is the sale-derived advertising exception.
	ExistingCustomer = pub.ExistingCustomer
	// MarketingOptIn needs the recipient's consent before advertising.
	MarketingOptIn = pub.MarketingOptIn
	// MarketingOptOut allows advertising until the recipient refuses.
	MarketingOptOut = pub.MarketingOptOut
	// ControllerIdentity is who is writing.
	ControllerIdentity = pub.ControllerIdentity
	// PrivacyContact is who answers about the data.
	PrivacyContact = pub.PrivacyContact
	// ObjectionRoute is how to say stop.
	ObjectionRoute = pub.ObjectionRoute
	// AdvertiserContact is the advertiser's own reachability.
	AdvertiserContact = pub.AdvertiserContact
	// MeansWeb is a web page or a social-network link.
	MeansWeb = pub.MeansWeb
	// MeansEmail is an email address.
	MeansEmail = pub.MeansEmail
	// MeansPhone is a phone number.
	MeansPhone = pub.MeansPhone
	// AnySender cites whatever the sender's country.
	AnySender = pub.AnySender
	// SenderHere cites only a sender established in the pack's jurisdiction.
	SenderHere = pub.SenderHere
	// SenderElsewhere cites only a sender established outside it.
	SenderElsewhere = pub.SenderElsewhere
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
	for i := range out.Disclosures {
		out.Disclosures[i].Means = slices.Clone(r.Disclosures[i].Means)
	}
	out.Instruments = slices.Clone(r.Instruments)
	out.Citations = slices.Clone(r.Citations)
	out.ClaimsNeedProof = slices.Clone(r.ClaimsNeedProof)
	out.ConsentRequestFirstFor = slices.Clone(r.ConsentRequestFirstFor)

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

// AnyOptOut reports whether a registered jurisdiction allows advertising until
// the recipient refuses. A recipient whose country is unknown could be in one.
func AnyOptOut() bool {
	mu.RLock()
	defer mu.RUnlock()
	for _, r := range rules {
		if r.MarketingMode.OptOut() {
			return true
		}
	}
	return false
}

// Strictest folds the applicable jurisdictions into the one rule set the
// engine applies, taking the STRICTEST answer for each obligation.
//
// The engine's marketing selector names one code per recipient. When a caller
// names several, each obligation resolves to whichever position permits less.
// That is the shortest non-zero window, every declared disclosure, the
// tightest cap, and an exception only where every jurisdiction grants it.
//
// An unknown jurisdiction contributes nothing to fold, which is why the caller
// must treat "no rules at all" as the consent-only floor rather than as
// permission — Strictest cannot invent an obligation for a country it was
// never told about.
//
// THE APPLIED CODES are the third answer, and they are what a decision records.
// A fold is no one jurisdiction's rule set, so stricter() clears Jurisdiction
// and Version rather than misnaming the fold with one country's label — which
// left a folded decision with nothing to say about the rules it was judged
// under. The codes that actually contributed answer that, and they are not the
// codes the caller passed in: an unknown one contributes nothing and must not
// appear as though it bound the send.
func Strictest(codes ...jurisdiction.Code) (Rules, Applied, bool) {
	var out Rules
	var applied Applied
	found := false
	for _, code := range codes {
		r, ok := For(code)
		if !ok {
			applied.Unknown = append(applied.Unknown, code)
			continue
		}
		applied.Codes = append(applied.Codes, code)
		applied.SubjectPrefixes = AddSubjectPrefixes(applied.SubjectPrefixes, r.SubjectPrefix)
		if !found {
			out, found = r, true
			continue
		}
		out = stricter(out, r)
	}
	return out, applied, found
}

// Applied names which jurisdictions a fold actually used.
//
// A DECISION RECORDS THIS, because Rules alone cannot say it. A single
// jurisdiction's rule set carries its own code and version; a fold of two
// carries neither, by design. Without the codes, a subject asking which rules
// judged their message would get an answer only for the installations that
// happen to declare one country.
type Applied struct {
	// Codes are the jurisdictions that contributed obligations, in the order
	// they were asked about. One entry means Rules.Version is meaningful;
	// more than one means the fold zeroed it and these are the answer.
	Codes []jurisdiction.Code

	// Unknown are the codes named that no pack declares. They bound nothing;
	// the baseline applies to them.
	Unknown []jurisdiction.Code

	// SubjectPrefixes are the distinct advertising labels the contributing
	// packs owe, first declared first. Rules.SubjectPrefix holds the first one
	// only, because one subject begins with one label. The subject check reads
	// this list, so a second, different label is still owed.
	SubjectPrefixes []string
}

// AddSubjectPrefixes appends each declared label that labels does not already
// hold. Two packs owing the same label, in any case, owe it once.
func AddSubjectPrefixes(labels []string, declared ...string) []string {
	for _, d := range declared {
		label := strings.TrimSpace(d)
		if label != "" && !slices.ContainsFunc(labels, func(l string) bool { return strings.EqualFold(l, label) }) {
			labels = append(labels, label)
		}
	}
	return labels
}

// Folded reports whether more than one jurisdiction contributed, which is
// exactly when Rules.Version stops meaning anything.
func (a Applied) Folded() bool { return len(a.Codes) > 1 }

// stricter folds one rule set into another, obligation by obligation.
func stricter(a, b Rules) Rules {
	out := a
	// The fold is no longer any one jurisdiction's, and stamping it with one
	// country's code would misname the rules a decision was taken under.
	out.Jurisdiction = ""
	// A version is only meaningful for a single jurisdiction's set; a folded
	// one carries none, and the decision records Applied.Codes instead — which
	// Strictest returns for exactly this reason.
	out.Version = 0
	// Provenance, citations and digest describe one pack's source. A fold has
	// none, and the citation lookup asks each recipient's own pack.
	out.Provenance, out.Citations, out.Digest = Provenance{}, nil, ""
	out.ReplyWindow = shorterWindow(a.ReplyWindow, b.ReplyWindow)
	out.DealFollowUpWindow = shorterWindow(a.DealFollowUpWindow, b.DealFollowUpWindow)
	// Every disclosure either side requires. An obligation one country imposes
	// is not lifted by another that does not.
	out.Disclosures = append(append([]Disclosure{}, a.Disclosures...), b.Disclosures...)
	// The first prefix either side requires. A second, different one cannot
	// stand on the same subject; Strictest keeps it in Applied.SubjectPrefixes,
	// and the subject check holds such a message.
	if out.SubjectPrefix == "" {
		out.SubjectPrefix = b.SubjectPrefix
	}
	out.FrequencyCap = tighterCap(a.FrequencyCap, b.FrequencyCap)
	out.ConsentOnly = a.ConsentOnly || b.ConsentOnly
	// Opt-out only where both sides allow it. The send decision does not read this
	// fold: its mode is the recipient's pack alone (packRulesFor).
	if !a.MarketingMode.OptOut() || !b.MarketingMode.OptOut() {
		out.MarketingMode = ""
	}
	out.OptOutAcknowledgement = a.OptOutAcknowledgement || b.OptOutAcknowledgement
	out.OptOutAcknowledgementWithin = shorterWindow(a.OptOutAcknowledgementWithin, b.OptOutAcknowledgementWithin)
	// A floor either side requires holds, so the longer one wins.
	out.OptOutRecordsKeep = max(a.OptOutRecordsKeep, b.OptOutRecordsKeep)
	// One receipt prints one zone. The first declared wins, as the prefix does.
	if out.OptOutAcknowledgementUTCOffset == 0 {
		out.OptOutAcknowledgementUTCOffset = b.OptOutAcknowledgementUTCOffset
	}
	out.DataProtectionFollowsRecipient = a.DataProtectionFollowsRecipient || b.DataProtectionFollowsRecipient
	out.NoLegitimateInterests = a.NoLegitimateInterests || b.NoLegitimateInterests
	if out.DisclosureLanguage == "" {
		out.DisclosureLanguage = b.DisclosureLanguage
	}
	out.RecordConfirmationWithoutNews = a.RecordConfirmationWithoutNews || b.RecordConfirmationWithoutNews
	out.ConsentConfirmationNeedsRequest = a.ConsentConfirmationNeedsRequest || b.ConsentConfirmationNeedsRequest
	out.ConsentRequestFirstFor = requestFirstFor(a, b)
	// An exception only where BOTH grant it: a route to advertising without
	// consent that one jurisdiction does not recognise is not available in a
	// send both govern.
	out.MarketingExceptions = commonExceptions(a.MarketingExceptions, b.MarketingExceptions)
	foldAdvertisingContent(&out, b)
	return out
}

// foldAdvertisingContent adds b's advertising content rules to out. A claim
// either side restricts stays restricted.
func foldAdvertisingContent(out *Rules, b Rules) {
	out.RefusalSectionLast = out.RefusalSectionLast || b.RefusalSectionLast
	out.ForeignAdvertiserNeedsLocalAgency = out.ForeignAdvertiserNeedsLocalAgency || b.ForeignAdvertiserNeedsLocalAgency
	out.ClaimsNeedProof = unionOf(out.ClaimsNeedProof, b.ClaimsNeedProof)
}

// requestFirstFor is the classes either side's request rule covers. A side
// whose rule covers every class makes the fold cover every class.
func requestFirstFor(a, b Rules) []PurposeClass {
	var out []PurposeClass
	for _, side := range []Rules{a, b} {
		if !side.ConsentConfirmationNeedsRequest {
			continue
		}
		if len(side.ConsentRequestFirstFor) == 0 {
			return nil
		}
		for _, class := range side.ConsentRequestFirstFor {
			if !slices.Contains(out, class) {
				out = append(out, class)
			}
		}
	}
	return out
}

// unionOf returns the words of a, then the words of b that a lacks, in a new slice.
func unionOf(a, b []string) []string {
	out := slices.Clone(a)
	for _, word := range b {
		if !slices.Contains(out, word) {
			out = append(out, word)
		}
	}
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
