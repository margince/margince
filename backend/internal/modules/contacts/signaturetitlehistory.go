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

// SignatureHistoryPage is one page of read watermarks the legacy repair looked
// at, and how many of them it reopened.
type SignatureHistoryPage struct {
	Scanned  []ids.UUID
	Reopened int
}

// oneMailWatermark is the rows the repair inspects and the estimate counts.
const oneMailWatermark = "st.history_depth IS NULL"

// Only legacy one-mail watermarks reopen. A depth written by the current pass
// keeps a second repair from reopening work it already considered.
var reopenSignatureHistorySQL = `
	WITH page AS (
	    SELECT st.contact_id FROM contact_signature_enrich_state st
	     WHERE ` + oneMailWatermark + ` AND st.contact_id > $1
	     ORDER BY st.contact_id
	     LIMIT $2
	), reopened AS (
	    DELETE FROM contact_signature_enrich_state st
	     USING page, contact p
	     WHERE st.contact_id = page.contact_id AND p.id = st.contact_id
	       AND p.archived_at IS NULL AND p.merged_into_id IS NULL
	       AND ` + olderMailMayFillTitleSQL("p") + `
	       AND EXISTS (SELECT 1 FROM activity_link al JOIN activity a ON a.id = al.activity_id
	                    WHERE ` + signatureSourceSQL("$3") + ` AND a.id <> st.activity_id)
	    RETURNING st.contact_id
	)
	SELECT page.contact_id, EXISTS (SELECT 1 FROM reopened r WHERE r.contact_id = page.contact_id)
	  FROM page ORDER BY page.contact_id`

// ReopenSignatureTitleHistoryTx reopens one page of the watermarks the one-mail
// signature pass left on contacts without a title. It pages over the rows
// CountOneMailSignatureWatermarks counts, so the estimate is the rows it
// inspects. Only the bookkeeping row goes: no field is written.
// defaultEnabled is the workspace answer for a mailbox that never chose
// whether its mail may be read.
func ReopenSignatureTitleHistoryTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int, defaultEnabled bool) (SignatureHistoryPage, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return SignatureHistoryPage{}, err
	}
	if err := auth.Require(ctx, entityContact, principal.ActionRead); err != nil {
		return SignatureHistoryPage{}, err
	}
	rows, err := tx.Query(ctx, reopenSignatureHistorySQL, after, limit, defaultEnabled)
	if err != nil {
		return SignatureHistoryPage{}, fmt.Errorf("contacts: reopening signature watermarks: %w", err)
	}
	defer rows.Close()
	var page SignatureHistoryPage
	for rows.Next() {
		var id ids.UUID
		var reopened bool
		if err := rows.Scan(&id, &reopened); err != nil {
			return SignatureHistoryPage{}, fmt.Errorf("contacts: reading a signature watermark: %w", err)
		}
		page.Scanned = append(page.Scanned, id)
		if reopened {
			page.Reopened++
		}
	}
	if err := rows.Err(); err != nil {
		return SignatureHistoryPage{}, fmt.Errorf("contacts: reopening signature watermarks: %w", err)
	}
	return page, nil
}

// CountOneMailSignatureWatermarks counts, up to limit+1, the watermarks the
// one-mail pass set. The repair inspects each of them.
func CountOneMailSignatureWatermarks(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, limit int64,
) (int64, error) {
	if err := auth.Require(ctx, entityContact, principal.ActionRead); err != nil {
		return 0, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (
		SELECT 1 FROM contact_signature_enrich_state st WHERE `+oneMailWatermark+` LIMIT $1) watermarks`,
		limit+1).Scan(&count)
	return count, err
}
