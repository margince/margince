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
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// openDealSQL is what makes a deal a candidate: open and not archived.
const openDealSQL = `d.status = 'open' AND d.archived_at IS NULL`

// OpenDealsOfProject answers up to two open deals of one project.
func (s *Store) OpenDealsOfProject(ctx context.Context, tx pgx.Tx, projectID ids.UUID) ([]ids.UUID, error) {
	if denied, err := candidateReadDenied(ctx); denied || err != nil {
		return nil, err
	}
	return collectCandidates(ctx, tx, `SELECT d.id FROM deal d WHERE d.project_id = $1 AND `+openDealSQL+`
		ORDER BY d.id LIMIT 2`, projectID)
}

// OpenDealsOfCompany answers up to two open deals of one company.
func (s *Store) OpenDealsOfCompany(ctx context.Context, tx pgx.Tx, companyID ids.UUID) ([]ids.UUID, error) {
	if denied, err := candidateReadDenied(ctx); denied || err != nil {
		return nil, err
	}
	return collectCandidates(ctx, tx, `SELECT d.id FROM deal d WHERE d.company_id = $1 AND `+openDealSQL+`
		ORDER BY d.id LIMIT 2`, companyID)
}

// OpenDealsOfCompanyUpTo answers up to limit open deals of one company, the
// candidates a deal topic check weighs.
func (s *Store) OpenDealsOfCompanyUpTo(ctx context.Context, tx pgx.Tx, companyID ids.UUID, limit int) ([]ids.UUID, error) {
	if denied, err := candidateReadDenied(ctx); denied || err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	query := `SELECT d.id FROM deal d WHERE d.company_id = ` + arg(companyID) + ` AND ` + openDealSQL + ` ORDER BY d.id LIMIT ` + arg(limit)
	return collectCandidates(ctx, tx, query, args...)
}

// IsOpenDealOfCompanies answers whether deal is an open deal of one of the
// companies. A seat without the deal read grant learns no deal.
func (s *Store) IsOpenDealOfCompanies(ctx context.Context, tx pgx.Tx, deal ids.UUID, companies []ids.UUID) (bool, error) {
	if denied, err := candidateReadDenied(ctx); denied || err != nil {
		return false, err
	}
	var open bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM deal d
		 WHERE d.id = $1 AND d.company_id = ANY($2) AND `+openDealSQL+`)`, deal, companies).Scan(&open); err != nil {
		return false, fmt.Errorf("deals: checking the chosen deal of a logged activity: %w", err)
	}
	return open, nil
}

// OpenDealsWithStakeholders answers up to two open deals of one company on
// which one of contacts is a live stakeholder. A caller without the edge grant
// learns no stakeholder and gets none.
func (s *Store) OpenDealsWithStakeholders(
	ctx context.Context, tx pgx.Tx, companyID ids.UUID, contacts []ids.UUID,
) ([]ids.UUID, error) {
	if denied, err := candidateReadDenied(ctx); denied || err != nil {
		return nil, err
	}
	args := []any{companyID, contacts}
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
	return collectCandidates(ctx, tx, `SELECT d.id FROM deal d
		 WHERE d.company_id = $1 AND `+openDealSQL+`
		   AND EXISTS (SELECT 1 FROM relationship r
		                WHERE r.deal_id = d.id AND r.kind = 'deal_stakeholder'
		                  AND r.contact_id = ANY($2) AND r.archived_at IS NULL
		                  AND (`+edgeScope+`))
		 ORDER BY d.id LIMIT 2`, args...)
}

// maxOfferedDeals bounds the deals one deal question records as offered.
const maxOfferedDeals = 50

// OfferedDeals answers the open deals a deal question offers: those of the
// project when it names one, else those of the company.
func (s *Store) OfferedDeals(ctx context.Context, tx pgx.Tx, projectID, companyID ids.UUID) ([]ids.UUID, error) {
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
