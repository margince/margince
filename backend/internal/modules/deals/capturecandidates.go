// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The open deals the capture rule chooses a mail's deal among
// (capture.DealCandidates), at most two per read.
//
// The reads count every open deal, not only those in the caller's row scope.
// A seat that sees one of two deals has not learned which one a mail is about.
// The link the rule writes still passes auth.EnsureLinkTarget.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/dealseat"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// openDealSQL is what makes a deal a candidate: open and not archived.
const openDealSQL = `d.status = 'open' AND d.archived_at IS NULL`

// candidateDealSQL is openDealSQL, or with a non-zero only the one named deal
// that is not archived, whatever its status.
func candidateDealSQL(only ids.UUID, arg func(any) int) string {
	if only.IsZero() {
		return openDealSQL
	}
	return fmt.Sprintf(`(d.id = $%d AND d.archived_at IS NULL)`, arg(only))
}

// CandidatesOnly reads candidates as the open-deal reads do, but answers no
// deal other than the one named. The capture repair files old mail on a deal
// Margince closed for silence through it, so a deal opened since cannot take
// that mail.
func (s *Store) CandidatesOnly(deal ids.UUID) *ClosedDealCandidates {
	return &ClosedDealCandidates{store: s, admit: deal}
}

// ClosedDealCandidates is capture.DealCandidates narrowed to one deal.
type ClosedDealCandidates struct {
	store *Store
	admit ids.UUID
}

// OpenDealsOfProject answers up to two open deals of one project.
func (s *Store) OpenDealsOfProject(ctx context.Context, tx pgx.Tx, projectID ids.UUID) ([]ids.UUID, error) {
	return dealsOfProject(ctx, tx, projectID, ids.Nil)
}

// OpenDealsOfProject is Store.OpenDealsOfProject narrowed to its one deal.
func (c *ClosedDealCandidates) OpenDealsOfProject(ctx context.Context, tx pgx.Tx, projectID ids.UUID) ([]ids.UUID, error) {
	return dealsOfProject(ctx, tx, projectID, c.admit)
}

func dealsOfProject(ctx context.Context, tx pgx.Tx, projectID, admit ids.UUID) ([]ids.UUID, error) {
	if denied, err := candidateReadDenied(ctx); denied || err != nil {
		return nil, err
	}
	args := []any{projectID}
	arg := func(v any) int { args = append(args, v); return len(args) }
	query := `SELECT d.id FROM deal d WHERE d.project_id = $1 AND ` + candidateDealSQL(admit, arg) + `
		ORDER BY d.id LIMIT 2`
	return collectCandidates(ctx, tx, query, args...)
}

// OpenDealsOfCompany answers up to two open deals of one company.
func (s *Store) OpenDealsOfCompany(ctx context.Context, tx pgx.Tx, companyID ids.UUID) ([]ids.UUID, error) {
	return dealsOfCompany(ctx, tx, companyID, ids.Nil)
}

// OpenDealsOfCompany is Store.OpenDealsOfCompany narrowed to its one deal.
func (c *ClosedDealCandidates) OpenDealsOfCompany(ctx context.Context, tx pgx.Tx, companyID ids.UUID) ([]ids.UUID, error) {
	return dealsOfCompany(ctx, tx, companyID, c.admit)
}

func dealsOfCompany(ctx context.Context, tx pgx.Tx, companyID, admit ids.UUID) ([]ids.UUID, error) {
	if denied, err := candidateReadDenied(ctx); denied || err != nil {
		return nil, err
	}
	args := []any{companyID}
	arg := func(v any) int { args = append(args, v); return len(args) }
	query := `SELECT d.id FROM deal d WHERE d.company_id = $1 AND ` + candidateDealSQL(admit, arg) + `
		ORDER BY d.id LIMIT 2`
	return collectCandidates(ctx, tx, query, args...)
}

// OpenDealsOfCompanyUpTo answers up to limit open deals of one company, the
// candidates a deal topic check weighs.
func (s *Store) OpenDealsOfCompanyUpTo(ctx context.Context, tx pgx.Tx, companyID ids.UUID, limit int) ([]ids.UUID, error) {
	return dealsOfCompanyUpTo(ctx, tx, companyID, limit, ids.Nil)
}

