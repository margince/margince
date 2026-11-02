// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/credentialmask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MailMaskPage is what one page of the repair scanned and changed, and the id
// the next page starts after.
type MailMaskPage struct {
	Scanned int
	Changed int
	Last    ids.UUID
}

type maskedMail struct {
	id          ids.UUID
	subject     string
	body        string
	nextSubject string
	nextBody    string
}

// MaskMailCredentialsTx masks the credentials stored in one page of mail text.
//
// Mail captured or sent before the capture-side mask still holds passwords and
// access keys in clear, where a reader of the record or a search over it finds
// them. Every kind='email' row is in scope, because the capture path and the
// send path both wrote text this way.
//
// The audit row carries no text, as at capture. An image of the body would
// write the credential into a row that nothing can take it out of.
func MaskMailCredentialsTx(
	ctx context.Context, tx pgx.Tx, after ids.UUID, limit int,
) (MailMaskPage, error) {
	page, rows, err := readMailToMask(ctx, tx, after, limit)
	if err != nil {
		return MailMaskPage{}, err
	}
	for _, row := range rows {
		changed, err := maskOneMail(ctx, tx, row)
		if err != nil {
			return MailMaskPage{}, err
		}
		if changed {
			page.Changed++
		}
	}
	return page, nil
}

// readMailToMask keeps only the rows a mask would change, and advances the
// cursor over every row it read: a page of clean mail must not be read again.
func readMailToMask(
	ctx context.Context, tx pgx.Tx, after ids.UUID, limit int,
) (MailMaskPage, []maskedMail, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, coalesce(subject, ''), coalesce(body, '')
		  FROM activity
		 WHERE kind = 'email' AND id > $1
		 ORDER BY id
		 LIMIT $2`, after, limit)
	if err != nil {
		return MailMaskPage{}, nil, fmt.Errorf("activities: reading mail to mask: %w", err)
	}
	defer rows.Close()
	page := MailMaskPage{Last: after}
	var changed []maskedMail
	for rows.Next() {
		var row maskedMail
		if err := rows.Scan(&row.id, &row.subject, &row.body); err != nil {
			return MailMaskPage{}, nil, fmt.Errorf("activities: reading a mail to mask: %w", err)
		}
		page.Scanned++
		page.Last = row.id
		row.nextSubject = credentialmask.MaskText(row.subject)
		row.nextBody = credentialmask.MaskText(row.body)
		if row.nextSubject != row.subject || row.nextBody != row.body {
			changed = append(changed, row)
		}
	}
	if err := rows.Err(); err != nil {
		return MailMaskPage{}, nil, fmt.Errorf("activities: reading mail to mask: %w", err)
	}
	return page, changed, nil
}

// maskOneMail writes the masked text, the trail that says it was masked, and
// the event that rebuilds what was derived from the old text.
func maskOneMail(ctx context.Context, tx pgx.Tx, row maskedMail) (bool, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE activity SET subject = nullif($2, ''), body = nullif($3, '')
		 WHERE id = $1 AND kind = 'email'`, row.id, row.nextSubject, row.nextBody)
	if err != nil {
		return false, fmt.Errorf("activities: masking the credentials in mail %s: %w", row.id, err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	// AuditEvent, not Audit: the before-image of these fields IS the credential
	// this removes, and audit_log is append-only, so recording it would put the
	// value somewhere nothing can take it out of again.
	auditID, err := storekit.AuditEvent(ctx, tx, "update", "activity", row.id,
		map[string]any{"stored_credentials_masked": true})
	if err != nil {
		return false, err
	}
	masked := true
	fields := crmcontracts.PublicEventActivityChangedFields{}
	if row.nextBody != row.body {
		fields.Body = &masked
	}
	if row.nextSubject != row.subject {
		// The contract names the new subject, and the masked one is what it is
		// safe to carry: the value it replaces is the credential.
		fields.Subject = &row.nextSubject
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, row.id,
		crmcontracts.PublicEventActivityUpdated{ChangedFields: fields}); err != nil {
		return false, err
	}
	return true, nil
}
