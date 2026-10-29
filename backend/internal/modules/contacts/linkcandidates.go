// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/apperrors"
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

// ListLinkCandidates answers who on this company's domains the caller could
// link to it, and whether more exist than the cap. A caller who may not link
// anyone is offered no one.
func (s *Store) ListLinkCandidates(ctx context.Context, companyID ids.CompanyID) ([]LinkCandidate, bool, error) {
	for _, object := range []string{companyEntity, contactEntity, tableRelationship} {
		if err := auth.Require(ctx, object, principal.ActionRead); err != nil {
			return nil, false, err
		}
	}
	mayLink, err := mayLinkEmployees(ctx)
	if err != nil {
		return nil, false, err
	}
	var out []LinkCandidate
	err = s.txSnapshot(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisibleLive(ctx, tx, companyEntity, companyID.UUID); err != nil || !mayLink {
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

// mayLinkEmployees reports whether LinkCompanyContacts would admit the caller
// at all: the verbs it requires, held by a seat that may write.
func mayLinkEmployees(ctx context.Context) (bool, error) {
	if actor, ok := principal.Actor(ctx); ok && actor.SeatType == principal.SeatRead {
		return false, nil
	}
	err := requireRelationshipCreate(ctx, contactEntity)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return false, nil
	}
	return err == nil, err
}

// linkCandidatesTx is the one reading of a link candidate. The list and the
// link share it, so the link accepts what the list offers and nothing else. A
// non-nil `only` narrows it to those contacts.
//
// It matches a domain and skips workspace seats as the daily plant does. A batch
// to link is checked for seats under the plant's locks too. It asks the write
// authority the edge create asks.
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
	seats, err := storekit.WorkspaceSeatAddresses(ctx, tx)
	if err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	matching := arg(domains)
	scope, err := linkableContactClause(ctx, arg)
	if err != nil {
		return nil, err
	}
	narrowed := scopeAllRows
	if only != nil {
		narrowed = storekit.SQLf("c.id = ANY($%d)", arg(only))
	}
	// The IN arm repeats the lateral's match for the planner: it drives the read
	// from idx_contact_email_domain_reversed instead of every live contact. The
	// unique index refuses a link while it holds an edge, and a contact whose end
	// date is still ahead works there now.
	rows, err := tx.Query(ctx, storekit.SQLf(`
		SELECT c.id, c.full_name, c.title, matched.email
		  FROM contact c
		  JOIN LATERAL (
		       SELECT ce.email FROM contact_email ce
		        WHERE ce.contact_id = c.id AND ce.archived_at IS NULL
		          AND EXISTS (SELECT 1 FROM unnest($%[1]d::text[]) own(domain)
		                       WHERE `+owndomain.CoversSQL("ce.email_domain_folded", "own.domain")+`)`+
		ReachableEmailOrder+` LIMIT 1) matched ON true
		 WHERE c.archived_at IS NULL AND c.merged_into_id IS NULL
		   AND c.id IN (SELECT held.contact_id FROM contact_email held
		                  JOIN unnest($%[1]d::text[]) own(domain)
		                    ON `+owndomain.CoversIndexedSQL("held.email_domain_folded", "own.domain")+`
		                 WHERE held.archived_at IS NULL)
		   AND NOT EXISTS (
		       SELECT 1 FROM relationship rel
		        WHERE rel.contact_id = c.id AND rel.company_id = $%[2]d
		          AND (%[3]s OR (rel.kind = 'employment' AND rel.archived_at IS NULL AND %[4]s)))
		   AND %[5]s AND %[6]s AND %[7]s
		 ORDER BY lower(c.full_name), c.id
		 LIMIT %[8]d`,
		matching, arg(companyID), employment.LiveSlotSQL("rel"), employment.IsCurrentSQL("rel.ended_at"),
		storekit.HoldsNoSeatAddressSQL("c.id", seats.For(ids.Nil), arg), scope, narrowed, limit), args...)
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
	if only != nil {
		if out, err = withoutSeatsTx(ctx, tx, out); err != nil {
			return nil, err
		}
	}
	return out, attachCandidateEmployers(ctx, tx, out)
}

// withoutSeatsTx drops the workspace seats from a batch about to be linked,
// through seatContactsTx and its locks, as plantDomainEmployment does.
func withoutSeatsTx(ctx context.Context, tx pgx.Tx, candidates []LinkCandidate) ([]LinkCandidate, error) {
	contactIDs := make([]ids.ContactID, 0, len(candidates))
	for _, c := range candidates {
		contactIDs = append(contactIDs, c.ContactID)
	}
	seats, err := seatContactsTx(ctx, tx, contactIDs)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(candidates, func(c LinkCandidate) bool { return seats[c.ContactID] }), nil
}

// linkableContactClause is the contacts the caller may read and change, the
// pair of probes auth.EnsureWritableLive runs on the edge's anchor.
func linkableContactClause(ctx context.Context, arg func(any) int) (string, error) {
	readable, err := auth.ScopeClauseFor(ctx, contactEntity, "c", arg)
	if err != nil {
		return "", err
	}
	writable, err := auth.WriteAuthorityClauseFor(ctx, contactEntity, "c", arg)
	if err != nil {
		return "", err
	}
	arms := []string{scopeAllRows}
	for _, arm := range []string{readable, writable} {
		if arm != "" {
			arms = append(arms, arm)
		}
	}
	return "(" + strings.Join(arms, " AND ") + ")", nil
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
