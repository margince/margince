// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The user's confirmation of one previewed bulk change.
//
// A preview of more than bulkConfirmAbove records mints a token and stores only
// its hash, beside a hash of exactly what was previewed. The execution presents
// the token and spends the row in its own transaction: an execution of anything
// else does not match, a second execution finds the row spent, and a change
// that rolls back leaves the token unspent for the retry.
//
// The token is a random secret rather than a signed claim. The stored row is
// what makes it single-use, and a signature would add nothing the row does not
// already decide.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// bulkConfirmAbove is the largest selection that runs without a confirmation.
const bulkConfirmAbove = 10

// bulkConfirmFor is how long the user has between seeing a preview and
// confirming it. Long enough to read three sample rows and a list of
// exclusions; short enough that the versions it pinned are still worth
// comparing.
const bulkConfirmFor = 15 * time.Minute

// bulkConfirmTokenBytes is the token's entropy before encoding.
const bulkConfirmTokenBytes = 32

// errConfirmTokenRequired and errConfirmTokenRefused are the two answers an
// execution without a usable confirmation gets. Neither says which of used,
// expired, foreign or mismatched a refused token was: the remedy is the same
// in every case — preview again.
var (
	errConfirmTokenRequired = httperr.Validation("confirm_token", "confirm_token_required",
		fmt.Sprintf("a change to more than %d records needs the confirm_token its preview returned; preview it first", bulkConfirmAbove))
	errConfirmTokenRefused = httperr.Validation("confirm_token", "confirm_token_invalid",
		"this confirm_token does not confirm this change: it is used, expired, or was issued for a different selection; preview the change again")
)

// bulkBinding hashes what a confirmation covers: who asked, the record type,
// the verb, the new owner and every item with its version, in id order.
func bulkBinding(requestedBy string, change bulkChange) ([]byte, error) {
	items := slices.Clone(change.items)
	slices.SortFunc(items, func(a, b crmcontracts.BulkItem) int { return strings.Compare(a.Id.String(), b.Id.String()) })
	encoded, err := json.Marshal(struct {
		RequestedBy string                      `json:"requested_by"`
		RecordType  crmcontracts.BulkRecordType `json:"record_type"`
		Verb        crmcontracts.BulkVerb       `json:"verb"`
		OwnerID     *ids.UUID                   `json:"owner_id"`
		Items       []crmcontracts.BulkItem     `json:"items"`
	}{requestedBy, change.recordType, change.verb, change.ownerID, items})
	if err != nil {
		return nil, fmt.Errorf("hash the previewed change: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return sum[:], nil
}

// mintBulkConfirmation stores a confirmation for change and answers the token
// and when it stops being accepted.
func mintBulkConfirmation(ctx context.Context, tx pgx.Tx, change bulkChange, now time.Time) (string, time.Time, error) {
	requestedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return "", time.Time{}, err
	}
	binding, err := bulkBinding(requestedBy, change)
	if err != nil {
		return "", time.Time{}, err
	}
	secret := make([]byte, bulkConfirmTokenBytes)
	if _, err := rand.Read(secret); err != nil {
		return "", time.Time{}, fmt.Errorf("draw a confirmation token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(secret)
	expires := now.Add(bulkConfirmFor)
	// A lapsed confirmation opens nothing, so each new one clears them away
	// rather than a sweep of its own doing it.
	if _, err := tx.Exec(ctx, `DELETE FROM bulk_confirmation WHERE expires_at <= $1`, now); err != nil {
		return "", time.Time{}, fmt.Errorf("clear lapsed confirmations: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO bulk_confirmation (token_hash, requested_by, binding, expires_at, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		tokenHash(token), requestedBy, binding, expires, now); err != nil {
		return "", time.Time{}, fmt.Errorf("store the confirmation: %w", err)
	}
	return token, expires, nil
}

// spendBulkConfirmation consumes the confirmation token names, refusing one
// that does not confirm exactly change for this caller.
func spendBulkConfirmation(ctx context.Context, tx pgx.Tx, change bulkChange, token string, now time.Time) error {
	requestedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	binding, err := bulkBinding(requestedBy, change)
	if err != nil {
		return err
	}
	var spent bool
	err = tx.QueryRow(ctx,
		`UPDATE bulk_confirmation SET consumed_at = $4
		  WHERE token_hash = $1 AND requested_by = $2 AND binding = $3
		    AND consumed_at IS NULL AND expires_at > $4
		 RETURNING true`,
		tokenHash(token), requestedBy, binding, now).Scan(&spent)
	if errors.Is(err, pgx.ErrNoRows) {
		return errConfirmTokenRefused
	}
	if err != nil {
		return fmt.Errorf("spend the confirmation: %w", err)
	}
	return nil
}

func tokenHash(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
