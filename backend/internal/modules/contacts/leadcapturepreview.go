// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CapturedLeadFoldPreview keeps capture effects visible before an existing lead absorbs them.
type CapturedLeadFoldPreview struct {
	Current crmcontracts.Lead
	Patch   UpdateLeadInput
}

// PreviewCapturedLeadFold resolves authorized effects without applying the proposed change.
func (s *Store) PreviewCapturedLeadFold(ctx context.Context, id ids.LeadID, in CapturedLeadFields, version *int64) (CapturedLeadFoldPreview, error) {
	var out CapturedLeadFoldPreview
	if err := auth.Require(ctx, leadEntity, principal.ActionRead); err != nil {
		return out, err
	}
	if err := auth.Require(ctx, leadEntity, principal.ActionUpdate); err != nil {
		return out, err
	}
	if err := previewFieldsVisible(ctx, leadEntity, contactNameColumn, "company_name", "title"); err != nil {
		return out, err
	}
	active, err := s.activeColumns(ctx, leadEntity)
	if err != nil {
		return out, err
	}
	err = s.txSnapshot(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureWritableLive(ctx, tx, leadEntity, id.UUID); err != nil {
			return err
		}
		if err := storekit.EnsureIfMatch(ctx, tx, leadEntity, id.UUID, version); err != nil {
			return err
		}
		current, err := readLead(ctx, tx, id, storekit.LiveOnly, active)
		if err != nil {
			return err
		}
		out = CapturedLeadFoldPreview{Current: current, Patch: emptyFieldPatch(current, in)}
		return nil
	})
	if err != nil {
		return CapturedLeadFoldPreview{}, err
	}
	return out, nil
}
