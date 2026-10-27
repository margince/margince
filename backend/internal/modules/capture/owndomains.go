// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The internal-vs-external decision (ADR-0082/A127, formulas §20), in one
// place for mail and calendar alike. One implementation, because a rule that
// holds in one channel and not the other is not a confidentiality control.

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/mailidentity"
	"github.com/margince/margince/backend/internal/platform/owndomain"
)

// InternalDomains is the workspace's own mail domains. owndomain.Set holds the
// fold and the coverage test; this adds what capture asks of a message.
type InternalDomains struct {
	owndomain.Set
}

// NewInternalDomains folds the registered domains through the one own-domain rule.
func NewInternalDomains(raw []string) InternalDomains {
	return InternalDomains{owndomain.New(raw)}
}

// AllInternal reports whether every one of these addresses is on an own domain
// — the zero-rows condition (formulas §20).
//
// False when the set is empty, and false when there are no addresses to judge:
// both are "we cannot say this is internal", and the direction to fail in is
// toward keeping a message. One external participant makes the whole message
// external, which is what keeps the intro motion working — a colleague writing
// to a prospect with the prospect copied is correspondence, not chatter.
func (d InternalDomains) AllInternal(addresses []string) bool {
	if d.Empty() {
		return false
	}
	judged := 0
	for _, a := range addresses {
		if strings.TrimSpace(a) == "" {
			continue
		}
		if !d.Covers(a) {
			return false
		}
		judged++
	}
	return judged > 0
}

// External returns the addresses that are NOT on an own domain, in the order
// given and de-duplicated — the parties a captured message may create records
// for. It is deliberately separate from the message's author: which party WROTE
// a message and which parties are candidates for a record are two questions,
// and answering the second with the first records a prospect as the author of a
// colleague's mail (ADR-0082 §3).
func (d InternalDomains) External(addresses []string) []string {
	seen := make(map[string]bool, len(addresses))
	out := make([]string, 0, len(addresses))
	for _, a := range addresses {
		a = strings.ToLower(strings.TrimSpace(a))
		if a == "" || seen[a] || d.Covers(a) {
			continue
		}
		seen[a] = true
		out = append(out, a)
	}
	return out
}

// anchorDomains is what the installation's own company currently claims.
const anchorDomains = auth.OwnCompanyDomainsSQL

// ownDomainsTx reads every domain that might be ours — the company's own, plus
// every candidate a connected mailbox contributed. This is the set for
// decisions that only affect whether a COLLEAGUE becomes a contact; getting
// that wrong costs a junk record, which a human can see and undo.
func ownDomainsTx(ctx context.Context, tx pgx.Tx) (InternalDomains, error) {
	return queryDomains(ctx, tx, `SELECT domain FROM workspace_email_domain UNION `+anchorDomains)
}

// TrustedOwnDomainsTx reads only the domains a human vouched for: those the own
// company claims, and those an administrator confirmed.
//
// Storing a message or not is a stronger consequence than creating a contact or
// not, so it takes the stronger evidence. A mailbox on its own is not evidence
// — it proves whose mailbox it is, never whose domain it is, and a contractor's
// genuine account at a customer's company would otherwise suppress that
// customer's correspondence workspace-wide.
//
// Asked fresh every time rather than stamped on the row when the mailbox was
// seen. A company that corrects a mistyped domain, or changes it, stops
// suppressing the old one immediately — a cached answer would go on hiding mail
// for a domain nobody claims any more, and nothing would ever revoke it.
func TrustedOwnDomainsTx(ctx context.Context, tx pgx.Tx) (InternalDomains, error) {
	return queryDomains(ctx, tx, auth.TrustedOwnDomainsSQL)
}

func queryDomains(ctx context.Context, tx pgx.Tx, query string) (InternalDomains, error) {
	own, err := owndomain.Read(ctx, tx, query)
	return InternalDomains{own}, err
}

// SelfSet is one SEAT's own addresses: the exact aliases they declared, plus
// any private domains of theirs, which cover subdomains the way the workspace's
// own domains do.
//
// It sits beside InternalDomains because the two answer the same shape of
// question about different subjects — "is this address us" against "is this
// address me" — and composes one rather than restating it: a domain claim is a
// domain claim, and a second suffix test that folded case or handled
// subdomains differently would disagree with this one on exactly the addresses
// nobody writes a fixture for.
//
// The zero value is a seat that has declared nothing, and it answers false to
// everything — which leaves every gate reading it exactly where it was.
type SelfSet struct {
	addresses map[string]struct{}
	// identityOnly are addresses that say WHO the seat is without proving what
	// reached their mailbox. They answer Covers and never CoversAddressExactly —
	// see that method for why the two questions cannot share one set.
	identityOnly map[string]struct{}
	domains      InternalDomains
}

