// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/storedobjects"
)

// attachmentIntents is this module's handle on the intent ledger, which lives in
// platform because five modules put bytes and a module never imports a sibling.
func (s *Store) attachmentIntents() *storedobjects.Ledger {
	return storedobjects.NewLedger(s.db)
}

// recordAttachmentIntent declares an attachment's key provisional before its put.
func (s *Store) recordAttachmentIntent(ctx context.Context, key string) error {
	return s.attachmentIntents().Record(ctx, storedobjects.KindAttachment, key)
}

// UnreferencedAttachmentKeys answers which of these keys no attachment row carries.
//
// THE REFERENCE CHECK IS THE SECOND LOCK, not the first. The first is that a key was
// declared provisional at all — nothing but a writer's own declaration puts one in
// the ledger. This exists because the two are different failures: a clear that did
// not run leaves a live file's key provisional, and a reaper trusting the ledger
// alone would delete the bytes of a document somebody can still see.
//
// Answered HERE because `attachment` is this module's table. The sweep holds the
// ledger and asks each kind's owner, since no module may read another's rows and the
// ledger sits below all of them.
func (s *Store) UnreferencedAttachmentKeys(ctx context.Context, keys []string) ([]string, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, nil
	}
	var out []string
	err := s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT k FROM unnest($1::text[]) AS k
			 WHERE NOT EXISTS (SELECT 1 FROM attachment a WHERE a.storage_key = k)`, keys)
		if err != nil {
			return fmt.Errorf("check which attachment keys are unreferenced: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var key string
			if err := rows.Scan(&key); err != nil {
				return fmt.Errorf("read an unreferenced attachment key: %w", err)
			}
			out = append(out, key)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
