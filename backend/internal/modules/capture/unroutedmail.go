// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Mail in a member's own mailbox that was sent to addresses that are not theirs.
//
// Such a message has no capture_import row, so no confidentiality question is
// opened for it and nothing judges it. The owner can read these mails already.
// "I receive this address" is the ordinary own-address claim
// (OwnerIdentityStore.AddConfirmed), and the stranded-mail sweep then imports
// the mail. "Not mine" is a row in capture_alias_refused.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// UnroutedAddress is one address the caller's unjudged mail was sent to.
type UnroutedAddress struct {
	Address  string
	Messages int
}

// UnroutedMail is the caller's unjudged mail, counted per address.
type UnroutedMail struct {
	// Messages counts each message once, however many listed addresses it names.
	Messages  int
	Addresses []UnroutedAddress
	// More is true when the list was cut: more addresses than UnroutedAddressCap,
	// or more mail than UnroutedScanCap, the newest of which was read.
	More bool
}

// The bounds that keep one read short on a mailbox with a long history.
const (
	UnroutedAddressCap = 50
	UnroutedScanCap    = 5000
)

// unroutedMessagesSQL selects the newest of the caller's mail that no seat
// imported and that names none of the caller's own addresses. $1 is the caller,
// $2 their own addresses and $3 the scan cap. The captured_by test keeps it to
// mail the caller's own connector brought in, so a colleague's capture is never
// counted. Mail the caller sent names its recipients as parties, not as
// addresses they failed to receive.
const unroutedMessagesSQL = `
	SELECT a.id FROM activity a
	 WHERE a.kind = 'email' AND a.archived_at IS NULL AND a.restricted_at IS NULL
	   AND a.direction IS DISTINCT FROM 'outbound'
	   AND a.captured_by IN (
	         SELECT 'connector:' || c.provider || ':' || c.user_id FROM capture_connection c
	          WHERE c.user_id = $1)
	   AND NOT EXISTS (SELECT 1 FROM capture_import i WHERE i.activity_id = a.id)
	   AND NOT EXISTS (
	         SELECT 1 FROM activity_participant own
	          WHERE own.activity_id = a.id AND own.address = ANY($2))
	 ORDER BY a.occurred_at DESC LIMIT $3`

// unroutedCountsSQL counts the scanned messages per address the caller has not
// refused, most mail first, and carries the totals on every row.
const unroutedCountsSQL = `
	WITH scanned AS (` + unroutedMessagesSQL + `),
	recipients AS (
	  SELECT p.activity_id, p.address
	    FROM activity_participant p JOIN scanned s ON s.id = p.activity_id
	   WHERE p.role IN ('to', 'cc', 'bcc') AND p.address IS NOT NULL
	     AND NOT EXISTS (
	           SELECT 1 FROM capture_alias_refused r WHERE r.user_id = $1 AND r.value = p.address)),
	per_address AS (
	  SELECT address, count(DISTINCT activity_id) AS messages FROM recipients GROUP BY address)
	SELECT address, messages,
	       (SELECT count(DISTINCT activity_id) FROM recipients),
	       (SELECT count(*) FROM per_address),
	       (SELECT count(*) FROM scanned)
	  FROM per_address ORDER BY messages DESC, address LIMIT $4`

// UnroutedMail answers the caller's own unjudged mail and nobody else's.
func (s *OwnerIdentityStore) UnroutedMail(ctx context.Context) (UnroutedMail, error) {
	actor, err := seatItself(ctx)
	if err != nil {
		return UnroutedMail{}, err
	}
	var out UnroutedMail
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		self, err := readOwnerIdentitiesTx(ctx, tx, actor.UserID)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, unroutedCountsSQL,
			actor.UserID, self.Addresses(), UnroutedScanCap, UnroutedAddressCap)
		if err != nil {
			return fmt.Errorf("capture: counting the mail sent to addresses that are not yours: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var row UnroutedAddress
			var addresses, scanned int
			if err := rows.Scan(&row.Address, &row.Messages, &out.Messages, &addresses, &scanned); err != nil {
				return fmt.Errorf("capture: reading the mail sent to addresses that are not yours: %w", err)
			}
			out.More = addresses > UnroutedAddressCap || scanned >= UnroutedScanCap
			out.Addresses = append(out.Addresses, row)
		}
		return rows.Err()
	})
	return out, err
}

// RefuseUnroutedAddress records that the caller does not receive this address.
// Its mail stays unjudged and leaves the count. Idempotent.
func (s *OwnerIdentityStore) RefuseUnroutedAddress(ctx context.Context, raw string) error {
	return s.setAddressRefusal(ctx, raw, true)
}

// RestoreUnroutedAddress undoes RefuseUnroutedAddress.
func (s *OwnerIdentityStore) RestoreUnroutedAddress(ctx context.Context, raw string) error {
	return s.setAddressRefusal(ctx, raw, false)
}

// setAddressRefusal writes the row, then its audit row and event in the same
// transaction. It writes nothing when the row is already as asked. The address
// itself stays out of the trail, as it does for an owner identity.
func (s *OwnerIdentityStore) setAddressRefusal(ctx context.Context, raw string, refused bool) error {
	actor, err := seatItself(ctx)
	if err != nil {
		return err
	}
	value, err := validRuleValue(identityKinds, IdentityKindAddress, raw)
	if err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		query := `DELETE FROM capture_alias_refused WHERE user_id = $1 AND value = $2`
		if refused {
			query = `INSERT INTO capture_alias_refused (user_id, value) VALUES ($1, $2)
				ON CONFLICT (user_id, value) DO NOTHING`
		}
		tag, err := tx.Exec(ctx, query, actor.UserID, value)
		if err != nil {
			return fmt.Errorf("capture: recording that an address is not yours: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		workspace := storekit.MustWorkspace(ctx)
		auditID, err := storekit.Audit(ctx, tx, "update", captureSettingsObject, workspace,
			map[string]any{auditKeyKind: IdentityKindAddress, "refused": !refused},
			map[string]any{auditKeyKind: IdentityKindAddress, "refused": refused})
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, auditID, workspace,
			crmcontracts.InternalEventCaptureSettingsUpdated{ChangedKeys: []string{unroutedRefusalKey}})
	})
}

// unroutedRefusalKey names the setting an event says changed. It is a key, so
// the event carries no address.
const unroutedRefusalKey = "unrouted_mail_refusal"
