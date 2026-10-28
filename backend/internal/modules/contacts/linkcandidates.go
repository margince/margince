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
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// linkCandidateCap bounds a candidate list and a link batch alike, so every
// batch the list offers is one the link accepts.
const linkCandidateCap = 50

// LinkCandidate is a contact on one of a company's domains who does not work
// there yet, with the address that matched.
type LinkCandidate struct {
	ContactID ids.ContactID
	FullName  string
	Title     *string
	Email     string
	Employer  *crmcontracts.ContactEmployer
}

// ListLinkCandidates answers who on this company's domains could be linked to
// it, under the caller's contact row scope, and whether more exist than the cap.
func (s *Store) ListLinkCandidates(ctx context.Context, companyID ids.CompanyID) ([]LinkCandidate, bool, error) {
	for _, object := range []string{companyEntity, contactEntity, tableRelationship} {
		if err := auth.Require(ctx, object, principal.ActionRead); err != nil {
			return nil, false, err
		}
	}
	var out []LinkCandidate
	err := s.txSnapshot(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisibleLive(ctx, tx, companyEntity, companyID.UUID); err != nil {
			return err
		}
		var err error
		out, err = s.linkCandidatesTx(ctx, tx, companyID, nil, linkCandidateCap+1)
		return err
	})
	if err != nil {
		return nil, false, err
	}
	truncated := len(out) > linkCandidateCap
	if truncated {
		out = out[:linkCandidateCap]
	}
	return out, truncated, nil
}

// linkCandidatesTx is the one reading of a link candidate. The list and the
// link share it, so the link accepts what the list offers and nothing else. A
// non-nil `only` narrows it to those contacts.
func (s *Store) linkCandidatesTx(
	ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, only []ids.UUID, limit int,
) ([]LinkCandidate, error) {
	if err := auth.Require(ctx, contactEntity, principal.ActionRead); err != nil {
		return nil, err
	}
	domains, err := s.companyMatchDomainsTx(ctx, tx, companyID)
	if err != nil || len(domains) == 0 {
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	matching := arg(domains)
	scope, err := auth.ScopeClauseFor(ctx, contactEntity, "c", arg)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = scopeAllRows
	}
	narrowed := scopeAllRows
	if only != nil {
		narrowed = storekit.SQLf("c.id = ANY($%d)", arg(only))
	}
	// The unique index refuses a link while it holds an edge. A contact whose end
	// date is still ahead works there now.
	rows, err := tx.Query(ctx, storekit.SQLf(`
		SELECT c.id, c.full_name, c.title, matched.email
		  FROM contact c
		  JOIN LATERAL (
		       SELECT ce.email FROM contact_email ce
		        WHERE ce.contact_id = c.id AND ce.archived_at IS NULL AND ce.email_domain_folded = ANY($%[1]d)`+
		ReachableEmailOrder+` LIMIT 1) matched ON true
		 WHERE c.archived_at IS NULL AND c.merged_into_id IS NULL
		   AND c.id IN (SELECT held.contact_id FROM contact_email held
		                 WHERE held.archived_at IS NULL AND held.email_domain_folded = ANY($%[1]d))
		   AND NOT EXISTS (
		       SELECT 1 FROM relationship rel
		        WHERE rel.contact_id = c.id AND rel.company_id = $%[2]d
		          AND (%[3]s OR (rel.kind = 'employment' AND rel.archived_at IS NULL AND %[4]s)))
		   AND %[5]s AND %[6]s
		 ORDER BY lower(c.full_name), c.id
		 LIMIT %[7]d`,
		matching, arg(companyID), employment.LiveSlotSQL("rel"), employment.IsCurrentSQL("rel.ended_at"),
		scope, narrowed, limit), args...)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading link candidates: %w", err)
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (LinkCandidate, error) {
		var c LinkCandidate
		return c, row.Scan(&c.ContactID, &c.FullName, &c.Title, &c.Email)
	})
	if err != nil {
		return nil, fmt.Errorf("contacts: reading link candidates: %w", err)
	}
	return out, attachCandidateEmployers(ctx, tx, out)
}

// companyMatchDomainsTx is the company's live folded domains a contact address
// may match. Consumer mail never matches, even on a company that carries it.
func (s *Store) companyMatchDomainsTx(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID) ([]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT domain_folded FROM company_domain
		 WHERE company_id = $1 AND archived_at IS NULL AND domain_folded IS NOT NULL`, companyID)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading a company's domains: %w", err)
	}
	folded, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("contacts: reading a company's domains: %w", err)
	}
	matcher, err := s.consumerMailMatcher(ctx, tx)
	if err != nil {
		return nil, err
	}
	out := folded[:0]
	for _, domain := range folded {
		if !matcher.IsConsumer(domain) {
			out = append(out, domain)
		}
	}
	return out, nil
}

func attachCandidateEmployers(ctx context.Context, tx pgx.Tx, candidates []LinkCandidate) error {
	contactIDs := make([]ids.UUID, 0, len(candidates))
	for _, c := range candidates {
		contactIDs = append(contactIDs, c.ContactID.UUID)
	}
	employers, err := readContactEmployers(ctx, tx, contactIDs)
	if err != nil {
		return err
	}
	for i := range candidates {
		if employer, ok := employers[candidates[i].ContactID.UUID]; ok {
			candidates[i].Employer = &employer
		}
	}
	return nil
}
