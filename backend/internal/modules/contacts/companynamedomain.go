// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// A company whose display name is a web domain claims that domain for the
// dedupe ladder. Capture mints a company from mail under its domain, and the
// same business is often already filed under its name with the domain claimed
// there. The pair goes to the review queue from whichever side arrives second:
// the ladder asks when a company is created or renamed under such a name
// (nameDomainClaimant), and fileNameDomainTwins asks when a domain is claimed.
// Only the whole domain counts. Its first label is not compared with other
// names, because unrelated businesses share words.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// displayNameDomain answers the domain a display name spells, normalized the
// way company_domain stores one. A name with a space or no dot is not a domain.
func displayNameDomain(name string) (string, bool) {
	domain, err := values.ParseDomain(name)
	if err != nil {
		return "", false
	}
	return domain.String(), true
}

// rankNameDomainClaimantFirst puts the domain's claimant at the head of the
// ladder's answer. The match stays a review: every caller that files a fuzzy
// pair files this one, and none routes a record onto it.
func rankNameDomainClaimantFirst(match CompanyMatch, claimants []CompanyCandidateScore) CompanyMatch {
	if len(claimants) == 0 {
		return match
	}
	claimant := claimants[0]
	ranked := []CompanyCandidateScore{claimant}
	for _, scored := range match.Ranked {
		if scored.CompanyID == claimant.CompanyID {
			ranked[0].ExactName = scored.ExactName
			continue
		}
		ranked = append(ranked, scored)
	}
	return CompanyMatch{
		Decision:   DecisionFuzzyReview,
		CompanyID:  claimant.CompanyID,
		Confidence: claimant.Confidence,
		Ranked:     ranked,
	}
}

// fileNameDomainTwins puts on the review queue every live company whose
// display name spells a domain the claimant has just claimed.
//
// The LIKE narrows on the display-name trigram index; displayNameDomain
// decides, so "shop.acme.example" is not a twin of acme.example. The name lock
// is the one renames take, so a rename onto the domain and the claim of it
// cannot both commit unseen by the other.
func fileNameDomainTwins(ctx context.Context, tx pgx.Tx, claimant ids.CompanyID, domain, source, by string) error {
	if err := lockCompanyNameWrites(ctx, tx); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `
		SELECT id, display_name FROM company
		 WHERE archived_at IS NULL AND NOT is_anchor AND id <> $1
		   AND f_fold_apostrophes(lower(display_name)) LIKE '%' || $2 || '%'`,
		claimant, domain)
	if err != nil {
		return fmt.Errorf("contacts: reading companies named after a claimed domain: %w", err)
	}
	type twin struct {
		id   ids.CompanyID
		name string
	}
	candidates, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (twin, error) {
		var t twin
		err := row.Scan(&t.id, &t.name)
		return t, err
	})
	if err != nil {
		return fmt.Errorf("contacts: reading companies named after a claimed domain: %w", err)
	}
	for _, candidate := range candidates {
		if named, ok := displayNameDomain(candidate.name); !ok || named != domain {
			continue
		}
		if err := recordNearMatch(ctx, tx, entityCompany, claimant.UUID, candidate.id.UUID,
			identityConflictConfidence,
			nearMatchEvidence(laneDomain, domain, candidate.name, identityConflictConfidence),
			source, by); err != nil {
			return err
		}
	}
	return nil
}