// OpenDealsOfCompanyUpTo is Store.OpenDealsOfCompanyUpTo narrowed to its one deal.
func (c *ClosedDealCandidates) OpenDealsOfCompanyUpTo(ctx context.Context, tx pgx.Tx, companyID ids.UUID, limit int) ([]ids.UUID, error) {
	return dealsOfCompanyUpTo(ctx, tx, companyID, limit, c.admit)
}

func dealsOfCompanyUpTo(ctx context.Context, tx pgx.Tx, companyID ids.UUID, limit int, admit ids.UUID) ([]ids.UUID, error) {
	if denied, err := candidateReadDenied(ctx); denied || err != nil {
		return nil, err
	}
	var args []any
	argPos := func(v any) int { args = append(args, v); return len(args) }
	query := fmt.Sprintf(`SELECT d.id FROM deal d WHERE d.company_id = $%d AND %s ORDER BY d.id LIMIT $%d`,
		argPos(companyID), candidateDealSQL(admit, argPos), argPos(limit))
	return collectCandidates(ctx, tx, query, args...)
}

// IsOpenDealOfCompanies answers whether deal is an open deal of one of the
// companies. A seat without the deal read grant learns no deal.
func (s *Store) IsOpenDealOfCompanies(ctx context.Context, tx pgx.Tx, deal ids.UUID, companies []ids.UUID) (bool, error) {
	return isDealOfCompanies(ctx, tx, deal, companies, ids.Nil)
}

// IsOpenDealOfCompanies is Store.IsOpenDealOfCompanies narrowed to its one deal.
func (c *ClosedDealCandidates) IsOpenDealOfCompanies(ctx context.Context, tx pgx.Tx, deal ids.UUID, companies []ids.UUID) (bool, error) {
	return isDealOfCompanies(ctx, tx, deal, companies, c.admit)
}

func isDealOfCompanies(ctx context.Context, tx pgx.Tx, deal ids.UUID, companies []ids.UUID, admit ids.UUID) (bool, error) {
	if denied, err := candidateReadDenied(ctx); denied || err != nil {
		return false, err
	}
	args := []any{deal, companies}
	arg := func(v any) int { args = append(args, v); return len(args) }
	query := `SELECT EXISTS (SELECT 1 FROM deal d
		 WHERE d.id = $1 AND d.company_id = ANY($2) AND ` + candidateDealSQL(admit, arg) + `)`
	var open bool
	if err := tx.QueryRow(ctx, query, args...).Scan(&open); err != nil {
		return false, fmt.Errorf("deals: checking the chosen deal of a logged activity: %w", err)
	}
	return open, nil
}

// IsOpenDealOfProject answers whether deal is an open deal of the project. A
// seat without the deal read grant learns no deal.
func (s *Store) IsOpenDealOfProject(ctx context.Context, tx pgx.Tx, deal, project ids.UUID) (bool, error) {
	return isDealOfProject(ctx, tx, deal, project, ids.Nil)
}

// IsOpenDealOfProject is Store.IsOpenDealOfProject narrowed to its one deal.
func (c *ClosedDealCandidates) IsOpenDealOfProject(ctx context.Context, tx pgx.Tx, deal, project ids.UUID) (bool, error) {
	return isDealOfProject(ctx, tx, deal, project, c.admit)
}

func isDealOfProject(ctx context.Context, tx pgx.Tx, deal, project, admit ids.UUID) (bool, error) {
	if denied, err := candidateReadDenied(ctx); denied || err != nil {
		return false, err
	}
	args := []any{deal, project}
	arg := func(v any) int { args = append(args, v); return len(args) }
	query := `SELECT EXISTS (SELECT 1 FROM deal d
		 WHERE d.id = $1 AND d.project_id = $2 AND ` + candidateDealSQL(admit, arg) + `)`
	var open bool
	if err := tx.QueryRow(ctx, query, args...).Scan(&open); err != nil {
		return false, fmt.Errorf("deals: checking a hook's deal against the project: %w", err)
	}
	return open, nil
}

// OpenDealsWithStakeholders answers up to two open deals of one company on
// which one of contacts is a live stakeholder. A caller without the edge grant
// learns no stakeholder and gets none.
func (s *Store) OpenDealsWithStakeholders(
	ctx context.Context, tx pgx.Tx, companyID ids.UUID, contacts []ids.UUID,
) ([]ids.UUID, error) {
	return openDealsStaked(ctx, tx, &companyID, contacts, 2, ids.Nil)
}

