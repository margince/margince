// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	contactArchiveRelationships = "(contact_id = @record OR counterparty_contact_id = @record) AND archived_at IS NULL"
	contactArchiveMemberships   = "entity_type = 'contact' AND entity_id = @record"
)

// PreviewContactArchive resolves authorized effects without applying the proposed change.
func (s *Store) PreviewContactArchive(ctx context.Context, id ids.ContactID, version *int64) (storekit.ArchiveScope, error) {
	var out storekit.ArchiveScope
	if err := auth.Require(ctx, contactEntity, principal.ActionRead); err != nil {
		return out, err
	}
	if err := auth.Require(ctx, contactEntity, principal.ActionDelete); err != nil {
		return out, err
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureWritableLive(ctx, tx, contactEntity, id.UUID); err != nil {
			return err
		}
		if err := previewFieldsVisible(ctx, contactEntity, fieldEmail, contactEmailsField, fieldPhone, contactPhonesField, "reachability"); err != nil {
			return err
		}
		if err := storekit.EnsureIfMatch(ctx, tx, contactEntity, id.UUID, version); err != nil {
			return err
		}
		var err error
		out, err = storekit.PreviewArchiveScope(ctx, tx, id.UUID, contactArchiveRelationships, contactArchiveMemberships)
		return err
	})
	if err != nil {
		return storekit.ArchiveScope{}, err
	}
	return out, nil
}
