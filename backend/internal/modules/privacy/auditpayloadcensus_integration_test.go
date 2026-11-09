// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAuditPayloadRedactionCoversEveryCatalogColumn(t *testing.T) {
	for _, sample := range []json.RawMessage{
		json.RawMessage(`"Census Contact census-contact@example.test +49305550101"`),
		json.RawMessage(`{"evidence":[{"name":"Census Contact","address":"census-contact@example.test","phone":"+49305550101"}]}`),
	} {
		t.Run(string(sample), func(t *testing.T) {
			ctx := principal.SystemActing(context.Background(), "privacy_test")
			tx := subjectColumnsTx(ctx, t)
			expectedRows, err := tx.Query(ctx, `SELECT c.relname,count(*) FROM pg_class c
 JOIN pg_namespace n ON n.oid=c.relnamespace JOIN pg_attribute a ON a.attrelid=c.oid
 WHERE n.nspname='public' AND c.relkind IN ('r','p') AND a.attnum>0 AND NOT a.attisdropped
 GROUP BY c.relname`)
			if err != nil {
				t.Fatal(err)
			}
			expected, err := pgx.CollectRows(expectedRows, pgx.RowToStructByPos[struct {
				Name    string
				Columns int
			}])
			if err != nil {
				t.Fatal(err)
			}
			rows, err := tx.Query(ctx, `WITH images AS (
 SELECT table_name,count(*) AS columns,jsonb_object_agg(column_name,@sample::jsonb) AS payload
 FROM information_schema.columns WHERE table_schema='public' GROUP BY table_name)
 SELECT table_name,columns,audit_redacted_image(table_name,payload),
 audit_evidence_after_erasure('update',payload,payload,jsonb_build_object('evidence',payload))
 FROM images ORDER BY table_name`, pgx.StrictNamedArgs{"sample": sample})
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			seen := map[string]int{}
			columns := 0
			for rows.Next() {
				var kind string
				var count int
				var image, evidence []byte
				if err := rows.Scan(&kind, &count, &image, &evidence); err != nil {
					t.Fatal(err)
				}
				seen[kind] = count
				columns += count
				for _, marker := range [][]byte{[]byte("Census Contact"), []byte("census-contact@example.test"), []byte("+49305550101")} {
					if bytes.Contains(image, marker) || bytes.Contains(evidence, marker) {
						t.Errorf("%s retains personal values in its redacted image or evidence", kind)
					}
				}
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			for _, table := range expected {
				if seen[table.Name] != table.Columns {
					t.Errorf("the census read %d of %d columns for %s", seen[table.Name], table.Columns, table.Name)
				}
			}
			for _, kind := range []string{"contact", "lead", "dedupe_candidate", "activity", "audit_log", "event_outbox"} {
				if seen[kind] == 0 {
					t.Errorf("the catalog census missed %s", kind)
				}
			}
			t.Logf("checked %d columns across %d catalog records", columns, len(seen))
		})
	}
}