// OpenDealsWithStakeholders is Store.OpenDealsWithStakeholders narrowed to its one deal.
func (c *ClosedDealCandidates) OpenDealsWithStakeholders(
	ctx context.Context, tx pgx.Tx, companyID ids.UUID, contacts []ids.UUID,
) ([]ids.UUID, error) {
	return openDealsStaked(ctx, tx, &companyID, contacts, 2, c.admit)
}

// OpenDealsOfStakeholders answers up to limit open deals, at any company, on
// which one of contacts is a live stakeholder. A consultant's seat on a client's
// deal links the mail to that deal, which their employer does not.
func (s *Store) OpenDealsOfStakeholders(ctx context.Context, tx pgx.Tx, contacts []ids.UUID, limit int) ([]ids.UUID, error) {
	return openDealsStaked(ctx, tx, nil, contacts, limit, ids.Nil)
}

// OpenDealsOfStakeholders is Store.OpenDealsOfStakeholders narrowed to its one deal.
func (c *ClosedDealCandidates) OpenDealsOfStakeholders(ctx context.Context, tx pgx.Tx, contacts []ids.UUID, limit int) ([]ids.UUID, error) {
	return openDealsStaked(ctx, tx, nil, contacts, limit, c.admit)
}

// openDealsStaked reads the deals a seat of one of contacts sits on, narrowed
// to company when one is given and to limit rows.
func openDealsStaked(ctx context.Context, tx pgx.Tx, company *ids.UUID, contacts []ids.UUID, limit int, admit ids.UUID) ([]ids.UUID, error) {
	if denied, err := candidateReadDenied(ctx); denied || err != nil {
		return nil, err
	}
	args := []any{contacts}
	arg := func(v any) int { args = append(args, v); return len(args) }
	edgeScope, err := auth.EdgeReadScope(ctx, "r", arg)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if edgeScope == "" {
		edgeScope = predicateAlways
	}
	companyClause := ""
	if company != nil {
		companyClause = fmt.Sprintf(" AND d.company_id = $%d", arg(*company))
	}
	limitPos := fmt.Sprintf("$%d", arg(limit))
	query := `SELECT d.id FROM deal d
		 WHERE ` + candidateDealSQL(admit, arg) + companyClause + `
		   AND EXISTS (SELECT 1 FROM relationship r
		                WHERE r.deal_id = d.id AND ` + dealseat.CurrentSQL("r") + `
		                  AND r.contact_id = ANY($1)
		                  AND (` + edgeScope + `))
		 ORDER BY d.id LIMIT ` + limitPos
	return collectCandidates(ctx, tx, query, args...)
}

// maxOfferedDeals bounds the deals one deal question records as offered.
const maxOfferedDeals = 50

// OfferedDeals answers the open deals a deal question offers. They are the
// project's when it names one, else the company's. With neither, they are the
// deals the question's stakeholder contacts hold a seat on.
func (s *Store) OfferedDeals(ctx context.Context, tx pgx.Tx, projectID, companyID ids.UUID, stakeholders []ids.UUID) ([]ids.UUID, error) {
	if len(stakeholders) > 0 && projectID.IsZero() && companyID.IsZero() {
		return s.OpenDealsOfStakeholders(ctx, tx, stakeholders, maxOfferedDeals)
	}
	if denied, err := candidateReadDenied(ctx); denied || err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	owner := `d.company_id = ` + arg(companyID)
	if !projectID.IsZero() {
		args = nil
		owner = `d.project_id = ` + arg(projectID)
	}
	query := `SELECT d.id FROM deal d WHERE ` + owner + ` AND ` + openDealSQL + ` ORDER BY d.id LIMIT ` + arg(maxOfferedDeals)
	return collectCandidates(ctx, tx, query, args...)
}

// candidateReadDenied answers whether the caller may not read deals at all. A
// denial is no fault: the rule then files the mail under no deal.
func candidateReadDenied(ctx context.Context) (bool, error) {
	err := auth.Require(ctx, "deal", principal.ActionRead)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return true, nil
	}
	return false, err
}

func collectCandidates(ctx context.Context, tx pgx.Tx, query string, args ...any) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("deals: reading a mail's candidate deals: %w", err)
	}
	found, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("deals: reading a mail's candidate deals: %w", err)
	}
	return found, nil
}
