// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MergeSignalReader keeps preview reads under the owning module’s authorization policy.
type MergeSignalReader func(context.Context, pgx.Tx, string, ids.UUID) ([]crmcontracts.Signal, error)

// WithMergeSignalReader uses the owning module’s authorized reader so preview and execution share its policy.
func (s *Store) WithMergeSignalReader(read MergeSignalReader) *Store {
	s.mergeSignalReader = read
	return s
}

// WithMergeSignalReader uses the owning module’s authorized reader so preview and execution share its policy.
func (h Handlers) WithMergeSignalReader(read MergeSignalReader) Handlers {
	h.store.WithMergeSignalReader(read)
	return h
}

func (s *Store) previewMergeSignals(ctx context.Context, tx pgx.Tx, kind string, source ids.UUID) ([]crmcontracts.Signal, error) {
	return readMergeDependencies(ctx, tx, kind, source, s.mergeSignalReader, `SELECT EXISTS (SELECT 1 FROM signal WHERE entity_type=@kind AND entity_id=@source)`)
}
