// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const whatsappProvider = "whatsapp"

func whatsAppPhone(ci connector.ChannelIdentity) string {
	if ci.Provider != whatsappProvider || ci.AccountScope != "" {
		return ""
	}
	for _, digit := range ci.ChannelUserID {
		if digit < '0' || digit > '9' {
			return ""
		}
	}
	phone, err := values.ParsePhone("+" + ci.ChannelUserID)
	if err != nil {
		return ""
	}
	return phone.String()
}

// The contact lock protects the fill from erasure. Only an account named by a
// retained provider original proves a phone; a colleague's answer alone does not.
func materializeChannelPhone(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, ci connector.ChannelIdentity) (bool, error) {
	phone := whatsAppPhone(ci)
	if phone == "" {
		return false, nil
	}
	var eligible bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM contact_channel_identity WHERE contact_id=@contact AND provider='whatsapp' AND account_scope='' AND channel_user_id=@user AND archived_at IS NULL)
 AND NOT EXISTS(SELECT 1 FROM contact_phone WHERE contact_id=@contact AND phone=@phone)`, pgx.NamedArgs{contactKey: contactID, "user": ci.ChannelUserID, FactPhone: phone}).Scan(&eligible); err != nil {
		return false, err
	}
	if !eligible {
		return false, nil
	}
	var original ids.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM raw_capture WHERE source_system='whatsapp' AND channel_subjects IS NOT NULL AND channel_subjects @> @subjects::text[] LIMIT 1`, pgx.NamedArgs{"subjects": []string{storekit.ChannelRawSubjectKey(ci.Provider, ci.AccountScope, ci.ChannelUserID)}}).Scan(&original)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return addChannelPhone(ctx, tx, contactID, phone, original)
}

// Account evidence adds a number and never supersedes one by country or date.
// Dated signatures use applyObservedNumbers because they can replace older numbers.
func addChannelPhone(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, phone string, original ids.UUID) (bool, error) {
	if err := auth.HoldWritableLive(ctx, tx, contactEntity, contactID.UUID); err != nil {
		return false, err
	}
	restricted, err := storekit.RestrictedContacts(ctx, tx, []ids.UUID{contactID.UUID})
	if err != nil {
		return false, err
	}
	if restricted[contactID.UUID] {
		return false, nil
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return false, err
	}
	seen, err := seeContactSets(ctx, tx, contactID)
	if err != nil {
		return false, err
	}
	numbers := []crmcontracts.ContactPhone{}
	if seen.was.Phones != nil {
		numbers = *seen.was.Phones
	}
	primary, position := true, 0
	for _, number := range numbers {
		if number.IsPrimary {
			primary = false
		}
		position = max(position, number.Position+1)
	}
	if err := insertContactPhones(ctx, tx, contactID, whatsappProvider, by, []ContactPhoneInput{{Phone: phone, PhoneType: "other", IsPrimary: primary, Position: position}}); err != nil {
		return false, err
	}
	if err := touchChildSetParent(ctx, tx, contactEntity, contactID.UUID); err != nil {
		return false, err
	}
	ref := original.String()
	if err := storekit.StampFields(ctx, tx, contactEntity, contactID.UUID, whatsappProvider, by, []storekit.FieldStamp{{Field: fieldPhone, EvidenceRef: &ref}}); err != nil {
		return false, err
	}
	before, after := map[string]any{}, map[string]any{}
	if err := seen.recordContactSetChange(ctx, tx, contactID, before, after); err != nil {
		return false, err
	}
	if err := auditChannelIdentityChange(ctx, tx, contactID, map[string]any{auditKeyPhones: before[auditKeyPhones]}, map[string]any{auditKeyPhones: after[auditKeyPhones]}); err != nil {
		return false, err
	}
	return true, nil
}
