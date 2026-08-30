// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The internal-vs-external decision (ADR-0082/A127, formulas §20), in one
// place for mail and calendar alike. One implementation, because a rule that
// holds in one channel and not the other is not a confidentiality control.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"
)

// normalizeDomain folds a mail domain to the one form the own-domain set is
// compared in: lowercased, trailing dot stripped, IDNA-encoded. A domain that
// fails IDNA is returned lowercased rather than dropped — the caller is deciding
// whether to KEEP a message, and discarding an unreadable domain here would
// silently turn a parse failure into "internal", which is the one answer that
// loses correspondence rather than keeping it.
func normalizeDomain(domain string) string {
	domain = strings.ToLower(strings.TrimSpace(domain))
	domain = strings.TrimSuffix(domain, ".")
	if domain == "" {
		return ""
	}
	ascii, err := idna.Lookup.ToASCII(domain)
	if err != nil {
		return domain
	}
	return ascii
}

// domainOfAddress returns the normalized domain of a mail address, or "" when
// the address carries none that can be read. An address with no readable domain
// is not internal (see InternalDomains.Covers).
func domainOfAddress(address string) string {
	address = strings.TrimSpace(address)
	at := strings.LastIndex(address, "@")
	if at < 0 || at == len(address)-1 {
		return ""
	}
	return normalizeDomain(address[at+1:])
}

// InternalDomains is the workspace's own mail domains, normalized once so a
// membership test is a comparison rather than a query.
type InternalDomains struct {
	domains []string
}

