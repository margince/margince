// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/erasedmark"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// erasedLinkBatch bounds one pass; the next run takes what is left.
const erasedLinkBatch = 100

type erasedLink struct {
	activity ids.UUID
	ref      string
}

// DestroyErasedLinks deletes the vault secrets that erased meeting rows still
// name. Erasure only marks the rows, so a rolled-back erasure never loses a
// secret; this pass runs after the commit. Each reference is cleared only once
// the vault has deleted it, so a refusing vault is retried on the next run.
func (s *Store) DestroyErasedLinks(ctx context.Context) error {
	var invitations, proposals []erasedLink
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		if invitations, err = readErasedLinks(ctx, tx,
			`SELECT activity_id, management_ref FROM meeting_invitation
			  WHERE status = 'erased' AND management_ref <> '' ORDER BY activity_id LIMIT $1`, erasedLinkBatch); err != nil {
			return err
		}
		proposals, err = readErasedLinks(ctx, tx,
			`SELECT activity_id, link_ref FROM meeting_proposal
			  WHERE left(token_hash, length($2::text)) = $2 ORDER BY activity_id LIMIT $1`, erasedLinkBatch, erasedmark.Prefix)
		return err
	})
	if err != nil {
		return fmt.Errorf("activities: reading erased meeting links: %w", err)
	}
	var faults []error
	for _, link := range invitations {
		faults = append(faults, s.settleErasedLink(ctx, link,
			`UPDATE meeting_invitation SET management_ref = '' WHERE activity_id = $1 AND management_ref = $2`))
	}
	for _, link := range proposals {
		faults = append(faults, s.settleErasedLink(ctx, link,
			`DELETE FROM meeting_proposal WHERE activity_id = $1 AND link_ref = $2`))
	}
	return errors.Join(faults...)
}

func readErasedLinks(ctx context.Context, tx pgx.Tx, query string, args ...any) ([]erasedLink, error) {
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (erasedLink, error) {
		var link erasedLink
		return link, row.Scan(&link.activity, &link.ref)
	})
}

func (s *Store) settleErasedLink(ctx context.Context, link erasedLink, dropRef string) error {
	if err := s.deleteMeetingLink(ctx, link.ref); err != nil {
		return fmt.Errorf("activities: destroying an erased meeting link: %w", err)
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, dropRef, link.activity, link.ref)
		return err
	})
}
