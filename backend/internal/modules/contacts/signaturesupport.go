// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Which mails still state a value a signature filled. A further address and a
// postal address are kept while some mail supports them: each supporting mail
// is a key of the evidence row's signature_support, with the snippet it quoted
// and whether it also vouched for the employer's street.

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/salutation"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// quotesValue holds when the snippet prints the value, case and spacing aside.
// A value the snippet does not print is the model's, not the contact's.
func quotesValue(snippet, value string) bool {
	fold := func(s string) string { return strings.Join(strings.Fields(strings.ToLower(s)), " ") }
	return value != "" && strings.Contains(fold(snippet), fold(value))
}

// quotesAddress holds when the snippet prints the address as a whole token,
// so a longer address that ends in it does not count.
func quotesAddress(snippet, address string) bool {
	return slices.Contains(salutation.PrintedAddresses(snippet), strings.ToLower(address))
}

// addSignatureSupportTx records that sourceRef states the value of one
// signature evidence row, and reports whether such a row holds it.
func addSignatureSupportTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, field, valueKey, value, sourceRef, snippet string) (bool, error) {
	tag, err := tx.Exec(ctx, `UPDATE contact_profile_field
		   SET signature_support = signature_support || jsonb_build_object($5::text,
		       jsonb_build_object('snippet', $6::text,
		         'street', coalesce((signature_support -> $5::text ->> 'street')::boolean, false)))
		 WHERE contact_id = $1 AND field = $2 AND value_key = $3 AND value = $4 AND source = $7`,
		contactID, field, valueKey, value, sourceRef, snippet, enrichSource)
	if err != nil {
		return false, fmt.Errorf("contacts: recording the mail that states a signature's %s: %w", field, err)
	}
	return tag.RowsAffected() > 0, nil
}

// markStreetSupportTx records that sourceRef also vouched for the employer's
// street the contact's address row states.
func markStreetSupportTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, sourceRef string) error {
	if _, err := tx.Exec(ctx, `UPDATE contact_profile_field
		   SET signature_support = jsonb_set(signature_support, ARRAY[$2::text, 'street'], 'true'::jsonb)
		 WHERE contact_id = $1 AND field = $3 AND source = $4 AND signature_support ? $2::text`,
		contactID, sourceRef, fieldAddress, enrichSource); err != nil {
		return fmt.Errorf("contacts: recording the mail that states an employer's street: %w", err)
	}
	return nil
}

// dropSignatureSupportTx removes one narrowed mail's support. A row another
// mail still supports moves its evidence onto that mail. A row left with no
// support keeps the narrowed mail as its source, so the take-back finds it. An
// address row that lost its last street support takes the street back.
func dropSignatureSupportTx(ctx context.Context, tx pgx.Tx, sourceRef string) error {
	rows, err := tx.Query(ctx, `WITH dropped AS (
		  SELECT f.id, f.signature_support - $1::text AS rest,
		         coalesce((f.signature_support -> $1::text ->> 'street')::boolean, false) AS backed_street
		    FROM contact_profile_field f WHERE f.signature_support ? $1::text AND f.source = $2 FOR UPDATE),
		 next AS (
		  SELECT d.id, d.rest, d.backed_street, (SELECT min(k) FROM jsonb_object_keys(d.rest) k) AS ref FROM dropped d)
		UPDATE contact_profile_field f
		   SET signature_support = n.rest,
		       source_ref = CASE WHEN f.source_ref = $1 AND n.ref IS NOT NULL THEN n.ref ELSE f.source_ref END,
		       evidence_snippet = CASE WHEN f.source_ref = $1 AND n.ref IS NOT NULL
		                               THEN n.rest -> n.ref ->> 'snippet' ELSE f.evidence_snippet END
		  FROM next n
		 WHERE f.id = n.id
		RETURNING f.contact_id, f.value, f.field = $3 AND n.backed_street AND n.ref IS NOT NULL
		   AND NOT EXISTS (SELECT 1 FROM jsonb_each(n.rest) e WHERE (e.value ->> 'street')::boolean)`,
		sourceRef, enrichSource, fieldAddress)
	if err != nil {
		return fmt.Errorf("contacts: dropping a narrowed mail's signature support: %w", err)
	}
	type dropped struct {
		ContactID  ids.ContactID
		Value      string
		StreetLost bool
	}
	lost, err := pgx.CollectRows(rows, pgx.RowToStructByPos[dropped])
	if err != nil {
		return fmt.Errorf("contacts: dropping a narrowed mail's signature support: %w", err)
	}
	for _, row := range lost {
		if !row.StreetLost {
			continue
		}
		if err := withdrawEmployerStreetTx(ctx, tx, row.Value, row.ContactID); err != nil {
			return err
		}
	}
	return nil
}
