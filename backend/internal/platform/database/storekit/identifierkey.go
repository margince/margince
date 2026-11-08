// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SuppressionKeyLabel names the key erased identifiers are hashed under. The
// program derives it from the root key, so it lives outside the database it
// protects.
const SuppressionKeyLabel = "margince erasure suppression v1"

// rootKeyVariable names the setting the key derives from, for the boot refusal.
const rootKeyVariable = "MARGINCE_KEYVAULT_ROOT_KEY"

// keyIDBytes is how much of the key's own MAC names it in a key_id column.
const keyIDBytes = 8

// identifierKey is the suppression key and the id its rows are stamped with.
type identifierKey struct {
	secret []byte
	id     string
}

// boundSuppressionKey is the key the program bound at boot, nil for none.
var boundSuppressionKey atomic.Pointer[identifierKey]

// UseSuppressionKey binds the key this process hashes erased identifiers
// under. A program calls it once with its configuration; nil binds none, and
// identifiers are then hashed without a key.
func UseSuppressionKey(secret []byte) {
	if secret == nil {
		boundSuppressionKey.Store(nil)
		return
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("key-id"))
	boundSuppressionKey.Store(&identifierKey{secret: secret, id: hex.EncodeToString(mac.Sum(nil)[:keyIDBytes])})
}

// currentIdentifierKey is the key this process hashes erased identifiers
// under, or false when the installation configures no root key.
func currentIdentifierKey() (identifierKey, bool) {
	key := boundSuppressionKey.Load()
	if key == nil {
		return identifierKey{}, false
	}
	return *key, true
}

// identifierDigest is the unkeyed sha256 of the normalized value. Rows written
// without a key hold it. The keyed form is a MAC over it, so a stored digest
// can be re-keyed without the value it came from.
func identifierDigest(value string) string {
	digest := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(value))))
	return hex.EncodeToString(digest[:])
}

func (k identifierKey) keyed(digest string) string {
	mac := hmac.New(sha256.New, k.secret)
	mac.Write([]byte(digest))
	return hex.EncodeToString(mac.Sum(nil))
}

// SuppressionKeyID is the key_id a hash SuppressionHash writes now is stamped
// with, or nil for an unkeyed hash.
func SuppressionKeyID() *string {
	key, ok := currentIdentifierKey()
	if !ok {
		return nil
	}
	return &key.id
}

// SuppressionForms answers the stored forms each value can have, in order. The
// keyed hash comes first when a key is bound. The unkeyed one follows, as rows
// written before the key, or without one, hold it. Every reader matches against
// all of them, so a row the re-hash has not reached yet still suppresses.
func SuppressionForms(values ...string) []string {
	key, keyed := currentIdentifierKey()
	forms := make([]string, 0, 2*len(values))
	for _, value := range values {
		digest := identifierDigest(value)
		if keyed {
			forms = append(forms, key.keyed(digest))
		}
		forms = append(forms, digest)
	}
	return forms
}

// RekeySuppressionHash is the keyed form of a stored unkeyed hash, made with
// the key SuppressionKeyID names. It reports false when no key is bound.
func RekeySuppressionHash(digest string) (string, bool) {
	key, keyed := currentIdentifierKey()
	if !keyed {
		return "", false
	}
	return key.keyed(digest), true
}

// RowQuerier is the one verb a bounded count needs.
type RowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// CountUnkeyedIdentifiers counts the rows selector names, stopping one past
// maximum. It answers zero when no key is bound, since nothing can be re-keyed.
func CountUnkeyedIdentifiers(ctx context.Context, q RowQuerier, selector string, maximum int64) (int64, error) {
	if SuppressionKeyID() == nil {
		return 0, nil
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (`+selector+` LIMIT @maximum) bounded`,
		pgx.StrictNamedArgs{"maximum": maximum + 1}).Scan(&count)
	return count, err
}

// RefuseForeignSuppressionKey refuses to start a process whose root key did
// not make the keyed hashes stored. Under another key, or none, every keyed
// row matches nothing, and each erased address would be stored again.
func RefuseForeignSuppressionKey(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return nil
	}
	var migrated bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'erasure_suppression' AND column_name = 'key_id')`).Scan(&migrated); err != nil {
		return fmt.Errorf("store: checking which key hashed the erasure list: %w", err)
	}
	if !migrated {
		return nil
	}
	current := ""
	if key, ok := currentIdentifierKey(); ok {
		current = key.id
	}
	var foreign bool
	if err := pool.QueryRow(ctx, `SELECT
		EXISTS (SELECT 1 FROM erasure_suppression WHERE key_id IS NOT NULL AND key_id <> $1)
		OR EXISTS (SELECT 1 FROM communication_suppression WHERE address_key_id IS NOT NULL AND address_key_id <> $1)`,
		current).Scan(&foreign); err != nil {
		return fmt.Errorf("store: checking which key hashed the erasure list: %w", err)
	}
	if !foreign {
		return nil
	}
	return fmt.Errorf("store: the erasure list was hashed under a key derived from a different %s, "+
		"or one that is now unset, so erased addresses would no longer be refused. Restore the root key "+
		"this installation used; the list cannot be rebuilt without it", rootKeyVariable)
}
