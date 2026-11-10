// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func pinLeadAuditClock(t *testing.T) time.Time {
	t.Helper()
	at := time.Date(2026, 12, 1, 12, 0, 0, 123456789, time.UTC)
	previous := leadSLAClock
	leadSLAClock = func() time.Time { return at }
	t.Cleanup(func() { leadSLAClock = previous })
	return at
}

func requireStoredLeadClockAudit(ctx context.Context, t *testing.T, s *Store, id ids.LeadID) {
	t.Helper()
	var checked int
	var submicro bool
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT DISTINCT ON (k.key) k.key, k.value #>> '{}', to_jsonb(l)->>k.key
   FROM audit_log a JOIN lead l ON l.id = a.entity_id
   CROSS JOIN LATERAL jsonb_each(a.after) k
   JOIN pg_attribute c ON c.attrelid = 'lead'::regclass AND c.attname = k.key AND c.atttypid = 'timestamptz'::regtype
   WHERE a.entity_type = 'lead' AND l.id = @lead AND k.value <> 'null'::jsonb
   ORDER BY k.key, a.occurred_at DESC, a.id DESC`, pgx.StrictNamedArgs{"lead": id})
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var column, stated, live string
			if err := rows.Scan(&column, &stated, &live); err != nil {
				return err
			}
			before, err := time.Parse(time.RFC3339Nano, stated)
			if err != nil {
				return err
			}
			after, err := time.Parse(time.RFC3339Nano, live)
			if err != nil {
				return err
			}
			if !storekit.SameStoredInstant(before, after) {
				t.Errorf("%s: audit %s differs from stored %s", column, stated, live)
			}
			submicro = submicro || before.Nanosecond()%1000 != 0
			checked++
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked == 0 || !submicro {
		t.Fatalf("clock census checked %d timestamps, submicrosecond stamp=%v", checked, submicro)
	}
}
