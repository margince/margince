// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MergeNoticeReader keeps preview reads under the owning module’s authorization policy.
type MergeNoticeReader func(context.Context, pgx.Tx, string, ids.UUID) ([]crmcontracts.NotificationItem, error)

// WithMergeNoticeReader uses the owning module’s authorized reader so preview and execution share its policy.
func (s *Store) WithMergeNoticeReader(read MergeNoticeReader) *Store {
	s.mergeNoticeReader = read
	return s
}

// WithMergeNoticeReader uses the owning module’s authorized reader so preview and execution share its policy.
func (h Handlers) WithMergeNoticeReader(read MergeNoticeReader) Handlers {
	h.store.WithMergeNoticeReader(read)
	return h
}

func (s *Store) previewMergeNotices(ctx context.Context, tx pgx.Tx, kind string, source ids.UUID) ([]crmcontracts.NotificationItem, error) {
	return readMergeDependencies(ctx, tx, kind, source, s.mergeNoticeReader, `SELECT EXISTS (SELECT 1 FROM notice WHERE target_type=@kind AND target_id=@source)`)
}
