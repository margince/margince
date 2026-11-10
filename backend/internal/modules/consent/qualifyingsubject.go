// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func qualifyingSubjectRef(ref subjectRef) (subject, bool, error) {
	if ref.Kind != entityContact && ref.Kind != entityLead {
		return subject{}, false, nil
	}
	id, err := ids.Parse(ref.ID)
	if err != nil {
		return subject{}, false, fmt.Errorf("consent: resolve the exchange subject: %w", err)
	}
	var target RecordInput
	if ref.Kind == entityContact {
		target.ContactID = ids.From[ids.ContactKind](id)
	} else {
		target.LeadID = ids.From[ids.LeadKind](id)
	}
	sub, err := consentSubject(target)
	return sub, err == nil, err
}

func subjectKindID(sub subject, kind string) ids.UUID {
	if sub.entityType == kind {
		return sub.id
	}
	return ids.UUID{}
}

func bindQualifyingAddresses(ctx context.Context, tx pgx.Tx, sub subject, others ...ids.UUID) error {
	subjects := append([]ids.UUID{sub.id}, others...)
	column := pgx.Identifier{sub.column}.Sanitize()
	addresses := storekit.RequestAddressesSQL("q.contact_id")
	if sub.entityType == entityLead {
		addresses = "(SELECT CASE WHEN email IS NULL THEN '{}'::text[] ELSE ARRAY[email] END FROM lead WHERE id = q.lead_id)"
	}
	_, err := tx.Exec(ctx, `UPDATE consent_qualifying_event q SET recorded_for_addresses = `+addresses+`
 WHERE q.`+column+`=ANY(@subjects) AND q.recorded_for_addresses IS NULL`, pgx.StrictNamedArgs{"subjects": subjects})
	if err != nil {
		return fmt.Errorf("consent: binding recorded exchanges to their addresses: %w", err)
	}
	return nil
}
