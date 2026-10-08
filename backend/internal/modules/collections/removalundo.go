// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

// A restore is keyed by the removal's audit row because that row kept the
// link's provenance, which the restore writes back instead of the restorer's.

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

// The refusals a restore answers besides not-found.
var (
	ErrRemovalMovedOn    = fmt.Errorf("the record's link changed after this removal, so it cannot be put back: %w", apperrors.ErrConflict)
	ErrRemovalUnkept     = fmt.Errorf("what this removal took off is no longer kept, so it cannot be put back: %w", apperrors.ErrConflict)
	ErrTagRetiredRestore = fmt.Errorf("the tag is archived: restore it first: %w", apperrors.ErrConflict)
)

type removal struct {
	id    ids.UUID
	image []byte
	at    time.Time
}

// ownRemoval answers not-found for any row but the caller's own removal, so an
// id says nothing about whose removal it was.
func ownRemoval(ctx context.Context, tx pgx.Tx, subject string, subjectID, auditID ids.UUID) (removal, error) {
	p, err := storekit.Actor(ctx)
	if err != nil {
		return removal{}, err
	}
	out := removal{id: auditID}
	err = tx.QueryRow(ctx, `
		SELECT after -> 'removed', occurred_at FROM audit_log
		 WHERE id = $1 AND entity_type = $2 AND entity_id = $3 AND action = 'update'
		   AND after ? 'removed' AND actor_type = $4 AND actor_id = $5`,
		auditID, subject, subjectID, string(p.Type), p.ID).Scan(&out.image, &out.at)
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
	EntityType string   `json:"entity_type"`
	EntityID   ids.UUID `json:"entity_id"`
	*storekit.TagAssignment
	*storekit.ListMembership
}

// refuseMovedOn refuses when a later audit row touched the same link: putting
// the removed one back would overwrite that change.
func refuseMovedOn(ctx context.Context, tx pgx.Tx, subject string, subjectID ids.UUID, r removal, record linkImage) error {
	pair, err := json.Marshal(linkImage{EntityType: record.EntityType, EntityID: record.EntityID})
	if err != nil {
		return err
	}
	var moved bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM audit_log
		 WHERE entity_type = $1 AND entity_id = $2 AND id <> $3 AND occurred_at >= $4
		   AND (after -> 'applied' @> $5::jsonb OR after -> 'added' @> $5::jsonb OR after -> 'removed' @> $5::jsonb))`,
		subject, subjectID, r.id, r.at, string(pair)).Scan(&moved)
	if err != nil {
		return fmt.Errorf("check the link for later changes: %w", err)
	}
	if moved {
		return ErrRemovalMovedOn
	}
	return nil
}
