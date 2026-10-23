// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// A company on a subdomain of a domain another company holds is a review pair.
// Such a subdomain is usually a product or a branch of the holder. Sometimes it
// is a separate business, which a human can tell apart. The pair never routes
// and never refuses. No consumer-mail exclusion applies, because PO-F-2
// matches domains and does not judge them.
//
// The pair is evidence and not a score, so it is filed at confidence 0. The
// nightly re-check keeps it only while the two companies' domains still nest
// (companyDomainsNest), the same test the filing path asks.

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/freemail"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// domainAncestors lists the domains a subdomain sits under, nearest first, down
// to its registrable domain: `team.eu.example.com` sits under `eu.example.com`
// and `example.com`. A registrable domain sits under nothing, because a public
// suffix such as `co.uk` is nobody's company.
func domainAncestors(domain string) []string {
	sub := normalizeDomain(domain)
	registrable := freemail.Registrable(sub)
	if registrable == "" || registrable == sub || !strings.HasSuffix(sub, "."+registrable) {
		return nil
	}
	var ancestors []string
	for rest := sub; rest != registrable; {
		_, parent, _ := strings.Cut(rest, ".")
		ancestors = append(ancestors, parent)
		rest = parent
	}
	return ancestors
}

// domainNests is the subdomain lane's pair test: one of the two domains sits
// under the other.
func domainNests(a, b string) bool {
	return slices.Contains(domainAncestors(a), normalizeDomain(b)) ||
		slices.Contains(domainAncestors(b), normalizeDomain(a))
}

// subdomainClaimants answers every live, non-anchor company holding a domain
// one of the candidate's domains sits under. It gives one score per company,
// carrying the nearest held ancestor, sorted by id so the queue does not
// shuffle between runs.
func subdomainClaimants(ctx context.Context, tx pgx.Tx, domains []string, exclude *ids.CompanyID) ([]CompanyCandidateScore, error) {
	var claimants []CompanyCandidateScore
	for _, d := range domains {
		ancestors := domainAncestors(d)
		if len(ancestors) == 0 {
			continue
		}
		rows, err := tx.Query(ctx, `
			SELECT DISTINCT ON (d.company_id) d.company_id, d.domain FROM company_domain d
			  JOIN company c ON c.id = d.company_id
			 WHERE d.domain = ANY($1) AND d.archived_at IS NULL
			   AND c.archived_at IS NULL AND NOT c.is_anchor
			   AND ($2::uuid IS NULL OR d.company_id <> $2)
			 ORDER BY d.company_id, length(d.domain) DESC`, ancestors, exclude)
		if err != nil {
			return nil, fmt.Errorf("dedupe company subdomain: %w", err)
		}
		held, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (CompanyCandidateScore, error) {
			score := CompanyCandidateScore{MatchedField: laneDomain, CandidateValue: normalizeDomain(d)}
			err := row.Scan(&score.CompanyID, &score.IncumbentValue)
			return score, err
		})
		if err != nil {
			return nil, fmt.Errorf("dedupe company subdomain: %w", err)
		}
		for _, score := range held {
			if !slices.ContainsFunc(claimants, func(s CompanyCandidateScore) bool { return s.CompanyID == score.CompanyID }) {
				claimants = append(claimants, score)
			}
		}
	}
	slices.SortFunc(claimants, func(a, b CompanyCandidateScore) int {
		return cmp.Compare(a.CompanyID.String(), b.CompanyID.String())
	})
	return claimants, nil
}

// fileSubdomainClaimants puts the company that just claimed a subdomain and
// every holder of a domain it sits under on the review queue. It runs where
// every domain claim lands (insertCompanyDomains), so a create and an edit that
// adds a domain file the same pairs.
func fileSubdomainClaimants(ctx context.Context, tx pgx.Tx, claimant ids.CompanyID, domain, source, by string) error {
	claimants, err := subdomainClaimants(ctx, tx, []string{domain}, &claimant)
	if err != nil {
		return err
	}
	for _, holder := range claimants {
		if _, err := recordDedupeCandidate(ctx, tx, entityCompany, claimant.UUID, holder.CompanyID.UUID, 0,
			nearMatchEvidence(laneDomain, holder.CandidateValue, holder.IncumbentValue, 0), source, by); err != nil {
			return fmt.Errorf("record company subdomain candidate: %w", err)
		}
	}
	return nil
}

// companyDomainsNest reports whether a live domain of one company sits under a
// live domain of the other, for the nightly re-check.
func companyDomainsNest(ctx context.Context, tx pgx.Tx, left, right ids.UUID) (bool, error) {
	rows, err := tx.Query(ctx, `
		SELECT company_id, domain FROM company_domain
		 WHERE company_id = ANY($1) AND archived_at IS NULL`, []ids.UUID{left, right})
	if err != nil {
		return false, fmt.Errorf("read dedupe company domains: %w", err)
	}
	type held struct {
		company ids.UUID
		domain  string
	}
	domains, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (held, error) {
		var h held
		err := row.Scan(&h.company, &h.domain)
		return h, err
	})
	if err != nil {
		return false, fmt.Errorf("read dedupe company domains: %w", err)
	}
	for _, l := range domains {
		for _, r := range domains {
			if l.company == left && r.company == right && domainNests(l.domain, r.domain) {
				return true, nil
			}
		}
	}
	return false, nil
}
