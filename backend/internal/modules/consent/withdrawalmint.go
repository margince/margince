// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Minting the withdrawal link a message carries.
//
// Split from withdrawalcredential.go, which holds the credential itself: this
// file is the SEND PATH's view of it — resolve the address to whoever holds
// it, and hand back a token the List-Unsubscribe header can carry.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// WithdrawalTokenForEmail mints (or reuses) the withdrawal credential this
// message's one-click link will carry.
//
// IT RESOLVES A LEAD TOO, which is the second half of the defect this file
// closes. PreferenceTokenForEmail resolves contacts only, so a lead-only
// recipient's marketing mail goes out with no List-Unsubscribe header at all —
// the send path treats "no token" as "no unsubscribe surface" and carries none.
//
// An address matching NOBODY still gets a credential. The message is going to
// that address regardless, and an opt-out is about where the mail went; a
// recipient we cannot name is exactly the one least able to ask a human.
//
// effectivePurpose is the key the credential is scoped to, or "" when it covers
// all marketing. The links are built from it, so they name what the press stops.
//
// ok is false only when a credential already covers this address and scope, in
// which case the previous link still works and there is nothing new to write
// into the mail. The caller keeps the header it would otherwise have built.
func (s *Store) WithdrawalTokenForEmail(
	ctx context.Context, recipientEmail, purposeKey string,
) (token, effectivePurpose string, ok bool, err error) {
	address := normalizeAddress(recipientEmail)
	if address == "" {
		return "", "", false, nil
	}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		effectivePurpose = ""
		in := WithdrawalMintInput{Address: address, Scope: WithdrawalScopeAllMarketing}
		if err := bindWithdrawalSubject(ctx, tx, &in); err != nil {
			return err
		}
		if purposeKey != "" {
			// Matched the way authorization matches a key, live purposes only: a
			// press on an archived purpose stops nothing, so it must not be named.
			var purposeID ids.UUID
			var storedKey string
			switch scanErr := tx.QueryRow(ctx,
				`SELECT id, key FROM consent_purpose WHERE lower(btrim(key)) = $1 AND archived_at IS NULL`,
				commsauthz.NormalizedPurposeKey(purposeKey)).Scan(&purposeID, &storedKey); {
			case errors.Is(scanErr, pgx.ErrNoRows):
				// A key no live purpose carries: the all-marketing scope is the
				// honest fallback, and it is the wider of the two, so the link
				// never stops less than the recipient expects.
			case scanErr != nil:
				return fmt.Errorf("consent: resolving the purpose for the unsubscribe link: %w", scanErr)
			default:
				in.Scope, in.PurposeID = WithdrawalScopeNamedPurpose, purposeID
				effectivePurpose = storedKey
			}
		}
		var mintErr error
		token, mintErr = s.ensureWithdrawalCredentialForSendTx(ctx, tx, in)
		return mintErr
	})
	if err != nil {
		return "", "", false, err
	}
	return token, effectivePurpose, token != "", nil
}

// bindWithdrawalSubject attaches the record holding this address, when one
// does. A contact wins over a lead: a promoted lead's mail is the contact's.
//
// AMBIGUITY LEAVES THE CREDENTIAL UNBOUND rather than picking. Two live
// records on one address is a data problem, and choosing between them by row
// order would stamp one contact's id on the other's opt-out link. The credential
// still works — it names the address, which is what the withdrawal acts on.
func bindWithdrawalSubject(ctx context.Context, tx pgx.Tx, in *WithdrawalMintInput) error {
	// count(*) rather than a LIMIT and a Scan: a bare read of the first row is
	// the silent pick this function exists not to make, and it looks identical
	// to the unambiguous case at the call site.
	contact, err := theOneSubjectHolding(ctx, tx, `
		SELECT pe.contact_id
		  FROM contact_email pe
		  JOIN contact p ON p.id = pe.contact_id AND p.archived_at IS NULL
		 WHERE lower(pe.email) = $1 AND pe.archived_at IS NULL`, in.Address)
	if err != nil {
		return fmt.Errorf("consent: resolving the contact the unsubscribe link is for: %w", err)
	}
	if !contact.IsZero() {
		in.ContactID = ids.From[ids.ContactKind](contact)
		return nil
	}
	lead, err := theOneSubjectHolding(ctx, tx, `
		SELECT id FROM lead
		 WHERE lower(email) = $1 AND archived_at IS NULL AND merged_into_id IS NULL`, in.Address)
	if err != nil {
		return fmt.Errorf("consent: resolving the lead the unsubscribe link is for: %w", err)
	}
	if !lead.IsZero() {
		in.LeadID = ids.From[ids.LeadKind](lead)
	}
	return nil
}

// theOneSubjectHolding returns the single record this query matches, or the
// zero id when it matches none OR SEVERAL.
//
// Several is deliberately the same answer as none. Stamping one of two
// candidates onto the credential would put one contact's id on the other's
// opt-out link, and the link works either way: it names the address, which is
// what the withdrawal acts on.
func theOneSubjectHolding(ctx context.Context, tx pgx.Tx, query, address string) (ids.UUID, error) {
	rows, err := tx.Query(ctx, query+" LIMIT 2", address)
	if err != nil {
		return ids.UUID{}, err
	}
	matches, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return ids.UUID{}, err
	}
	if len(matches) != 1 {
		return ids.UUID{}, nil
	}
	return matches[0], nil
}