// NewInternalDomains normalizes and de-duplicates the registered domains, and
// drops any that are not a domain someone can own.
//
// The public-suffix filter lives HERE, where the set is built, rather than only
// where an administrator types one in. The set is fed from three places — the
// admin surface, the mailbox seed, and the company's own registered domains —
// and the company's come from a website field a human filled in during cold
// start, which no validation of this kind has ever seen. A `co.uk` reaching the
// set from any of them would make every company beneath it internal and stop
// the workspace keeping their correspondence, unrecoverably. Filtering at
// construction makes that impossible to reintroduce by adding a fourth writer.
func NewInternalDomains(raw []string) InternalDomains {
	seen := make(map[string]bool, len(raw))
	out := make([]string, 0, len(raw))
	for _, d := range raw {
		n := normalizeDomain(d)
		if n == "" || seen[n] || !ownableDomain(n) {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return InternalDomains{domains: out}
}

// ownableDomain reports whether a domain is one somebody registers, as opposed
// to a public suffix under which anyone may register. EffectiveTLDPlusOne fails
// for exactly the latter.
func ownableDomain(domain string) bool {
	_, err := publicsuffix.EffectiveTLDPlusOne(domain)
	return err == nil
}

// Empty reports whether the workspace has registered no own domain at all.
//
// An empty set makes NOTHING internal, so every message is captured. That is
// the honest posture rather than a fallback guess: an installation that has
// named no domain of its own is making no claim about what its people's mail
// is, and inventing one from a connected mailbox would be right in some
// workspaces and wrong in the rest.
func (d InternalDomains) empty() bool { return len(d.domains) == 0 }

// Covers reports whether address belongs to one of the workspace's own domains.
//
// A registered domain covers its SUBDOMAINS: acme.com covers mail.acme.com.
// Exact string equality was the failure mode with teeth — it leaks the internal
// mail of every company that sends from a subdomain, and it looks exactly like
// working correctly. The suffix test includes the separating dot so acme.com
// does not cover the unrelated acme.com.example.net.
//
// An address with no readable domain is NOT covered, so the message is kept.
func (d InternalDomains) Covers(address string) bool {
	return d.CoversDomain(domainOfAddress(address))
}

// CoversDomain is Covers for a domain already separated from its address.
func (d InternalDomains) CoversDomain(domain string) bool {
	domain = normalizeDomain(domain)
	if domain == "" {
		return false
	}
	for _, own := range d.domains {
		if domain == own || strings.HasSuffix(domain, "."+own) {
			return true
		}
	}
	return false
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
	if d.empty() {
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

// anchorDomains is what the installation's own company currently claims. It is
// a READ of another module's table, which reads are free to be: the question is
// "does a human say this domain is ours", and the anchor organization is the
// only place that answer lives.
const anchorDomains = `
	SELECT d.domain
	  FROM organization_domain d
	  JOIN organization o ON o.id = d.organization_id
	 WHERE o.is_anchor AND o.archived_at IS NULL AND d.archived_at IS NULL`

// ownDomainsTx reads every domain that might be ours — the company's own, plus
// every candidate a connected mailbox contributed. This is the set for
// decisions that only affect whether a COLLEAGUE becomes a contact; getting
// that wrong costs a junk record, which a human can see and undo.
func ownDomainsTx(ctx context.Context, tx pgx.Tx) (InternalDomains, error) {
	return queryDomains(ctx, tx, `SELECT domain FROM workspace_email_domain UNION `+anchorDomains)
}

// trustedOwnDomainsTx reads only the domains a human vouched for: those the own
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
func trustedOwnDomainsTx(ctx context.Context, tx pgx.Tx) (InternalDomains, error) {
	return queryDomains(ctx, tx,
		`SELECT domain FROM workspace_email_domain WHERE verified UNION `+anchorDomains)
}

func queryDomains(ctx context.Context, tx pgx.Tx, query string) (InternalDomains, error) {
	rows, err := tx.Query(ctx, query)
	if err != nil {
		return InternalDomains{}, fmt.Errorf("capture: reading the own-domain set: %w", err)
	}
	defer rows.Close()

	var raw []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return InternalDomains{}, fmt.Errorf("capture: reading the own-domain set: %w", err)
		}
		raw = append(raw, d)
	}
	if err := rows.Err(); err != nil {
		return InternalDomains{}, fmt.Errorf("capture: reading the own-domain set: %w", err)
	}
	return NewInternalDomains(raw), nil
}

// Domains returns the normalized domains in the set, for a caller that needs to
// show them rather than test against them.
//
// A COPY: the set decides which mail is stored, and every other method on it
// only reads. Handing out the backing slice would let a caller that appends or
// sorts its display list change what counts as internal, with nothing at the
// call site to suggest it had.
func (d InternalDomains) Domains() []string {
	return append([]string(nil), d.domains...)
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
	domains   InternalDomains
}

// NewSelfSet folds and de-duplicates a seat's declared addresses and domains.
func NewSelfSet(addresses, domains []string) SelfSet {
	set := SelfSet{addresses: make(map[string]struct{}, len(addresses)), domains: NewInternalDomains(domains)}
	for _, address := range addresses {
		if folded := foldAddress(address); folded != "" {
			set.addresses[folded] = struct{}{}
		}
	}
	return set
}

// foldAddress is one address in the form both sides of a comparison agree on:
// lowercased, and its DOMAIN half normalized the way a claimed domain is, so a
// unicode domain and its punycode form are one address rather than two.
//
// The local part is folded in case and otherwise left alone. Plus-addressing is
// deliberately NOT stripped: `me+news@` is a different address, and treating it
// as the same would silence mail to an address the seat never declared — the
// direction that loses a message rather than keeping one.
func foldAddress(address string) string {
	folded := strings.ToLower(strings.TrimSpace(address))
	at := strings.LastIndex(folded, "@")
	if at < 0 {
		return folded
	}
	domain := normalizeDomain(folded[at+1:])
	if domain == "" {
		return folded
	}
	return folded[:at+1] + domain
}

// Covers reports whether this address is one the seat declared as their own,
// exactly or by one of their domains.
func (s SelfSet) Covers(address string) bool {
	folded := foldAddress(address)
	if folded == "" {
		return false
	}
	if _, ok := s.addresses[folded]; ok {
		return true
	}
	return s.domains.Covers(folded)
}

// Empty reports whether the seat declared nothing, which lets a caller skip
// work rather than test every address against an empty set.
func (s SelfSet) Empty() bool { return len(s.addresses) == 0 && s.domains.empty() }

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
		if !s.Covers(address) {
			out = append(out, address)
		}
	}
	return out
}
