// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MergeCommitmentReader keeps preview reads under the owning module’s authorization policy.
type MergeCommitmentReader func(context.Context, pgx.Tx, string, ids.UUID) ([]crmcontracts.WeeklyPlanCommitment, error)

// WithMergeCommitmentReader uses the owning module’s authorized reader so preview and execution share its policy.
func (s *Store) WithMergeCommitmentReader(read MergeCommitmentReader) *Store {
	s.mergeCommitmentReader = read
	return s
}

// WithMergeCommitmentReader uses the owning module’s authorized reader so preview and execution share its policy.
func (h Handlers) WithMergeCommitmentReader(read MergeCommitmentReader) Handlers {
	h.store.WithMergeCommitmentReader(read)
	return h
}

func (s *Store) previewMergeCommitments(ctx context.Context, tx pgx.Tx, kind string, source ids.UUID) ([]crmcontracts.WeeklyPlanCommitment, error) {
	return readMergeDependencies(ctx, tx, kind, source, s.mergeCommitmentReader, `SELECT EXISTS (SELECT 1 FROM weekly_plan_commitment WHERE linked_record_type=@kind AND linked_record_id=@source)`)
}
