// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// SuppressionCursor is where a re-key of the erasure list stopped, in the
// order of its primary key.
type SuppressionCursor struct {
	Kind string `json:"kind"`
	Hash string `json:"hash"`
}

// The named parameters the rekey pages bind.
const (
	rekeyKindParameter  = "kind"
	rekeyAfterParameter = "after"
	rekeyLimitParameter = "limit"
)

// RekeySuppressionListPageTx replaces up to limit unkeyed erasure_suppression
// rows after the cursor with their keyed form, keeping when each was erased.
// It answers the rows it inspected. The keyed row is written before the
// unkeyed one goes, so the address stays refused at every point. It inspects
// nothing when no key is bound.
func RekeySuppressionListPageTx(ctx context.Context, tx pgx.Tx, after SuppressionCursor, limit int) ([]SuppressionCursor, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	keyID := storekit.SuppressionKeyID()
	if keyID == nil {
		return nil, nil
	}
	rows, err := tx.Query(ctx, `SELECT kind, value_hash, created_at, lifted_at, lift_consent_event_id, lift_contact_id FROM erasure_suppression
		WHERE key_id IS NULL AND (kind, value_hash) > (@kind, @hash)
		ORDER BY kind, value_hash LIMIT @limit FOR UPDATE`,
		pgx.StrictNamedArgs{rekeyKindParameter: after.Kind, "hash": after.Hash, rekeyLimitParameter: limit})
	if err != nil {
		return nil, fmt.Errorf("privacy: reading unkeyed erasure list rows: %w", err)
	}
	var (
		page                  []SuppressionCursor
		kinds, unkeyed, keyed []string
		erasedAt              []time.Time
		liftedAt              []*time.Time
		liftConsents          []*ids.UUID
		liftContacts          []*ids.UUID
	)
	for rows.Next() {
		var row SuppressionCursor
		var at time.Time
		var lifted *time.Time
		var consent, contact *ids.UUID
		if err := rows.Scan(&row.Kind, &row.Hash, &at, &lifted, &consent, &contact); err != nil {
			rows.Close()
			return nil, err
		}
		hash, _ := storekit.RekeySuppressionHash(row.Hash)
		page, kinds, unkeyed, keyed, erasedAt = append(page, row), append(kinds, row.Kind),
			append(unkeyed, row.Hash), append(keyed, hash), append(erasedAt, at)
		liftedAt, liftConsents, liftContacts = append(liftedAt, lifted), append(liftConsents, consent), append(liftContacts, contact)
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(page) == 0 {
		return page, err
	}
	// The keyed row carries the unkeyed row's lift. When both forms exist and
	// either is armed, the keyed row ends armed: a lift never wins a merge.
	if _, err := tx.Exec(ctx, `INSERT INTO erasure_suppression (kind, value_hash, key_id, created_at, lifted_at, lift_consent_event_id, lift_contact_id)
		SELECT kind, hash, @key, at, lifted, consent, contact
		  FROM unnest(@kinds::text[], @keyed::text[], @at::timestamptz[], @lifted::timestamptz[], @consents::uuid[], @contacts::uuid[])
		       AS u(kind, hash, at, lifted, consent, contact)
		ON CONFLICT (kind, value_hash) DO UPDATE SET lifted_at = NULL, lift_consent_event_id = NULL, lift_contact_id = NULL
		 WHERE EXCLUDED.lifted_at IS NULL AND erasure_suppression.lifted_at IS NOT NULL`,
		pgx.StrictNamedArgs{"kinds": kinds, "keyed": keyed, "at": erasedAt, "lifted": liftedAt, "consents": liftConsents, "contacts": liftContacts, "key": *keyID}); err != nil {
		return nil, fmt.Errorf("privacy: writing the keyed erasure list rows: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM erasure_suppression s
		USING unnest(@kinds::text[], @unkeyed::text[]) AS u(kind, hash)
		WHERE s.kind = u.kind AND s.value_hash = u.hash AND s.key_id IS NULL`,
		pgx.StrictNamedArgs{"kinds": kinds, "unkeyed": unkeyed}); err != nil {
		return nil, fmt.Errorf("privacy: removing the unkeyed erasure list rows: %w", err)
	}
	return page, nil
}

// CountUnkeyedSuppressions bounds the re-key's estimate at one row past maximum.
func CountUnkeyedSuppressions(ctx context.Context, q storekit.RowQuerier, maximum int64) (int64, error) {
	return storekit.CountUnkeyedIdentifiers(ctx, q, `SELECT 1 FROM erasure_suppression WHERE key_id IS NULL`, maximum)
}

// RekeyAnonymizedAddressesPageTx re-keys the address hashes an anonymize kept
// on up to limit contacts after the cursor, and answers the contacts it
// inspected. It inspects nothing when no key is bound.
func RekeyAnonymizedAddressesPageTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ids.UUID, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	keyID := storekit.SuppressionKeyID()
	if keyID == nil {
		return nil, nil
	}
	rows, err := tx.Query(ctx, `SELECT id, anonymized_address_hashes FROM contact
		WHERE anonymized_address_hashes IS NOT NULL AND anonymized_address_key_id IS NULL AND id > @after
		ORDER BY id LIMIT @limit FOR UPDATE`, pgx.StrictNamedArgs{rekeyAfterParameter: after, rekeyLimitParameter: limit})
	if err != nil {
		return nil, fmt.Errorf("privacy: reading unkeyed anonymized address hashes: %w", err)
	}
	type kept struct {
		id     ids.UUID
		hashes []string
	}
	contacts, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (kept, error) {
		var k kept
		err := row.Scan(&k.id, &k.hashes)
		return k, err
	})
	if err != nil {
		return nil, err
	}
	scanned := make([]ids.UUID, 0, len(contacts))
	for _, contact := range contacts {
		rekeyed := make([]string, len(contact.hashes))
		for i, hash := range contact.hashes {
			rekeyed[i], _ = storekit.RekeySuppressionHash(hash)
		}
		if _, err := tx.Exec(ctx, `UPDATE contact SET anonymized_address_hashes = @hashes,
			anonymized_address_key_id = @key WHERE id = @id`,
			pgx.StrictNamedArgs{"hashes": rekeyed, "key": *keyID, "id": contact.id}); err != nil {
			return nil, fmt.Errorf("privacy: re-keying anonymized address hashes: %w", err)
		}
		scanned = append(scanned, contact.id)
	}
	return scanned, nil
}

// CountUnkeyedAnonymizedContacts bounds the re-key's estimate at one row past maximum.
func CountUnkeyedAnonymizedContacts(ctx context.Context, q storekit.RowQuerier, maximum int64) (int64, error) {
	return storekit.CountUnkeyedIdentifiers(ctx, q, `SELECT 1 FROM contact
		WHERE anonymized_address_hashes IS NOT NULL AND anonymized_address_key_id IS NULL`, maximum)
}
