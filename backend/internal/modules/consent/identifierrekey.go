// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// storedValue is one row's id and the value a correction rewrites.
type storedValue struct {
	id    ids.UUID
	value string
}

func readStoredValues(ctx context.Context, tx pgx.Tx, query string, after ids.UUID, limit int) ([]storedValue, error) {
	rows, err := tx.Query(ctx, query, pgx.StrictNamedArgs{"after": after, "limit": limit})
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (storedValue, error) {
		var v storedValue
		err := row.Scan(&v.id, &v.value)
		return v, err
	})
}

func storedIDs(page []storedValue) []ids.UUID {
	out := make([]ids.UUID, len(page))
	for i, v := range page {
		out[i] = v.id
	}
	return out
}

// RekeyKeptStopsPageTx re-keys the address hash of up to limit stops after the
// cursor, and answers the stops it inspected. A stop keeps its row and id, so
// whatever names it still does. It inspects nothing when no key is bound.
func RekeyKeptStopsPageTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ids.UUID, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	keyID := storekit.SuppressionKeyID()
	if keyID == nil {
		return nil, nil
	}
	page, err := readStoredValues(ctx, tx, `SELECT id, address_hash FROM communication_suppression
		WHERE address_hash IS NOT NULL AND address_key_id IS NULL AND id > @after
		ORDER BY id LIMIT @limit FOR UPDATE`, after, limit)
	if err != nil {
		return nil, fmt.Errorf("consent: reading unkeyed stops: %w", err)
	}
	rekeyed := make([]string, len(page))
	for i, stop := range page {
		rekeyed[i], _ = storekit.RekeySuppressionHash(stop.value)
	}
	if _, err := tx.Exec(ctx, `UPDATE communication_suppression s
		SET address_hash = u.hash, address_key_id = @key
		FROM unnest(@ids::uuid[], @hashes::text[]) AS u(id, hash) WHERE s.id = u.id`,
		pgx.StrictNamedArgs{"ids": storedIDs(page), "hashes": rekeyed, fieldKey: *keyID}); err != nil {
		return nil, fmt.Errorf("consent: re-keying stops: %w", err)
	}
	return storedIDs(page), nil
}

// CountUnkeyedStops bounds the re-key's estimate at one row past maximum.
func CountUnkeyedStops(ctx context.Context, q storekit.RowQuerier, maximum int64) (int64, error) {
	return storekit.CountUnkeyedIdentifiers(ctx, q, `SELECT 1 FROM communication_suppression
		WHERE address_hash IS NOT NULL AND address_key_id IS NULL`, maximum)
}

// HashPreferenceLinksPageTx replaces the stored link of up to limit rows after
// the cursor with its hash, and answers the rows it inspected. The link still
// opens: preferenceTokenRow finds it by the hash. It inspects nothing without a
// root key, because a send could not hand the link out again.
func HashPreferenceLinksPageTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ids.UUID, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	if preferenceLinkKey() == nil {
		return nil, nil
	}
	page, err := readStoredValues(ctx, tx, `SELECT id, token FROM preference_token
		WHERE token IS NOT NULL AND id > @after ORDER BY id LIMIT @limit FOR UPDATE`, after, limit)
	if err != nil {
		return nil, fmt.Errorf("consent: reading stored preference links: %w", err)
	}
	hashes := make([]string, len(page))
	for i, link := range page {
		hashes[i] = hashPublicToken(link.value)
	}
	if _, err := tx.Exec(ctx, `UPDATE preference_token p SET token_hash = u.hash, token = NULL
		FROM unnest(@ids::uuid[], @hashes::text[]) AS u(id, hash) WHERE p.id = u.id`,
		pgx.StrictNamedArgs{"ids": storedIDs(page), "hashes": hashes}); err != nil {
		return nil, fmt.Errorf("consent: hashing stored preference links: %w", err)
	}
	return storedIDs(page), nil
}

// CountStoredPreferenceLinks bounds the hashing's estimate at one row past
// maximum. It answers zero without a root key, as the hashing inspects nothing.
func CountStoredPreferenceLinks(ctx context.Context, q storekit.RowQuerier, maximum int64) (int64, error) {
	if preferenceLinkKey() == nil {
		return 0, nil
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM preference_token WHERE token IS NOT NULL LIMIT @maximum) bounded`,
		pgx.StrictNamedArgs{"maximum": maximum + 1}).Scan(&count)
	return count, err
}
