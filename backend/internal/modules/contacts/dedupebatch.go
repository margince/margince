// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The ladder's answer about records a batch will create but has not written.
// A batch writer lands its rows one at a time, so its commit meets each earlier
// row as an incumbent. A dry run writes nothing and carries those rows itself.
// A member is a rival only when the ladder would both admit and score it: the
// per-row scorers run in Go, and the query's admission predicate runs in
// Postgres over the members the scorer kept.

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

// CompanyBatch is the companies a batch will create, in the order it decided.
type CompanyBatch struct {
	members []batchCompany
}

type batchCompany struct {
	display, legal string
	names          companyNamePair
	domains        []string
}

// Add records a company the batch will create.
func (b *CompanyBatch) Add(c CompanyCandidate) {
	domains := make([]string, 0, len(c.Domains))
	for _, d := range c.Domains {
		domains = append(domains, normalizeDomain(d))
	}
	b.members = append(b.members, batchCompany{
		display: c.DisplayName, legal: c.LegalName,
		names: companyNamePairOf(c.DisplayName, c.LegalName), domains: domains,
	})
}

// HasRival answers whether DedupeCompany would name an earlier member as a
// duplicate of c, had that member been written. Either c's display name spells
// a member's domain, or the fuzzy tier admits a member and scores it over the
// threshold.
func (b *CompanyBatch) HasRival(ctx context.Context, tx pgx.Tx, c CompanyCandidate) (bool, error) {
	spelled, scored := b.scoredRivals(c)
	if spelled || len(scored) == 0 {
		return spelled, nil
	}
	displays := make([]string, 0, len(scored))
	legals := make([]string, 0, len(scored))
	for _, m := range scored {
		displays = append(displays, m.display)
		legals = append(legals, m.legal)
	}
	args := []any{displays, legals}
	arms := companyAdmissionArms(&args, c)
	if len(arms) == 0 {
		return false, nil
	}
	// The columns carry fuzzyCompany's names, so its arms read these rows
	// unchanged. An empty legal name is NULL, as the stored column is.
	var admitted bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM (SELECT d AS display_name, NULLIF(l, '') AS legal_name
		                   FROM unnest($1::text[], $2::text[]) AS m(d, l)) AS company
		   WHERE `+strings.Join(arms, " OR ")+`)`, args...).Scan(&admitted); err != nil {
		return false, fmt.Errorf("dedupe company batch admission: %w", err)
	}
	return admitted, nil
}

// scoredRivals is HasRival's Go half: whether c's display name spells a
// member's domain, and the members the fuzzy scorer puts over the threshold.
func (b *CompanyBatch) scoredRivals(c CompanyCandidate) (bool, []batchCompany) {
	if NormalizeCompanyName(c.DisplayName) == "" && NormalizeCompanyName(c.LegalName) == "" {
		return false, nil
	}
	candidate := companyNamePairOf(c.DisplayName, c.LegalName)
	spelled, spellsDomain := displayNameDomain(c.DisplayName)
	var scored []batchCompany
	for _, held := range b.members {
		if spellsDomain && slices.Contains(held.domains, spelled) {
			return true, nil
		}
		if _, rival := companyPairRival(candidate, held.names); rival {
			scored = append(scored, held)
		}
	}
	return false, scored
}

// ContactBatch is the contacts a batch will create, filed under the mail
// domains of their addresses.
//
// Filed by domain because a batch row carries no employer. A shared mail
// domain is then the only employer evidence between two rows, and a name alone
// cannot clear the threshold.
type ContactBatch struct {
	namesByDomain map[string][]string
}

// Add records a contact the batch will create. A consumer mail domain is not
// filed: the ladder never reads one as a shared employer.
func (b *ContactBatch) Add(c ContactCandidate) {
	if b.namesByDomain == nil {
		b.namesByDomain = map[string][]string{}
	}
	for _, e := range c.Emails {
		if domain := emailDomain(e); domain != "" && sharedEmployerDomain(c, domain) {
			b.namesByDomain[domain] = append(b.namesByDomain[domain], c.FullName)
		}
	}
}

// BestRival is the confidence the fuzzy tier of DedupeContact would give the
// best earlier member it admits, had that member been written, or 0 when none
// clears the threshold. An address two rows share is the exact tier's
// question, which the batch asks first.
func (b *ContactBatch) BestRival(ctx context.Context, tx pgx.Tx, c ContactCandidate) (float64, error) {
	names, confidences := b.scoredRivals(c)
	if len(names) == 0 {
		return 0, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT ord FROM unnest($1::text[]) WITH ORDINALITY AS p(full_name, ord)
		 WHERE `+contactNameAdmitsSQL("p.full_name", "$2"), names, c.FullName)
	if err != nil {
		return 0, fmt.Errorf("dedupe contact batch admission: %w", err)
	}
	admitted, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return 0, fmt.Errorf("dedupe contact batch admission: %w", err)
	}
	best := 0.0
	for _, ord := range admitted {
		best = max(best, confidences[ord-1])
	}
	return best, nil
}

// scoredRivals is BestRival's Go half: the members on a shared employer domain
// that the scorer puts over the threshold, with their confidences.
func (b *ContactBatch) scoredRivals(c ContactCandidate) ([]string, []float64) {
	if NormalizeContactName(c.FullName) == "" {
		return nil, nil
	}
	var names []string
	var confidences []float64
	for _, e := range c.Emails {
		domain := emailDomain(e)
		for _, name := range b.namesByDomain[domain] {
			row := contactCandidateRow{fullName: name, mailDomains: []string{domain}}
			if confidence := contactConfidence(c, row); confidence >= dedupeReviewThreshold {
				names = append(names, name)
				confidences = append(confidences, confidence)
			}
		}
	}
	return names, confidences
}
