// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webhooks

// Where a file's visibility comes from.
//
// Its own file, and only this, because deliveryvisibility.go sits at its length
// ceiling: a change there has to pay for itself, and moving long-standing
// neighbours out to make room would re-date code nobody touched.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// attachmentParent names the record a file hangs off, which is the only thing
// that decides who may see the file: there is no `attachment` object grant, and
// the store's own read path asks for the parent's grant and then the parent's
// row scope (activities' resolveAttachmentParent).
//
// The file's own archival is not filtered. attachment.archived is
// emitted after archived_at is set, so a probe that required a live row would
// decline to deliver the very event saying the file is gone, silently, because
// an undeliverable event looks like one nobody subscribed to. An absent
// row answers an empty type, which the caller reads as not-visible.
func (s *Store) attachmentParent(ctx context.Context, attachmentID ids.UUID) (string, ids.UUID, error) {
	var parentType string
	var parentID ids.UUID
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT entity_type, entity_id FROM attachment WHERE id = $1`, attachmentID).
			Scan(&parentType, &parentID)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ids.UUID{}, nil
	}
	if err != nil {
		return "", ids.UUID{}, err
	}
	return parentType, parentID, nil
}
