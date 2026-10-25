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
