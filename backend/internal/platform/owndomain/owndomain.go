// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package owndomain decides whether an address is at one of the installation's
// own domains. Capture and the offered recipient both ask it, so they fold alike.
package owndomain

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"
)

// Fold folds a mail domain to the one form own domains are compared in:
// lowercased, trailing dot stripped, IDNA-encoded.
//
// A domain IDNA cannot read stays lowercased rather than dropped, so a parse
// failure never reads as "ours", the answer that loses correspondence.
func Fold(domain string) string {
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

// Spellings returns the ASCII form Fold gives for a domain, and its Unicode
// form when that differs. A header may carry either.
// Postgres cannot fold IDNA, so a SQL comparison against stored header
// addresses needs both.
func Spellings(domain string) []string {
	folded := Fold(domain)
	if folded == "" {
		return nil
	}
	out := []string{folded}
	if display, err := idna.Lookup.ToUnicode(folded); err == nil && display != folded {
		out = append(out, display)
	}
	return out
}

// OfAddress is the folded domain of a mail address, or "" when it has none.
func OfAddress(address string) string {
	address = strings.TrimSpace(address)
	at := strings.LastIndex(address, "@")
	if at < 0 || at == len(address)-1 {
		return ""
	}
	return Fold(address[at+1:])
}

// Ownable reports whether somebody registers this domain, as opposed to a
// public suffix anyone may register under.
func Ownable(domain string) bool {
	_, err := publicsuffix.EffectiveTLDPlusOne(domain)
	return err == nil
}

// Set is the own domains, folded once so a membership test is a comparison.
type Set struct {
	domains []string
}

// New folds and de-duplicates the domains and drops any nobody can own.
//
// The public-suffix filter is here, where every writer's domains meet. A
// `co.uk` from any one of them would make every company beneath it ours.
func New(raw []string) Set {
	seen := make(map[string]bool, len(raw))
	out := make([]string, 0, len(raw))
	for _, d := range raw {
		n := Fold(d)
		if n == "" || seen[n] || !Ownable(n) {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return Set{domains: out}
}

// Read folds the domains a query selects into a Set.
func Read(ctx context.Context, tx pgx.Tx, query string) (Set, error) {
	rows, err := tx.Query(ctx, query)
	if err != nil {
		return Set{}, fmt.Errorf("owndomain: reading the own-domain set: %w", err)
	}
	raw, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return Set{}, fmt.Errorf("owndomain: reading the own-domain set: %w", err)
	}
	return New(raw), nil
}

// Empty reports whether the set holds no domain, so it covers nobody.
func (s Set) Empty() bool { return len(s.domains) == 0 }

// Covers reports whether the address is at one of the domains or a subdomain.
//
// The suffix test includes the dot, so acme.com does not cover acme.com.example.net.
// An address with no readable domain is not covered.
func (s Set) Covers(address string) bool {
	return s.CoversDomain(OfAddress(address))
}

// CoversDomain is Covers for a domain already separated from its address.
func (s Set) CoversDomain(domain string) bool {
	domain = Fold(domain)
	if domain == "" {
		return false
	}
	for _, own := range s.domains {
		if domain == own || strings.HasSuffix(domain, "."+own) {
			return true
		}
	}
	return false
}

// Domains returns a copy of the folded domains, so a caller sorting its display
// list cannot change what counts as ours.
func (s Set) Domains() []string {
	return append([]string(nil), s.domains...)
}
