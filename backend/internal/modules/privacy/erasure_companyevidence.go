// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// companyDetailSQL spells a value of the given field column the way the match
// compares it: an address lowercased, a number as its digits.
func companyDetailSQL(field, value string) string {
	return `CASE ` + field + ` WHEN 'contact_email' THEN lower(btrim(` + value + `))
	        ELSE regexp_replace(` + value + `, '[^0-9]', '', 'g') END`
}

// matchedDetailSQL holds while detail d may match a company fact. That is an
// address with an @, or a number long enough to name one line.
const matchedDetailSQL = `((d.field = 'contact_email' AND d.detail LIKE '_%@_%')
	OR (d.field = 'phone' AND length(d.detail) >= 6))`

// purgeSubjectCompanyEvidence deletes the company facts that state the erased
// subject's own address or number. A site read may file an employee's direct
// line or mailbox as the company's phone or contact_email. No key leads from
// that row to the contact. A company number that is not the subject's stays.
// It reads the subject's identifier rows and evidence, so it runs first.
func purgeSubjectCompanyEvidence(ctx context.Context, tx pgx.Tx, contactID ids.UUID) error {
	if _, err := tx.Exec(ctx, `WITH d AS (
		  SELECT 'contact_email' AS field, lower(btrim(email)) AS detail FROM contact_email WHERE contact_id = $1
		  UNION SELECT 'phone', regexp_replace(phone, '[^0-9]', '', 'g') FROM contact_phone WHERE contact_id = $1
		  UNION SELECT CASE field WHEN 'email' THEN 'contact_email' ELSE 'phone' END,
		               CASE field WHEN 'email' THEN lower(btrim(value))
		                    ELSE regexp_replace(value, '[^0-9]', '', 'g') END
		          FROM contact_profile_field WHERE contact_id = $1 AND field IN ('email', 'phone')),
		 current_facts AS (
		  DELETE FROM field_provenance f USING d
		   WHERE f.object_type = 'company' AND f.value IS NOT NULL AND f.field_name = d.field
		     AND `+matchedDetailSQL+` AND `+companyDetailSQL("f.field_name", "f.value")+` = d.detail)
		DELETE FROM company_fact_legacy f USING d
		 WHERE f.field = d.field AND `+matchedDetailSQL+` AND `+companyDetailSQL("f.field", "f.value")+` = d.detail`,
		contactID); err != nil {
		return fmt.Errorf("privacy: deleting the company facts that state the subject's details: %w", err)
	}
	return nil
}
