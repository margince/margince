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

type MergeSignalReader func(context.Context, pgx.Tx, string, ids.UUID) ([]crmcontracts.Signal, error)

func (s *Store) WithMergeSignalReader(read MergeSignalReader) *Store {
	s.mergeSignalReader = read
	return s
}
func (p *Provider) WithMergeSignalReader(read MergeSignalReader) *Provider {
	p.store.WithMergeSignalReader(read)
	return p
}
func (h Handlers) WithMergeSignalReader(read MergeSignalReader) Handlers {
	h.store.WithMergeSignalReader(read)
	return h
}

func (s *Store) previewMergeSignals(ctx context.Context, tx pgx.Tx, kind string, source ids.UUID) ([]crmcontracts.Signal, error) {
	if s.mergeSignalReader != nil {
		return s.mergeSignalReader(ctx, tx, kind, source)
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM signal WHERE entity_type=@kind AND entity_id=@source)`, pgx.StrictNamedArgs{"kind": kind, "source": source}).Scan(&exists); err != nil {
		return nil, err
	}
	if exists {
		return nil, apperrors.ErrInvalidArgument
	}
	return []crmcontracts.Signal{}, nil
}
