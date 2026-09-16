// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
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
//   - SEVERAL companies with the same name → nothing is created and the
//     question is held for a human, because picking among exact twins by rank
//     would be choosing by uuid.
//   - No twin, or a merely CLOSE name → the caller creates, as before, and a
//     near-match still goes on the review queue. The fuzzy tier scores on
//     shared words, so two unrelated companies ending in the same nouns reach
//     it easily; holding on that would strand the contacts of every domain
//     whose name rhymes with an incumbent's.
//
// It reports the adopted company, or that the question was held. Both nil means
// carry on and create.
func (s *Store) adoptOrHoldForNameTwin(
	ctx context.Context, tx pgx.Tx, in ResolveDomainTriageInput,
	match CompanyMatch, by string,
) (adopted *ids.CompanyID, held bool, err error) {
	twins := exactNameRivals(match)
	if len(twins) == 1 {
		id, err := s.adoptDomainIntoCompany(ctx, tx, in, twins[0], by)
		return id, false, err
	}
	// Several companies carry this exact name, so which one this domain belongs
	// to is not something the machine can answer.
	candidate, ok := heldRival(twins)
	if !ok {
		return nil, false, nil
	}
	if err := withholdForNearDuplicate(ctx, tx, in, candidate); err != nil {
		return nil, false, err
	}
	return nil, true, nil
}

// auditKeyDomains names the domain list in an audit image, so the before and
// after of an adoption are read as one field changing rather than two facts.
const auditKeyDomains = "domains"

// liveCompanyDomains lists a company's unarchived domains, for the audit
// image an adoption writes.
func liveCompanyDomains(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID) ([]string, error) {
	rows, err := tx.Query(ctx,
		`SELECT domain FROM company_domain
		  WHERE company_id = $1 AND archived_at IS NULL
		  ORDER BY domain`, companyID)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading a company's domains: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var domain string
		if err := rows.Scan(&domain); err != nil {
			return nil, fmt.Errorf("contacts: reading a company's domains: %w", err)
		}
		out = append(out, domain)
	}
	return out, rows.Err()
}

// companyHasPrimaryDomain answers whether the record already has the one domain
// auto-enrichment reads it by.
func companyHasPrimaryDomain(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID) (bool, error) {
	var exists bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM company_domain
		   WHERE company_id = $1 AND is_primary AND archived_at IS NULL)`,
		companyID).Scan(&exists); err != nil {
		return false, fmt.Errorf("contacts: reading whether a company has a primary domain: %w", err)
	}
	return exists, nil
}

// exactNameRivals collects the ranked companies whose name folds to this one.
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
// ONLY several exact twins hold. That is the case the machine genuinely cannot
// answer: two companies carry this exact name, and choosing between them by
// rank would be choosing by uuid.
//
// A merely CLOSE name does not hold, and the distinction matters more than it
// looks. The fuzzy tier scores on shared words, so "New Employer GmbH" and
// "Incumbent Employer GmbH" reach 0.78 on "Employer GmbH" alone while being
// two unrelated companies. Holding on that would strand the contacts of every
// domain whose name happens to rhyme with an incumbent's. Those keep today's
// behaviour: the company is created and the pair goes on the review queue,
// where a human sees both records and can merge them.
func heldRival(twins []CompanyCandidateScore) (CompanyCandidateScore, bool) {
	if len(twins) > 1 {
		return twins[0], true
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
//
// The company keeps its OWN name: this domain resolved to the same one, and
// the incumbent record is the one that already carries it.
func (s *Store) adoptDomainIntoCompany(
	ctx context.Context, tx pgx.Tx, in ResolveDomainTriageInput,
	twin CompanyCandidateScore, by string,
) (*ids.CompanyID, error) {
	companyID := twin.CompanyID
	// Adopting CHANGES a company that already exists, which is a different
	// authority from the create this path was entered under. The caller's
	// ActionCreate says they may mint a record; it does not say they may write
	// a domain onto somebody else's.
	if err := auth.Require(ctx, entityCompany, principal.ActionUpdate); err != nil {
		return nil, err
	}
	// The twin came from a scan that read its own snapshot, and the adoption
	// writes to that row several statements later. The lock is what closes the
	// gap: LockRow(LiveOnly) resolves only a live row, so a company archived
	// between the scan and this write is refused here rather than handed a
	// domain. It also settles what the row HOLDS, which is what makes the audit
	// before-image below the state the write actually followed.
	//
	// EnsureWritable does not answer either question — it admits an archived
	// row, and it reads grant and team state no row lock covers. So the lock is
	// the liveness decision and the probe is the authority decision, and the
	// probe runs second for the reason companyvisibility.go gives: under READ
	// COMMITTED an unlocked check can be overtaken by a change to the row's own
	// visibility or owner before the write lands.
	if _, err := storekit.LockRow(ctx, tx, entityCompany, companyID.UUID, storekit.LiveOnly); err != nil {
		return nil, err
	}
	// The dedupe ladder scores every company in the installation, so a twin can
	// be a record outside the caller's own scope. Visibility is the wrong
	// question here — a manual read share widens it — and adopting writes a
	// domain onto the record. A miss reads as not-found rather than denied,
	// which keeps that company's existence hidden.
	if err := auth.EnsureWritable(ctx, tx, entityCompany, companyID.UUID); err != nil {
		return nil, err
	}
	// Read BEFORE the insert. The audit's before-image is what this company's
	// domains were, and reading after the write would record the answer as
	// though it were the question.
	before, err := liveCompanyDomains(ctx, tx, companyID)
	if err != nil {
		return nil, err
	}
	// Primary only when the company has none. The record's own primary domain
	// is the one it was created with and a domain arriving from a crawl has no
	// claim to displace it — but a company with NO primary domain is skipped by
	// auto-enrichment, whose query joins on is_primary, so leaving it without
	// one would quietly take it out of the enrichment it is now eligible for.
	hasPrimary, err := companyHasPrimaryDomain(ctx, tx, companyID)
	if err != nil {
		return nil, err
	}
	if err := insertCompanyDomains(ctx, tx, companyID, domainTriageSource(in.Domain), by,
		[]CompanyDomainInput{{Domain: in.Domain, IsPrimary: !hasPrimary}}); err != nil {
		return nil, err
	}
	// Adopting changes no column on the company row, so without this the record
	// looks untouched: a client holding the version from before the adoption
	// could replace the domain set and silently drop the domain just added,
	// with no conflict to notice. The same bump an ordinary domain replace-set
	// makes, for the same reason.
	if _, err := tx.Exec(ctx,
		`UPDATE company SET version = version + 1 WHERE id = $1`,
		companyID); err != nil {
		return nil, fmt.Errorf("contacts: marking the company changed by an adopted domain: %w", err)
	}
	auditID, err := storekit.Audit(ctx, tx, "update", entityCompany, companyID.UUID,
		map[string]any{auditKeyDomains: before},
		map[string]any{auditKeyDomains: append(append([]string{}, before...), in.Domain)})
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
