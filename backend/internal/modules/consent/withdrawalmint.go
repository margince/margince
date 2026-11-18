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
// ok is false only when a credential already covers this address and scope, in
// which case the previous link still works and there is nothing new to write
// into the mail. The caller keeps the header it would otherwise have built.
//
// purpose is the key the links must name: the catalogued key for a named
// credential, and empty for an all-marketing one. A link naming a key the
// credential does not carry is a link whose page cannot find what it stops.
func (s *Store) WithdrawalTokenForEmail(
	ctx context.Context, recipientEmail, purposeKey string,
) (token, purpose string, ok bool, err error) {
	address := normalizeAddress(recipientEmail)
	if address == "" {
		return "", "", false, nil
	}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		in := WithdrawalMintInput{Address: address, Scope: WithdrawalScopeAllMarketing}
		if err := bindWithdrawalSubject(ctx, tx, &in); err != nil {
			return err
		}
		// A key no purpose carries falls back to all-marketing: the wider of
		// the two, so the link never stops less than the recipient expects.
		purposeID, found, err := cataloguedPurposeTx(ctx, tx, purposeKey)
		if err != nil {
			return fmt.Errorf("consent: resolving the purpose for the unsubscribe link: %w", err)
		}
		if found {
			in.Scope, in.PurposeID = WithdrawalScopeNamedPurpose, purposeID
			purpose = normalizedPurposeKey(purposeKey)
		}
		var mintErr error
		token, mintErr = s.ensureWithdrawalCredentialForSendTx(ctx, tx, in)
		return mintErr
	})
	if err != nil {
		return "", "", false, err
	}
	return token, purpose, token != "", nil
}

// cataloguedPurposeTx answers the purpose a key names, archived or not, and
// found=false for an empty key or one the catalog never carried.
//
// The mint and the press both ask it, so a key the mint widened to
// all-marketing is a key the press widens too.
func cataloguedPurposeTx(ctx context.Context, tx pgx.Tx, key string) (ids.UUID, bool, error) {
	key = normalizedPurposeKey(key)
	if key == "" {
		return ids.UUID{}, false, nil
	}
	var id ids.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM consent_purpose WHERE key = $1`, key).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.UUID{}, false, nil
	}
	if err != nil {
		return ids.UUID{}, false, err
	}
	return id, true, nil
}

// purposeCatalogued is cataloguedPurposeTx for a caller outside a transaction.
func (s *Store) purposeCatalogued(ctx context.Context, key string) (bool, error) {
	var found bool
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		_, found, err = cataloguedPurposeTx(ctx, tx, key)
		return err
	})
	return found, err
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
