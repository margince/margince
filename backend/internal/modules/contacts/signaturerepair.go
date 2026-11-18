// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SignatureFieldSource is one field the signature pass wrote, with the stored
// body of the mail it was read from. Body is nil when that mail is gone,
// archived, restricted or emptied. NameEvidence is the line the same read
// took the name from. HeaderName is the display before the pass first renamed
// the contact, and Email their primary address.
type SignatureFieldSource struct {
	ID           ids.UUID
	ContactID    ids.ContactID
	Field        string
	Value        string
	Evidence     string
	Body         *string
	NameEvidence string
	HeaderName   string
	Email        string
}

// repairedSignatureFields are the fields the signature pass wrote off a mail,
// named by a ref the writer could have written.
const repairedSignatureFields = `f.source = $1 AND (` + sourceRefActivityID + `) IS NOT NULL`

// SignatureFieldSourcesTx pages, by id, the fields the signature pass wrote off
// a mail, after the given id.
func SignatureFieldSourcesTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]SignatureFieldSource, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT f.id, f.contact_id, f.field, f.value, f.evidence_snippet, a.body,
		       coalesce((SELECT n.evidence_snippet FROM contact_profile_field n
		                  WHERE n.contact_id = f.contact_id AND n.source_ref = f.source_ref
		                    AND n.source = $1 AND n.field = $4 LIMIT 1), ''),
		       coalesce((SELECT au.before->>'full_name' FROM audit_log au
		                  WHERE au.entity_type = 'contact' AND au.entity_id = f.contact_id
		                    AND au.actor_id = $5 AND au.before ? 'full_name'
		                  ORDER BY au.occurred_at, au.id LIMIT 1), c.full_name),
		       coalesce((SELECT e.email FROM contact_email e
		                  WHERE e.contact_id = f.contact_id AND e.is_primary AND e.archived_at IS NULL LIMIT 1), '')
		  FROM contact_profile_field f
		  JOIN contact c ON c.id = f.contact_id
		  LEFT JOIN activity a ON a.id = `+sourceRefActivityID+`
		       AND a.archived_at IS NULL AND a.restricted_at IS NULL
		 WHERE `+repairedSignatureFields+` AND f.id > $2
		 ORDER BY f.id
		 LIMIT $3`, enrichSource, after, limit, columnFirstName, enrichCapturedBy)
	if err != nil {
		return nil, fmt.Errorf("contacts: paging the signature fields: %w", err)
	}
	page, err := pgx.CollectRows(rows, pgx.RowToStructByPos[SignatureFieldSource])
	if err != nil {
		return nil, fmt.Errorf("contacts: paging the signature fields: %w", err)
	}
	return page, nil
}

// CountSignatureFieldSources counts the fields SignatureFieldSourcesTx pages,
// up to one past limit.
func CountSignatureFieldSources(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, limit int64,
) (int64, error) {
	if err := auth.Require(ctx, entityContact, principal.ActionRead); err != nil {
		return 0, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (
		SELECT 1 FROM contact_profile_field f WHERE `+repairedSignatureFields+` LIMIT $2) fields`,
		enrichSource, limit+1).Scan(&count)
	return count, err
}
