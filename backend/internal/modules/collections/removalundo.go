// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const auditIDField = "audit_id"

// The refusals a restore answers besides not-found.
var (
	ErrRemovalMovedOn    = fmt.Errorf("the record's link changed after this removal, so it cannot be put back: %w", apperrors.ErrConflict)
	ErrRemovalUnkept     = fmt.Errorf("what this removal took off is no longer kept, so it cannot be put back: %w", apperrors.ErrConflict)
	ErrTagRetiredRestore = fmt.Errorf("the tag is archived: restore it first: %w", apperrors.ErrConflict)
)

// removal is the audit row a restore is keyed by: it kept the link's provenance,
// which the restore writes back instead of the restorer's.
type removal struct {
	id    ids.UUID
	image []byte
	at    time.Time
}

// ownRemoval answers not-found for any row but the caller's own removal, or,
// for a bulk undo, one the undone batch wrote; an id says nothing about whose.
func ownRemoval(ctx context.Context, tx pgx.Tx, subject string, subjectID, auditID ids.UUID, batch *ids.UUID) (removal, error) {
	p, err := storekit.Actor(ctx)
	if err != nil {
		return removal{}, err
	}
	out := removal{id: auditID}
	err = tx.QueryRow(ctx, `
		SELECT after -> 'removed', occurred_at FROM audit_log
		 WHERE id = @audit_id AND entity_type = @subject AND entity_id = @subject_id AND action = 'update'
		   AND after ? 'removed'
		   AND (batch_id = @batch OR (@batch::uuid IS NULL AND actor_type = @actor_type AND actor_id = @actor_id))`,
		pgx.StrictNamedArgs{
			auditIDField: auditID, "subject": subject, "subject_id": subjectID, "batch": batch,
			"actor_type": string(p.Type), "actor_id": p.ID,
		}).Scan(&out.image, &out.at)
	if errors.Is(err, pgx.ErrNoRows) {
		return removal{}, apperrors.ErrNotFound
	}
	if err != nil {
		return removal{}, fmt.Errorf("read the removal to restore: %w", err)
	}
	return out, nil
}

func (r removal) decode(image *linkImage) error {
	if err := json.Unmarshal(r.image, image); err != nil {
		return fmt.Errorf("read the removal to restore: %w", err)
	}
	return nil
}

// linkImage is the record a tag or list audit row names, plus a removal's
// provenance.
type linkImage struct {
	EntityType string    `json:"entity_type"`
	EntityID   ids.UUID  `json:"entity_id"`
	RowID      *ids.UUID `json:"row_id,omitempty"`
	// Not created_at: the embedded ListMembership's would be shadowed.
	RowCreatedAt *time.Time `json:"row_created_at,omitempty"`
	*storekit.TagAssignment
	*storekit.ListMembership
}

// refuseMovedOn refuses when a later audit row touched the same link: putting
// the removed one back would overwrite that change. Ids are minted at write
// time, so they also order a row whose transaction began before the removal's.
func refuseMovedOn(ctx context.Context, tx pgx.Tx, subject string, subjectID ids.UUID, r removal, record linkImage) error {
	pair, err := json.Marshal(linkImage{EntityType: record.EntityType, EntityID: record.EntityID})
	if err != nil {
		return err
	}
	var moved bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM audit_log
		 WHERE entity_type = @subject AND entity_id = @subject_id AND id <> @removal
		   AND (occurred_at >= @at OR id > @removal)
		   AND (after -> 'applied' @> @pair::jsonb OR after -> 'added' @> @pair::jsonb OR after -> 'removed' @> @pair::jsonb))`,
		pgx.StrictNamedArgs{"subject": subject, "subject_id": subjectID, "removal": r.id, "at": r.at, "pair": string(pair)},
	).Scan(&moved)
	if err != nil {
		return fmt.Errorf("check the link for later changes: %w", err)
	}
	if moved {
		return ErrRemovalMovedOn
	}
	return nil
}
