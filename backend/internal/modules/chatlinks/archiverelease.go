// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ReleaseArchived takes an archived deal or project out of its link. A link
// still carrying a live record stays with it; one left with none is retired,
// which frees its channel. A restore never brings the link back.
func (s *Store) ReleaseArchived(ctx context.Context, kind string, id ids.UUID) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	column, err := recordColumn(kind)
	if err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		live, err := recordLive(ctx, tx, kind, id)
		if err != nil || live {
			return err
		}
		before, err := heldLiveLinkOf(ctx, tx, kind, id)
		if errors.Is(err, ErrLinkNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		kept, err := carriesLiveRecordBesides(ctx, tx, before, kind)
		if err != nil {
			return err
		}
		if !kept {
			err := retireLink(ctx, tx, before)
			if errors.Is(err, ErrLinkNotFound) {
				return nil
			}
			return err
		}
		after, err := scanLink(tx.QueryRow(ctx, `UPDATE chat_link SET `+column+` = NULL
			WHERE id = $1 AND version = $2 AND archived_at IS NULL RETURNING `+linkColumns, before.ID, before.Version))
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if _, err := storekit.Audit(ctx, tx, "update", chatLinkEntity, after.ID, auditImage(before), auditImage(after)); err != nil {
			return err
		}
		// The card names the deal while the link carries one, so it is redrawn.
		return s.jobs.EnqueueFlushTx(ctx, tx, after.ID)
	})
}

// carriesLiveRecordBesides reports whether the link carries a live record
// other than its record of kind.
func carriesLiveRecordBesides(ctx context.Context, tx pgx.Tx, link Link, kind string) (bool, error) {
	if kind == RecordCompany {
		return false, nil
	}
	other, id := RecordProject, link.ProjectID
	if kind == RecordProject {
		other, id = RecordDeal, link.DealID
	}
	if id == nil {
		return false, nil
	}
	return recordLive(ctx, tx, other, *id)
}

// recordLive reports whether the deal or project exists and is not archived.
// kind is RecordDeal or RecordProject, each also its table. The row is held
// until tx ends, so a restore cannot land between this read and the release.
func recordLive(ctx context.Context, tx pgx.Tx, kind string, id ids.UUID) (bool, error) {
	var live bool
	err := tx.QueryRow(ctx, fmt.Sprintf(`SELECT archived_at IS NULL FROM %s WHERE id = $1 FOR SHARE`,
		pgx.Identifier{kind}.Sanitize()), id).Scan(&live)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return live, err
}

// retireLink archives a live link the caller holds, which frees its channel
// and its records, and audits it. A link no longer live answers ErrLinkNotFound.
func retireLink(ctx context.Context, tx pgx.Tx, link Link) error {
	tag, err := tx.Exec(ctx, `UPDATE chat_link SET archived_at = now()
		WHERE id = $1 AND version = $2 AND archived_at IS NULL`, link.ID, link.Version)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("chatlinks: retiring %s: %w", link.ID, ErrLinkNotFound)
	}
	_, err = storekit.Audit(ctx, tx, "archive", chatLinkEntity, link.ID, auditImage(link), nil)
	return err
}
