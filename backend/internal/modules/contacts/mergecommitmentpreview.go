// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"github.com/jackc/pgx/v5"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type MergeCommitmentReader func(context.Context, pgx.Tx, string, ids.UUID) ([]crmcontracts.WeeklyPlanCommitment, error)

func (s *Store) WithMergeCommitmentReader(read MergeCommitmentReader) *Store {
	s.mergeCommitmentReader = read
	return s
}
func (p *Provider) WithMergeCommitmentReader(read MergeCommitmentReader) *Provider {
	p.store.WithMergeCommitmentReader(read)
	return p
}
func (h Handlers) WithMergeCommitmentReader(read MergeCommitmentReader) Handlers {
	h.store.WithMergeCommitmentReader(read)
	return h
}

func (s *Store) previewMergeCommitments(ctx context.Context, tx pgx.Tx, kind string, source ids.UUID) ([]crmcontracts.WeeklyPlanCommitment, error) {
	if s.mergeCommitmentReader != nil {
		return s.mergeCommitmentReader(ctx, tx, kind, source)
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM weekly_plan_commitment WHERE linked_record_type=@kind AND linked_record_id=@source)`, pgx.StrictNamedArgs{"kind": kind, "source": source}).Scan(&exists); err != nil {
		return nil, err
	}
	if exists {
		return nil, apperrors.ErrInvalidArgument
	}
	return []crmcontracts.WeeklyPlanCommitment{}, nil
}
