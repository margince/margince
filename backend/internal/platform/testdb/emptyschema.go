// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package testdb

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// dataTables selects every relation a test can write rows to: ordinary tables
// in public and ext. It leaves out the migration ledgers and the reference
// tables the migrations seed. The emptiness proof of a fresh copy reads these.
const dataTables = `
	FROM pg_class c
	JOIN pg_namespace n ON n.oid = c.relnamespace
	WHERE n.nspname IN ('public', 'ext')
	  AND c.relkind = 'r'
	  AND c.relname NOT LIKE 'schema_migrations_%'
	  AND c.relname <> 'river_migration'
	  AND c.relname NOT IN ` + PreservedReferenceTables

// PreservedReferenceTables lists, as SQL, the tables the migrations seed. A
// fresh copy of the template holds rows in them, so the emptiness proof leaves
// them out. gates/preservedtableparity_test.go reads this list.
const PreservedReferenceTables = `('activity_kind', 'channel_provider', ` +
	`'lead_origin', 'lead_disqualify_reason', 'sdr_handoff_reason', 'field_mask', ` +
	`'currency_minor_digits', 'deal_acquisition_source', 'deal_loss_reason', 'record_role', 'partner_stage', ` +
	`'maskable_field', 'report_projection_fence', 'home_read_fence', ` +
	`'activity_review_template', 'activity_own_sent_mail_repair_cutoff', 'legal_entity')`

// queryIdents runs a query whose single column is an already-quoted identifier
// and collects the results.
func queryIdents(ctx context.Context, q *pgx.Conn, sql string) ([]string, error) {
	rows, err := q.Query(ctx, sql)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}
