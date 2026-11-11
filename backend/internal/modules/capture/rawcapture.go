// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/originalstore"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	sqlArgGeneration = "generation"
	sqlArgIDs        = "ids"
	sqlArgKeys       = "keys"
	sqlArgObject     = "object"
	sqlArgPayload    = "payload"
	sqlArgProvider   = "provider"
	sqlArgSource     = "source"
	sqlArgSystem     = "system"
)

// RawRecord is one provider original waiting to be stored verbatim.
type RawRecord struct {
	SourceSystem string // e.g. "telegram"
	SourceID     string // the provider's REDELIVERY key, unique per workspace; never the domain natural key
	Payload      []byte // the verbatim provider payload
}

// InsertRawCaptureTx is the ONLY sanctioned way for another package to
// persist a raw provider record inside a caller's own transaction. capture
// owns raw_capture (tableownership_test.go); compose writing that table
// directly fails the ownership gate, and Sink.Upsert opens its OWN
// transaction so it cannot share a webhook handler's. This function opens
// none of its own — it joins whatever transaction the caller already holds,
// which is the entire reason it exists: a webhook must land the raw payload
// and enqueue its normalize job atomically (design §6.2), and that atomicity
// is only possible if the raw insert runs on the caller's tx.
//
// source_id is the provider's redelivery key, not the domain natural key — a
// redelivery repeats it, and ON CONFLICT DO UPDATE refreshes the stored
// original rather than duplicating it, exactly like
// raw_capture_source_unique's own comment describes. The domain-row natural
// key is a different question and belongs on the domain row, not here.
//
// Because that conflict OVERWRITES the payload, the caller owns an invariant
// this function cannot check: source_id must be unique per workspace across
// every source the workspace has connected. A counter the provider scopes to
// one credential (Telegram's per-bot update_id) has to carry that credential
// into the key, or a second connection's delivery silently destroys the only
// copy of the first's already-acknowledged payload.
func InsertRawCaptureTx(ctx context.Context, tx pgx.Tx, rec RawRecord) (ids.UUID, error) {
	if len(rec.Payload) > originalstore.InlineLimit {
		var existing ids.UUID
		err := tx.QueryRow(ctx, `SELECT id FROM raw_capture
 WHERE source_system = @system AND source_id = @source FOR UPDATE`,
			pgx.NamedArgs{sqlArgSystem: rec.SourceSystem, sqlArgSource: rec.SourceID}).Scan(&existing)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return ids.Nil, fmt.Errorf("capture: lock a redelivered original: %w", err)
		}
	}
	inline, ref, err := originalstore.PrepareTx(ctx, tx, rec.Payload)
	if err != nil {
		return ids.Nil, err
	}
	var id ids.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO raw_capture (source_system, source_id, payload, payload_object, payload_offload_checked_at)
		VALUES (@system, @source, @payload, @object::jsonb, now())
		ON CONFLICT (source_system, source_id) DO UPDATE
		SET payload = EXCLUDED.payload, payload_object = EXCLUDED.payload_object,
 payload_offload_checked_at = now(), received_at = now()
		RETURNING id`,
		pgx.NamedArgs{sqlArgSystem: rec.SourceSystem, sqlArgSource: rec.SourceID, sqlArgPayload: inline, sqlArgObject: ref}).Scan(&id)
	if err != nil {
		return ids.Nil, fmt.Errorf("capture: raw store: %w", err)
	}
	return id, originalstore.Adopt(ctx, tx, ref)
}

// GetRawCapturePayloadTx reads back one raw_capture row's verbatim payload —
// the ingest worker's read half of InsertRawCaptureTx's write (design §6.3):
// the webhook persists the update and enqueues a job naming this row's id in
// one transaction, and the worker re-opens the row by that id once it runs.
// Joins the caller's own transaction like InsertRawCaptureTx, so RLS resolves
// under whatever workspace GUC the caller already set.
func GetRawCapturePayloadTx(ctx context.Context, tx pgx.Tx, id ids.UUID) ([]byte, error) {
	var payload []byte
	err := tx.QueryRow(ctx, `SELECT `+originalstore.SQL("r")+` FROM raw_capture r WHERE id = $1`, id).Scan(&payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("capture: raw capture %s: %w", id, apperrors.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("capture: reading raw capture payload: %w", err)
	}
	return originalstore.Decode(ctx, payload)
}