// NewSelfSet folds and de-duplicates a seat's declared addresses and domains.
func NewSelfSet(addresses, domains []string) SelfSet {
	return NewSelfSetWithIdentityOnly(addresses, nil, domains)
}

// NewSelfSetWithIdentityOnly separates the addresses that PROVE DELIVERY from
// the ones that merely name the seat.
//
// The distinction is not a nicety. An address in the first group is either the
// mailbox a provider attested at grant or one the seat declared about
// themselves; an address in the second is one the product knows belongs to them
// for another reason — the address they sign in with — which nobody proved they
// control as a mailbox. Only the first can answer "did this message reach me",
// because that answer grants read access to an incumbent message a colleague
// may have captured first.
func NewSelfSetWithIdentityOnly(addresses, identityOnly, domains []string) SelfSet {
	set := SelfSet{
		addresses:    make(map[string]struct{}, len(addresses)),
		identityOnly: make(map[string]struct{}, len(identityOnly)),
		domains:      NewInternalDomains(domains),
	}
	for _, address := range addresses {
		if folded := foldAddress(address); folded != "" {
			set.addresses[folded] = struct{}{}
		}
	}
	for _, address := range identityOnly {
		folded := foldAddress(address)
		if folded == "" {
			continue
		}
		if _, proven := set.addresses[folded]; proven {
			// The same address arrived both ways. The stronger claim stands:
			// listing it here as well would not weaken it, but keeping one
			// address in two sets invites a later reader to ask which wins.
			continue
		}
		set.identityOnly[folded] = struct{}{}
	}
	return set
}

// foldAddress is one address in the form both sides of a comparison agree on.
// Plus-addressing is kept: `me+news@` is a different address.
func foldAddress(address string) string { return mailidentity.FoldAddress(address) }

// Covers reports whether this address is one the seat declared as their own,
// exactly or by one of their domains.
// CoversAddressExactly is Covers restricted to the addresses a seat holds, with
// no domain arm.
//
// The two answer different questions. Covers asks "is this me", which a domain
// claim legitimately settles for filing. This asks "did this reach MY mailbox",
// which a domain claim cannot: a seat declares a domain with no proof of control
// (ValidExclusionValue checks syntax, nothing else), so a domain arm here would
// let anybody claim `acme.com` and then treat any message naming an acme.com
// address as delivered to them. An exact address is different in kind — it is
// either the mailbox the provider attested at grant, or one the seat declared
// about themselves, and neither names a colleague.
//
// The seat's LOGIN address is deliberately not among them, for the same reason
// in a different shape. It says who the seat is and proves nothing about what
// their mailbox received, so admitting it here would let a message that merely
// NAMES that address stand as proof of delivery — and a message's identity is a
// Message-ID the sender types. A colleague's held mail would then be reachable
// by anybody who could get their own capture to name the right address.
func (s SelfSet) CoversAddressExactly(address string) bool {
	folded := foldAddress(address)
	if folded == "" {
		return false
	}
	_, ok := s.addresses[folded]
	return ok
}

func (s SelfSet) Covers(address string) bool {
	folded := foldAddress(address)
	if folded == "" {
		return false
	}
	if _, ok := s.addresses[folded]; ok {
		return true
	}
	if _, ok := s.identityOnly[folded]; ok {
		return true
	}
	return s.domains.Covers(folded)
}

// Empty reports whether the seat declared nothing, which lets a caller skip
// work rather than test every address against an empty set.
func (s SelfSet) Empty() bool {
	return len(s.addresses) == 0 && len(s.identityOnly) == 0 && s.domains.Empty()
}

// WithoutSelf is the addresses that are NOT the seat's own, order preserved.
// The gates need the remainder rather than a yes/no: what makes a message
// internal is that nothing external is left, and what the creation ladder is
// about is the first thing that IS.
func (s SelfSet) WithoutSelf(addresses []string) []string {
	if s.Empty() {
		return addresses
	}
	out := make([]string, 0, len(addresses))
	for _, address := range addresses {
		// A blank entry is not an external party. AllInternal already skips
		// one, so letting it through here would make an owner-only message
		// look like it still had somebody else on it — "nothing external is
		// left" has to mean the same thing in both places.
		if strings.TrimSpace(address) == "" || s.Covers(address) {
			continue
		}
		out = append(out, address)
	}
	return out
}

// meetingPartiesTx reads what a meeting's sides are judged against, around the
// trusted domains the caller already holds.
func meetingPartiesTx(ctx context.Context, tx pgx.Tx, trusted InternalDomains) (owndomain.Parties, error) {
	possible, err := ownDomainsTx(ctx, tx)
	if err != nil {
		return owndomain.Parties{}, err
	}
	consumer, err := MatcherTx(ctx, tx)
	if err != nil {
		return owndomain.Parties{}, err
	}
	return owndomain.Parties{Trusted: trusted.Set, Possible: possible.Set, Consumer: consumer.IsConsumer}, nil
}
