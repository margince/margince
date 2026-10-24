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
// archived or restricted, so nobody can judge the field against it any more.
type SignatureFieldSource struct {
	ID        ids.UUID
	ContactID ids.ContactID
	Field     string
	Value     string
	Evidence  string
	Body      *string
}

// repairedSignatureFields leaves out the name parts. A name moves the
// contact's display name with it (followSignatureDisplay), which no
// retraction can put back, and the name gate already demands the contact's
// own name in the block.
const repairedSignatureFields = `f.source = $1 AND f.field NOT IN ('first_name', 'last_name')
		   AND f.source_ref LIKE 'activity:%'`

// SignatureFieldSourcesTx pages, by id, the fields the signature pass wrote off
// a mail, after the given id.
func SignatureFieldSourcesTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]SignatureFieldSource, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT f.id, f.contact_id, f.field, f.value, f.evidence_snippet, a.body
		  FROM contact_profile_field f
		  LEFT JOIN activity a ON a.id = `+sourceRefActivityID+`
		       AND a.archived_at IS NULL AND a.restricted_at IS NULL
		 WHERE `+repairedSignatureFields+` AND f.id > $2
		 ORDER BY f.id
		 LIMIT $3`, enrichSource, after, limit)
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
