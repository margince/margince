// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// MergeObservation keeps observation provenance available when relationships move.
type MergeObservation struct {
	Relationship crmcontracts.Relationship
	Before       *time.Time
	After        time.Time
}

func (s *Store) previewMergeObservations(ctx context.Context, tx pgx.Tx, kind string, source, target ids.UUID) ([]MergeObservation, error) {
	column := contactFK
	if kind == companyEntity {
		column = companyFK
	}
	rows, err := tx.Query(ctx, "SELECT f.id, f.was, f.earliest FROM ("+mergeObservationCandidates(column)+") f WHERE "+mergeObservationChanges+" ORDER BY f.id LIMIT 501", pgx.StrictNamedArgs{mergeSourceKey: source, mergeTargetKey: target})
	if err != nil {
		return nil, err
	}
	type change struct {
		ID     ids.UUID
		Before *time.Time
		After  time.Time
	}
	changes, err := pgx.CollectRows(rows, pgx.RowToStructByPos[change])
	if err != nil {
		return nil, err
	}
	if len(changes) > 500 {
		return nil, apperrors.ErrInvalidArgument
	}
	if len(changes) == 0 {
		return nil, nil
	}
	if err := auth.Require(ctx, "relationship", principal.ActionRead); err != nil {
		return nil, err
	}
	if err := previewFieldsVisible(ctx, "relationship", fieldFirstObservedAt); err != nil {
		return nil, err
	}
	var out []MergeObservation
	for _, change := range changes {
		edge, err := s.visibleRelationship(ctx, tx, change.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, MergeObservation{Relationship: wireRelationship(edge), Before: change.Before, After: change.After})
	}
	return out, nil
}
