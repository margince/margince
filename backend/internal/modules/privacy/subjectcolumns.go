// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The GDPR engines' view of the custom-field catalog. A workspace's
// cf_ columns hold subject data exactly like core columns, so Art. 17
// erasure and the Art. 15 export must cover them — and with ANY catalog
// status, not the record surface's active-only slice: retiring a field
// preserves the physical column and every value in it (the lifecycle
// never DROPs), so a retired column still holds PII the workspace is
// accountable for. The catalog read reaches every custom_field in the
// installation — core 0229 dropped the table's workspace column, so there
// is nothing narrower to read, and ADR-0061's one company per
// installation is what makes that the right set. Every column name is
// catalog-derived (server-minted at field creation), never client text,
// and is still identifier-quoted
// before splicing — the same posture as storekit's customcolumns
// mechanics.

import (
	"context"
	"fmt"
	"maps"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// subjectCustomColumns returns every custom-field column defined on one
// core object in the workspace bound to tx, retired included (see the
// package note above), ordered by column name for deterministic SQL.
func subjectCustomColumns(ctx context.Context, tx pgx.Tx, object string) ([]fieldcatalog.Column, error) {
	rows, err := tx.Query(ctx,
		`SELECT column_name, type FROM custom_field WHERE object = $1 ORDER BY column_name`, object)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	// Scanned into NAMED fields rather than by position. fieldcatalog.Column
	// belongs to the port, not to this module, so a positional mapping bound this
	// two-column SELECT to a struct shape privacy has no say over: the day the
	// port gained a third field for somebody else's read, this query started
	// failing on a mismatch it never mentioned. Only the columns erasure and the
	// Art. 15 export actually use are read.
	var cols []fieldcatalog.Column
	for rows.Next() {
		var c fieldcatalog.Column
		if err := rows.Scan(&c.Name, &c.Type); err != nil {
			return nil, err
		}
		cols = append(cols, c)
	}
	return cols, rows.Err()
}

// nullColumnAssignments renders the `, "cf_x" = NULL, …` fragment an
// anonymizing UPDATE splices after its fixed SET list — empty when the
// object has no custom columns, so the caller splices unconditionally.
func nullColumnAssignments(cols []fieldcatalog.Column) string {
	var b strings.Builder
	for _, c := range cols {
		b.WriteString(", ")
		b.WriteString(pgx.Identifier{c.Name}.Sanitize())
		b.WriteString(" = NULL")
	}
	return b.String()
}

// clearActivityCustomValues empties the custom-field values of the given
// activities. They are content, so every arm that destroys an activity's text
// calls this in the same transaction.
func clearActivityCustomValues(ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID) error {
	if len(activityIDs) == 0 {
		return nil
	}
	cols, err := subjectCustomColumns(ctx, tx, string(fieldcatalog.TargetActivity))
	if err != nil || len(cols) == 0 {
		return err
	}
	held := make([]string, len(cols))
	for i, c := range cols {
		held[i] = pgx.Identifier{c.Name}.Sanitize() + " IS NOT NULL"
	}
	// Every caller has archived the row already, so that assignment changes
	// nothing; it gives the erasure census a column to read the statement by.
	_, err = tx.Exec(ctx, fmt.Sprintf(`UPDATE activity SET archived_at = coalesce(archived_at, now())%s
		WHERE id = ANY($1) AND (%s)`, nullColumnAssignments(cols), strings.Join(held, " OR ")), activityIDs)
	return err
}

// appendActivityCustomValues merges each disclosed activity's custom-field
// values into the export's activity rows, keyed by column name. The section's
// own content_disclosed decides which rows those are, so a withheld one gets none.
func appendActivityCustomValues(ctx context.Context, tx pgx.Tx, activities []map[string]any) error {
	disclosed := map[string][]map[string]any{}
	var activityIDs []string
	for _, row := range activities {
		id, isID := row["id"].(string)
		if shown, isBool := row["content_disclosed"].(bool); isID && isBool && shown {
			if _, seen := disclosed[id]; !seen {
				activityIDs = append(activityIDs, id)
			}
			disclosed[id] = append(disclosed[id], row)
		}
	}
	cols, err := subjectCustomColumns(ctx, tx, string(fieldcatalog.TargetActivity))
	if err != nil || len(cols) == 0 || len(activityIDs) == 0 {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT id`+storekit.SelectSuffix(cols)+
		` FROM activity WHERE id = ANY($1::uuid[])`, activityIDs)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		dests := storekit.ScanDests(cols)
		if err := rows.Scan(append([]any{&id}, dests...)...); err != nil {
			return err
		}
		for _, row := range disclosed[id.String()] {
			maps.Copy(row, storekit.ExtractValues(cols, dests))
		}
	}
	return rows.Err()
}
