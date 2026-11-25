// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package notices

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// HandOverTx moves one announcement to a new recipient. Every other copy is
// taken back. The recipient's line is written, or, when an earlier hand-over
// took it back, opened again unread with the words in. Its mail attempt is
// kept, so handing a request back and forth sends each member one email at most.
func (s *Store) HandOverTx(ctx context.Context, tx pgx.Tx, in NewNotice) (ids.UUID, error) {
	if in.DedupeKey == "" {
		return ids.Nil, errors.New("notices: a hand-over names the announcement it moves")
	}
	if _, err := tx.Exec(ctx, `
		UPDATE notice SET retracted_at = now()
		 WHERE dedupe_key = $1 AND recipient_user_id <> $2 AND retracted_at IS NULL`,
		in.DedupeKey, in.Recipient); err != nil {
		return ids.Nil, fmt.Errorf("notices: taking back an announcement from its former recipients: %w", err)
	}
	metadata, subject, body := fittedWords(ctx, in)
	var reopened ids.UUID
	err := tx.QueryRow(ctx, `
		UPDATE notice SET retracted_at = NULL, read_at = NULL, subject = $3, body = $4, system_text = $5, subject_parts = $6
		 WHERE recipient_user_id = $1 AND dedupe_key = $2 AND retracted_at IS NOT NULL
		RETURNING id`, in.Recipient, in.DedupeKey, subject, body, metadata, in.SubjectParts).Scan(&reopened)
	if errors.Is(err, pgx.ErrNoRows) {
		return s.CreateTx(ctx, tx, in)
	}
	if err != nil {
		return ids.Nil, fmt.Errorf("notices: opening a taken-back announcement again: %w", err)
	}
	if _, err := storekit.Audit(ctx, tx, "update", "notice", reopened,
		map[string]any{"retracted": true}, map[string]any{"retracted": false}); err != nil {
		return ids.Nil, err
	}
	return reopened, nil
}

// RewordLiveTx writes new words into the recipient's live line of one
// announcement, fitted as a new line's are. A line taken back, or never
// written, is left alone. Whether it was read is kept.
func (s *Store) RewordLiveTx(ctx context.Context, tx pgx.Tx, in NewNotice) error {
	if in.DedupeKey == "" {
		return errors.New("notices: a reword names the announcement it changes")
	}
	metadata, subject, body := fittedWords(ctx, in)
	var id ids.UUID
	var was string
	err := tx.QueryRow(ctx, `
		UPDATE notice AS n SET subject = $3, body = $4, system_text = $5, subject_parts = $6
		  FROM (SELECT id, subject FROM notice WHERE recipient_user_id = $1 AND dedupe_key = $2 AND retracted_at IS NULL FOR UPDATE) AS old
		 WHERE n.id = old.id
		RETURNING n.id, old.subject`, in.Recipient, in.DedupeKey, subject, body, metadata, in.SubjectParts).Scan(&id, &was)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("notices: rewording a live announcement: %w", err)
	}
	_, err = storekit.Audit(ctx, tx, "update", "notice", id, map[string]any{"subject": was}, map[string]any{"subject": subject})
	return err
}
