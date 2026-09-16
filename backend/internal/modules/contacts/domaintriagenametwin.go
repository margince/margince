// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// adoptOrHoldForNameTwin decides what a domain does about a company that
// already carries its name.
//
// Three outcomes, and the middle one is the whole point:
//
//   - Exactly ONE company has the same name → the domain joins it. Two domains
//     of one business resolve to one label (the product brand and the corporate
//     site, the country domains), and minting a second record for the second
//     domain is what put one company in a workspace twice.
//   - A close name, or SEVERAL companies with the same name → nothing is
//     created and the question is held for a human. "Baqend GmbH" and "Baqend
//     Inc" score alike and are two different legal entities, and picking among
//     several exact twins by rank would be choosing by uuid.
//   - No twin at all → the caller creates, as before.
//
// It reports the adopted company, or that the question was held. Both nil means
// carry on and create.
func (s *Store) adoptOrHoldForNameTwin(
	ctx context.Context, tx pgx.Tx, in ResolveDomainTriageInput,
	match CompanyMatch, displayName, by string,
) (adopted *ids.CompanyID, held bool, err error) {
	twins := exactNameRivals(match)
	if len(twins) == 1 {
		id, err := s.adoptDomainIntoCompany(ctx, tx, in, twins[0], displayName, by)
		return id, false, err
	}
	// Several exact twins, or a near-match the fuzzy tier already flagged.
	// Either way the machine cannot tell which company this domain belongs to.
	candidate, ok := heldRival(match, twins)
	if !ok {
		return nil, false, nil
	}
	if err := withholdForNearDuplicate(ctx, tx, in, candidate); err != nil {
		return nil, false, err
	}
	return nil, true, nil
}

// exactNameRivals is every ranked company whose name IS this one, folded.
//
// Read off the ranked list rather than the single best score: the winner is
// chosen on confidence alone, and a near-match can outrank — or tie and
// displace — the company that actually shares the name.
func exactNameRivals(match CompanyMatch) []CompanyCandidateScore {
	var out []CompanyCandidateScore
	for _, score := range match.Ranked {
		if score.ExactName {
			out = append(out, score)
		}
	}
	return out
}

// heldRival names the company a held question should point a human at, and
// reports whether the question is held at all.
//
// Several exact twins are held on the first of them; a fuzzy review with no
// exact twin is held on its best-scoring rival. Anything else creates.
func heldRival(match CompanyMatch, twins []CompanyCandidateScore) (CompanyCandidateScore, bool) {
	if len(twins) > 1 {
		return twins[0], true
	}
	if match.Decision == DecisionFuzzyReview && len(match.Ranked) > 0 {
		return match.Ranked[0], true
	}
	return CompanyCandidateScore{}, false
}

// adoptDomainIntoCompany gives an existing company this domain instead of
// minting a second record for it.
//
// The domain lands as a SECONDARY one: the company's primary domain is the one
// it was created with, and a later domain arriving from a crawl has no claim to
// displace it. No unclaimed probe is needed — the exact-domain tier just proved
// no live company holds this domain, and uq_company_domain is the structural
// guarantee under a race.
func (s *Store) adoptDomainIntoCompany(
	ctx context.Context, tx pgx.Tx, in ResolveDomainTriageInput,
	twin CompanyCandidateScore, displayName, by string,
) (*ids.CompanyID, error) {
	companyID := twin.CompanyID
	if err := insertCompanyDomains(ctx, tx, companyID, domainTriageSource(in.Domain), by,
		[]CompanyDomainInput{{Domain: in.Domain, IsPrimary: false}}); err != nil {
		return nil, err
	}
	auditID, err := storekit.Audit(ctx, tx, "update", entityCompany, companyID.UUID, nil, map[string]any{
		auditKeyDomain: in.Domain, fieldDisplayName: displayName,
	})
	if err != nil {
		return nil, err
	}
	// The company gained a domain, which is a change to the record and reaches
	// its readers as one. A create event would be a lie — nothing was created —
	// and silence would leave every consumer of company.updated with a company
	// whose domains it cannot know changed.
	if err := storekit.EmitEvent(ctx, tx, auditID, companyID.UUID,
		crmcontracts.PublicEventCompanyUpdated{ChangedFields: map[string]any{"domains": in.Domain}}); err != nil {
		return nil, err
	}
	return &companyID, nil
}
