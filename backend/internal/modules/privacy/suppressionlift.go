// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The erasure list's two writes: an erasure arms an entry, and the erased
// subject alone lifts it, by answering a link mailed to the address. A lift is a
// column, not a DELETE, so a restore still erases what the subject had before.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ErrLiftNotCurrent refuses a lift whose link no longer matches the list. The
// address was lifted already, was never erased, or was erased again since the
// link was mailed. A link lifts one erasure once.
var ErrLiftNotCurrent = errors.New("privacy: the erasure this link was mailed for is no longer on the list")

// replayingErasureKey marks an erasure that ReapplySuppressions runs again on a
// record the list already names. It repeats the old erasure, so it leaves the
// row as it stands, lift included.
type replayingErasureKey struct{}

func replayingErasure(ctx context.Context) context.Context {
	return context.WithValue(ctx, replayingErasureKey{}, true)
}

// armSuppression puts identifier hashes on the erasure list for an erasure.
// Every erasure dates the row from now and clears any lift, so a confirmation
// link mailed before it no longer matches the row.
func armSuppression(ctx context.Context, tx pgx.Tx, kind string, hashes []string, keyID *string) error {
	if len(hashes) == 0 {
		return nil
	}
	replaying, _ := ctx.Value(replayingErasureKey{}).(bool)
	if _, err := tx.Exec(ctx, `
		INSERT INTO erasure_suppression (kind, value_hash, key_id)
		SELECT DISTINCT @kind::text, hash, @key::text FROM unnest(@hashes::text[]) AS hash
		ON CONFLICT (kind, value_hash) DO UPDATE
		   SET lifted_at = NULL, lift_consent_event_id = NULL, lift_contact_id = NULL, created_at = now()
		 WHERE NOT @replaying`,
		pgx.StrictNamedArgs{"kind": kind, "hashes": hashes, "key": keyID, "replaying": replaying}); err != nil {
		return fmt.Errorf("privacy: putting %s identifiers on the erasure list: %w", kind, err)
	}
	return nil
}

// EmailErasedAt answers when the address was last erased, and false when no
// entry refuses it now. A confirmation link carries this time, so a link
// mailed for one erasure cannot lift the next one.
func EmailErasedAt(ctx context.Context, tx pgx.Tx, email string) (time.Time, bool, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return time.Time{}, false, err
	}
	var erasedAt *time.Time
	if err := tx.QueryRow(ctx, `
		SELECT max(created_at) FROM erasure_suppression
		 WHERE kind = 'email' AND value_hash = ANY(@forms) AND lifted_at IS NULL`,
		pgx.StrictNamedArgs{"forms": storekit.EmailSuppressionForms(email)}).Scan(&erasedAt); err != nil {
		return time.Time{}, false, fmt.Errorf("privacy: reading when an address was erased: %w", err)
	}
	if erasedAt == nil {
		return time.Time{}, false, nil
	}
	return *erasedAt, true, nil
}

// LiftEmailSuppressionTx lifts every entry that refuses the address, on the
// subject's own request. erasedAt is the time their link carries. consentEvent
// is the grant they gave in the same transaction, and contact is the record
// that request stored, which a replay never erases. It takes the lock an
// erasure takes, so an erasure and a lift of one address never interleave.
func LiftEmailSuppressionTx(ctx context.Context, tx pgx.Tx, email string, erasedAt time.Time, consentEvent, contact ids.UUID) error {
	if err := storekit.LockSubjectKeys(ctx, tx, nil, []string{email}); err != nil {
		return err
	}
	current, erased, err := EmailErasedAt(ctx, tx, email)
	if err != nil {
		return err
	}
	if !erased || current.UnixMicro() != erasedAt.UnixMicro() {
		return ErrLiftNotCurrent
	}
	if _, err := tx.Exec(ctx, `
		UPDATE erasure_suppression SET lifted_at = now(), lift_consent_event_id = @consent, lift_contact_id = @contact
		 WHERE kind = 'email' AND value_hash = ANY(@forms) AND lifted_at IS NULL`,
		pgx.StrictNamedArgs{"forms": storekit.EmailSuppressionForms(email), "consent": consentEvent, "contact": contact}); err != nil {
		return fmt.Errorf("privacy: lifting an address from the erasure list: %w", err)
	}
	return nil
}
