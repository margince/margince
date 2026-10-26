// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// A card matched to a contact who already exists moves the contact's address
// parts only where no other writer owns them.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// cardOwnedContactAddress reads a contact's address and whether a card may
// replace it. It may when the address is empty, or when only the vCard import
// ever wrote it. The proof is the import's own audit evidence, which no request
// can set. It needs the create or address fill the import made, and no address
// edit by any other writer since.
const cardOwnedContactAddress = `SELECT address_line1, address_line2, address_city, address_region,
	       address_postal_code, address_country,
	       (address_line1 IS NULL AND address_line2 IS NULL AND address_city IS NULL AND address_region IS NULL
	        AND address_postal_code IS NULL AND address_country IS NULL)
	       OR (EXISTS (SELECT 1 FROM audit_log a
	                    WHERE a.entity_type = 'contact' AND a.entity_id = c.id AND a.evidence ->> 'source' = $2
	                      AND (a.action = 'create' OR a.after ? 'address_line1'))
	           AND NOT EXISTS (SELECT 1 FROM audit_log a
	                    WHERE a.entity_type = 'contact' AND a.entity_id = c.id AND a.after ? 'address_line1'
	                      AND a.evidence ->> 'source' IS DISTINCT FROM $2))
	  FROM contact c WHERE c.id = $1`

// fillAddressFromCard writes the card's address parts onto a matched contact,
// with the audit evidence a later card reads to know the address is a card's.
// It reports what it did, or an empty action when the card states no address.
func fillAddressFromCard(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, entry VCardEntry, corrected bool) (VCardFieldAction, error) {
	stated := vcardRecordAddress(entry)
	switch {
	case stated == nil:
		return "", nil
	case corrected:
		return VCardFieldKept, nil
	}
	if err := auth.Require(ctx, entityContact, principal.ActionRead); err != nil {
		return "", err
	}
	if err := auth.EnsureWritableLive(ctx, tx, entityContact, contactID.UUID); err != nil {
		return "", err
	}
	lock, err := storekit.LockRow(ctx, tx, entityContact, contactID.UUID, storekit.LiveOnly)
	if err != nil {
		return "", err
	}
	var current crmcontracts.Address
	var replaceable bool
	if err := tx.QueryRow(ctx, cardOwnedContactAddress, contactID, vcardSource).Scan(
		&current.Line1, &current.Line2, &current.City, &current.Region,
		&current.PostalCode, &current.Country, &replaceable); err != nil {
		return "", fmt.Errorf("contacts: reading the address a card would replace: %w", err)
	}
	if !replaceable {
		return VCardFieldKept, nil
	}
	held := addressOrNil(current) != nil
	p := storekit.NewPatch()
	if err := setAddress(p, &current, stated, storekit.AuditTrail{}); err != nil {
		return "", err
	}
	if len(p.Moved()) == 0 {
		return VCardFieldImported, nil
	}
	if err := p.ApplyLocked(ctx, tx, lock); err != nil {
		return "", fmt.Errorf("contacts: writing a card's address: %w", err)
	}
	auditID, err := storekit.AuditWithEvidence(ctx, tx, "update", entityContact, contactID.UUID,
		p.Before(), p.After(), map[string]any{auditKeySource: vcardSource, auditKeySourceRef: vcardSource})
	if err != nil {
		return "", err
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, contactID.UUID, crmcontracts.PublicEventContactUpdated{ChangedFields: p.After()}); err != nil {
		return "", err
	}
	if held {
		return VCardFieldReplaced, nil
	}
	return VCardFieldImported, nil
}
