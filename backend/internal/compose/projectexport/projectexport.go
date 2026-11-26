// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package projectexport writes the company an exported project row names.
package projectexport

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// WriteCustomers sets an exported project row's company_id to the company the
// project read names: its first live customer edge this exporter may read, or
// nothing. The stored project.company_id does not follow the edges, and naming
// it skips relationship.read. Rows of any other table are left alone.
func WriteCustomers(ctx context.Context, tx pgx.Tx, table string, columns []string, rows [][]any) error {
	at, idAt := slices.Index(columns, "company_id"), slices.Index(columns, "id")
	if table != "project" || at < 0 {
		return nil
	}
	projectIDs := make([]ids.ProjectID, 0, len(rows))
	for _, row := range rows {
		if id, named := rowID(row, idAt); named {
			projectIDs = append(projectIDs, id)
		}
	}
	customers, _, err := contacts.FirstCustomersOnProjectsTx(ctx, tx, projectIDs)
	if err != nil {
		return fmt.Errorf("projectexport: reading the exported projects' customers: %w", err)
	}
	for _, row := range rows {
		row[at] = nil
		if id, named := rowID(row, idAt); named && customers[id] != nil {
			row[at] = [16]byte(customers[id].UUID)
		}
	}
	return nil
}

// rowID reads a row's own id, as pgx hands a uuid column back.
func rowID(row []any, at int) (ids.ProjectID, bool) {
	if at < 0 || at >= len(row) {
		return ids.ProjectID{}, false
	}
	switch v := row[at].(type) {
	case [16]byte:
		return ids.From[ids.ProjectKind](ids.UUID(v)), true
	case ids.UUID:
		return ids.From[ids.ProjectKind](v), true
	default:
		return ids.ProjectID{}, false
	}
}
