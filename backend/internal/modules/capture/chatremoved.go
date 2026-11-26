// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// A chat message the provider deleted before this workspace filed it.
//
// A provider does not promise to order a deletion after the original. The
// original can also be redelivered later. Without a record of the deletion, the
// late original would file a message the sender took back. The record is a
// raw_capture row with no content, only a hash of the message's key. It names
// no account and ages out with the originals.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// chatRemovedSource is the raw_capture source system of a removal record.
const chatRemovedSource = "chat_removed"

// chatRemovedID is the removal record's source id: the message's source system
// and a hash of its source id, which embeds a conversation id.
func chatRemovedID(key connector.NaturalKey) string {
	sum := sha256.Sum256([]byte(key.SourceID))
	return key.SourceSystem + ":" + hex.EncodeToString(sum[:])
}

// lockChatKeyTx serialises filing, editing and removing the message under one
// key. A removal and a late original then cannot both pass their checks before
// either commits.
func lockChatKeyTx(ctx context.Context, tx pgx.Tx, key connector.NaturalKey) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		"chat_message:"+chatRemovedID(key)); err != nil {
		return fmt.Errorf("capture: waiting for a chat message's key: %w", err)
	}
	return nil
}

// writeChatTombstoneTx records that the provider removed the message under key.
func writeChatTombstoneTx(ctx context.Context, tx pgx.Tx, key connector.NaturalKey) error {
	_, err := InsertRawCaptureTx(ctx, tx, RawRecord{
		SourceSystem: chatRemovedSource, SourceID: chatRemovedID(key), Payload: []byte(`{"removed":true}`),
	})
	return err
}

// chatRemovedTx reports whether the provider removed the message under key
// before it was filed.
func chatRemovedTx(ctx context.Context, tx pgx.Tx, key connector.NaturalKey) (bool, error) {
	var removed bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM raw_capture WHERE source_system = $1 AND source_id = $2)`,
		chatRemovedSource, chatRemovedID(key)).Scan(&removed)
	if err != nil {
		return false, fmt.Errorf("capture: reading a chat message's removal record: %w", err)
	}
	return removed, nil
}

// removedBeforeFilingTx is the filing side of the removal record. It answers
// the drop reason for a chat message the provider already removed. It takes the
// key lock first, so the answer holds until this filing commits.
func removedBeforeFilingTx(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord) (string, error) {
	fields, ok := rec.Fields.(ActivityFields)
	if !ok || fields.Kind != ChatMessageKind || fields.ChannelProvider == "" {
		return "", nil
	}
	if err := lockChatKeyTx(ctx, tx, rec.NaturalKey); err != nil {
		return "", err
	}
	removed, err := chatRemovedTx(ctx, tx, rec.NaturalKey)
	if err != nil || !removed {
		return "", err
	}
	return "the provider removed this message before it was filed", nil
}
