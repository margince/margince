// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The manual source proves direct entry after provider claim and fill ledgers retire.
const unknownEmploymentCandidate = `r.kind = 'employment' AND r.employment_status = 'unknown'
 AND r.archived_at IS NULL AND r.captured_by NOT LIKE 'connector:%' AND r.source = 'manual'
 AND EXISTS (SELECT 1 FROM contact c WHERE c.id = r.contact_id AND c.archived_at IS NULL)
 AND EXISTS (SELECT 1 FROM company c WHERE c.id = r.company_id AND c.archived_at IS NULL)
 AND NOT EXISTS (SELECT 1 FROM provider_employment_resolution p WHERE p.relationship_id = r.id)`

// UnknownEmploymentPage advances past inspected rows even when a link must remain unchanged.
type UnknownEmploymentPage struct {
	Scanned []ids.UUID
	Changed int
}

// RepairUnknownEmploymentTx preserves import evidence while correcting manual employment links.
func (s *Store) RepairUnknownEmploymentTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int, runID string) (UnknownEmploymentPage, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return UnknownEmploymentPage{}, err
	}
	if err := auth.Require(ctx, "relationship", principal.ActionUpdate); err != nil {
		return UnknownEmploymentPage{}, err
	}
	rows, err := tx.Query(ctx, `SELECT r.id FROM relationship r WHERE r.id > @after_relationship AND `+unknownEmploymentCandidate+` ORDER BY r.id LIMIT @repair_page_limit`, pgx.NamedArgs{"after_relationship": after, "repair_page_limit": limit})
	if err != nil {
		return UnknownEmploymentPage{}, err
	}
	candidates, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return UnknownEmploymentPage{}, err
	}
	page := UnknownEmploymentPage{}
	for _, candidate := range candidates {
		page.Scanned = append(page.Scanned, candidate)
		changed := false
		err := pgx.BeginFunc(ctx, tx, func(savepoint pgx.Tx) error {
			var err error
			changed, err = s.repairUnknownEmploymentLinkTx(ctx, savepoint, candidate, runID)
			return err
		})
		if errors.Is(err, apperrors.ErrNotFound) {
			continue
		}
		if err != nil {
			return UnknownEmploymentPage{}, err
		}
		if changed {
			page.Changed++
		}
	}
	return page, nil
}

func (s *Store) repairUnknownEmploymentLinkTx(ctx context.Context, tx pgx.Tx, candidate ids.UUID, runID string) (bool, error) {
	if err := lockContactForEmployment(ctx, tx, candidate); err != nil {
		return false, err
	}
	var capturedBy string
	var version int64
	var current bool
	// The identity lock serializes slot checks with every employment writer.
	err := tx.QueryRow(ctx, `SELECT r.captured_by,r.version,`+employment.IsCurrentSQL("r.ended_at", "'current'", "r.ended_precision")+` FROM relationship r WHERE r.id = @id AND `+unknownEmploymentCandidate+`
 AND (r.ended_at IS NOT NULL OR NOT EXISTS (SELECT 1 FROM relationship other
 WHERE other.contact_id = r.contact_id AND other.company_id = r.company_id AND other.id <> r.id AND `+employment.LiveSlotSQL("other")+`)) FOR UPDATE OF r`, pgx.NamedArgs{"id": candidate}).Scan(&capturedBy, &version, &current)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	status := "former"
	if current {
		status = "current"
	}
	_, err = s.updateRelationshipInTx(ctx, tx, candidate, UpdateRelationshipInput{EmploymentStatus: &status, IfVersion: &version, Evidence: map[string]any{"data_task_run": runID, auditKeyReason: "manual_employment_status_follows_dates"}}, capturedBy)
	return err == nil, err
}

// CountUnknownEmploymentLinks bounds the estimate with the writer's candidate rule.
func CountUnknownEmploymentLinks(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, maximum int64,
) (int64, error) {
	if err := auth.Require(ctx, "relationship", principal.ActionRead); err != nil {
		return 0, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM relationship r WHERE `+unknownEmploymentCandidate+` LIMIT @estimate_limit) candidates`, pgx.NamedArgs{"estimate_limit": maximum + 1}).Scan(&count)
	return count, err
}
