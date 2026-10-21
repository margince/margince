// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/recordfiles"
)

// MergeDraft makes draft collisions visible before either record is retired.
type MergeDraft struct {
	Source crmcontracts.MailDraft
	Target *crmcontracts.MailDraft
}

// MergeContentReader keeps preview reads under the owning module’s authorization policy.
type MergeContentReader interface {
	PreviewFileCarryTx(context.Context, pgx.Tx, string, ids.UUID, ids.UUID) ([]recordfiles.CarriedFile, error)
	PreviewDraftCarryTx(context.Context, pgx.Tx, string, ids.UUID, ids.UUID) (*crmcontracts.MailDraft, *crmcontracts.MailDraft, error)
}

// WithMergeContentReader uses the owning module’s authorized reader so preview and execution share its policy.
func (s *Store) WithMergeContentReader(read MergeContentReader) *Store {
	s.mergeContentReader = read
	return s
}

// WithMergeContentReader uses the owning module’s authorized reader so preview and execution share its policy.
func (p *Provider) WithMergeContentReader(read MergeContentReader) *Provider {
	p.store.WithMergeContentReader(read)
	return p
}

// WithMergeContentReader uses the owning module’s authorized reader so preview and execution share its policy.
func (h Handlers) WithMergeContentReader(read MergeContentReader) Handlers {
	h.store.WithMergeContentReader(read)
	return h
}

func (s *Store) previewMergeContent(ctx context.Context, tx pgx.Tx, kind string, source, target ids.UUID, out *MergeSurvivorship) error {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return err
	}
	if s.mergeContentReader == nil {
		var held bool
		err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM attachment WHERE entity_type=@kind AND entity_id=@source)
 OR EXISTS (SELECT 1 FROM mail_draft WHERE anchor_type=@kind AND anchor_id=@source)`, pgx.StrictNamedArgs{recordKindParam: kind, mergeSourceKey: source}).Scan(&held)
		if err != nil {
			return err
		}
		if held {
			return apperrors.ErrInvalidArgument
		}
		return nil
	}
	files, err := s.mergeContentReader.PreviewFileCarryTx(ctx, tx, kind, source, target)
	if err != nil {
		return err
	}
	from, to, err := s.mergeContentReader.PreviewDraftCarryTx(ctx, tx, kind, source, target)
	if err != nil {
		return err
	}
	out.Files = files
	if from != nil {
		out.Draft = &MergeDraft{Source: *from, Target: to}
	}
	return nil
}
