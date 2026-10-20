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

type MergeNoticeReader func(context.Context, pgx.Tx, string, ids.UUID) ([]crmcontracts.NotificationItem, error)

func (s *Store) WithMergeNoticeReader(read MergeNoticeReader) *Store {
	s.mergeNoticeReader = read
	return s
}
func (p *Provider) WithMergeNoticeReader(read MergeNoticeReader) *Provider {
	p.store.WithMergeNoticeReader(read)
	return p
}
func (h Handlers) WithMergeNoticeReader(read MergeNoticeReader) Handlers {
	h.store.WithMergeNoticeReader(read)
	return h
}

func (s *Store) previewMergeNotices(ctx context.Context, tx pgx.Tx, kind string, source ids.UUID) ([]crmcontracts.NotificationItem, error) {
	if s.mergeNoticeReader != nil {
		return s.mergeNoticeReader(ctx, tx, kind, source)
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM notice WHERE target_type=@kind AND target_id=@source)`, pgx.StrictNamedArgs{"kind": kind, "source": source}).Scan(&exists); err != nil {
		return nil, err
	}
	if exists {
		return nil, apperrors.ErrInvalidArgument
	}
	return []crmcontracts.NotificationItem{}, nil
}
