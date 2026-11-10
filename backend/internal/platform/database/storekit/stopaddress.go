// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// StopOnAddressesSQL is true when the communication_suppression row aliased
// alias stops one of a set of addresses, by its plain address or its hash.
// lowered and hashes are the text[] parameters StopAddressArgs answers. The
// send check, the refusal review, the stop list and the Art. 15 export all ask
// through it, so none misses a stop that holds only a hash.
func StopOnAddressesSQL(alias, lowered, hashes string) string {
	return "((" + alias + ".address IS NOT NULL AND lower(" + alias + ".address) = ANY(" + lowered + "::text[]))" +
		" OR (" + alias + ".address_hash IS NOT NULL AND " + alias + ".address_hash = ANY(" + hashes + "::text[])))"
}

// StopAddressArgs answers the parameters of StopOnAddressesSQL. They are each
// address lowercased, and every SuppressionForms of each spelling EmailLookups
// knows, the hash an anonymize writes. An empty address matches nothing.
func StopAddressArgs(addresses ...string) (lowered, hashes []string) {
	lowered = make([]string, 0, len(addresses))
	for _, address := range addresses {
		if trimmed := strings.ToLower(strings.TrimSpace(address)); trimmed != "" {
			lowered = append(lowered, trimmed)
		}
	}
	return lowered, SuppressionForms(EmailLookups(addresses...)...)
}

// StopAddressLockKeys follows the address/hash matches in StopOnAddressesSQL.
// Stored hashes also participate when the subject's plain addresses are gone.
func StopAddressLockKeys(addresses, storedHashes []string) []string {
	lowered, hashes := StopAddressArgs(addresses...)
	keys := make([]string, 0, len(lowered)+len(hashes)+len(storedHashes))
	for _, address := range lowered {
		keys = append(keys, "communication-stop:address:"+address)
	}
	for _, hash := range append(hashes, storedHashes...) {
		if hash != "" {
			keys = append(keys, "communication-stop:hash:"+hash)
		}
	}
	slices.Sort(keys)
	return slices.Compact(keys)
}

// LockStopAddresses orders address-bound stops against the final send check.
func LockStopAddresses(ctx context.Context, tx pgx.Tx, addresses, hashes []string) error {
	for _, key := range StopAddressLockKeys(addresses, hashes) {
		if err := LockStopKey(ctx, tx, key); err != nil {
			return err
		}
	}
	return nil
}

// LockSubjectsStopAddresses reads under the caller's subject locks, and takes
// all copied or cleared address keys together to keep their ordering stable.
func LockSubjectsStopAddresses(ctx context.Context, tx pgx.Tx, subjects ...ids.UUID) error {
	var addresses, hashes []string
	if err := tx.QueryRow(ctx, `WITH stops AS (
 SELECT s.address, s.address_hash, coalesce(s.recorded_for_addresses, origin.recorded_for_addresses) AS bound
 FROM communication_suppression s
 LEFT JOIN communication_suppression origin ON origin.id = s.carried_from
 WHERE s.contact_id = ANY(@subjects::uuid[]) OR s.lead_id = ANY(@subjects::uuid[])
), addresses AS (
 SELECT address FROM stops WHERE address IS NOT NULL
 UNION ALL SELECT unnest(bound) FROM stops
 UNION ALL SELECT email FROM contact_email WHERE contact_id = ANY(@subjects::uuid[])
 UNION ALL SELECT email FROM lead WHERE id = ANY(@subjects::uuid[]) AND email IS NOT NULL
 UNION ALL SELECT unnest(recorded_for_addresses) FROM contact_consent
 WHERE contact_id = ANY(@subjects::uuid[]) OR lead_id = ANY(@subjects::uuid[])
)
 SELECT ARRAY(SELECT address FROM addresses), ARRAY(SELECT address_hash FROM stops WHERE address_hash IS NOT NULL)`,
		pgx.NamedArgs{"subjects": subjects}).Scan(&addresses, &hashes); err != nil {
		return fmt.Errorf("storekit: reading the subject's stop addresses: %w", err)
	}
	return LockStopAddresses(ctx, tx, addresses, hashes)
}
