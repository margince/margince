// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// unkeyedLoginRows are the failed-login log rows whose address hash has no
// key. The database lets each such hash be replaced once, by its keyed form.
const unkeyedLoginRows = `FROM system_log
	WHERE action = 'login' AND detail ? 'email_hash' AND NOT detail ? 'email_key_id'`

// RekeyLoginAttemptsPageTx re-keys the address hash of up to limit failed-login
// rows after the cursor, and answers the rows it inspected. It inspects
// nothing when no key is bound.
func RekeyLoginAttemptsPageTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ids.UUID, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	keyID := SuppressionKeyID()
	if keyID == nil {
		return nil, nil
	}
	rows, err := tx.Query(ctx, `SELECT id, detail->>'email_hash' `+unkeyedLoginRows+`
		AND id > @after ORDER BY id LIMIT @limit FOR UPDATE`, pgx.StrictNamedArgs{"after": after, pageLimitParameter: limit})
	if err != nil {
		return nil, fmt.Errorf("store: reading unkeyed failed-login rows: %w", err)
	}
	var (
		scanned []ids.UUID
		hashes  []string
	)
	for rows.Next() {
		var id ids.UUID
		var digest string
		if err := rows.Scan(&id, &digest); err != nil {
			rows.Close()
			return nil, err
		}
		hash, _ := RekeySuppressionHash(digest)
		scanned, hashes = append(scanned, id), append(hashes, hash)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE system_log l
		SET detail = l.detail || jsonb_build_object('email_hash', u.hash, 'email_key_id', @key::text)
		FROM unnest(@ids::uuid[], @hashes::text[]) AS u(id, hash) WHERE l.id = u.id`,
		pgx.StrictNamedArgs{"ids": scanned, "hashes": hashes, "key": *keyID}); err != nil {
		return nil, fmt.Errorf("store: re-keying failed-login rows: %w", err)
	}
	return scanned, nil
}

// CountUnkeyedLoginAttempts bounds the re-key's estimate at one row past maximum.
func CountUnkeyedLoginAttempts(ctx context.Context, q RowQuerier, maximum int64) (int64, error) {
	return CountUnkeyedIdentifiers(ctx, q, `SELECT 1 `+unkeyedLoginRows, maximum)
}
