// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Archiving a file: this subsystem's delete, and the event that states it.
//
// Its own file because attachment.go had grown past the length a reader can
// hold at once, and because the archive is where the before-image matters,
// what the event says about a file is read while the file is still live.

import (
	"context"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ArchiveAttachment soft-deletes the row (identical to the module's other
// archive verbs). The object bytes are retained: authoritative
// byte-erasure is the Art. 17 path, matching how every archived record's data
// persists until erasure. Authority inherits from the parent (Update + row
// scope). Archived/invisible reads as ErrNotFound.
func (s *Store) ArchiveAttachment(ctx context.Context, id ids.UUID) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		entityType, err := resolveAttachmentParent(ctx, tx, id, principal.ActionUpdate)
		if err != nil {
			return err
		}
		// Taken by name, and the liveness re-checked UNDER it, the same prelude
		// the metadata edit beside this one takes (documents.go).
		//
		// The check above holds no lock, so two archives of one file both pass
		// it. Serialised here, the second blocks until the first commits and
		// then re-evaluates `archived_at is NULL` against the row as it now
		// stands: no live row, ErrNotFound, nothing written. Without it both
		// would write, audit and EMIT, so one file leaving the listings once
		// would announce itself twice, and the two envelopes carry different
		// event ids, so consumer dedupe does not collapse them.
		if _, err := storekit.LockRow(ctx, tx, "attachment", id, storekit.LiveOnly); err != nil {
			return err
		}
		// Read before the archive, because the event states what the file was.
		// its parent, type and size are facts about the row that is leaving the
		// live listings, and a read afterwards would have to ask for an archived
		// row to get them.
		before, err := readAttachment(ctx, tx, id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE attachment SET archived_at = now() WHERE id = $1`, id); err != nil {
			return err
		}
		auditID, err := storekit.Audit(ctx, tx, "archive", "attachment", id, nil, map[string]any{
			fieldEntityType: entityType,
		})
		if err != nil {
			return err
		}
		// Archiving is this subsystem's delete, so it carries the same event
		// shape as the other two mutations rather than a bare notification: a
		// subscriber keeping its own index of a record's files needs to know
		// which parent lost one without reading back a row it can no longer
		// list.
		return storekit.EmitEvent(ctx, tx, auditID, id, crmcontracts.PublicEventAttachmentArchived{
			ParentType:  entityType,
			ParentId:    openapi_types.UUID(before.EntityId),
			ContentType: before.ContentType,
			ByteSize:    before.ByteSize,
		})
	})
}
