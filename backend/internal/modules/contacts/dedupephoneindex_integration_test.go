// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// The phone lane reads contact_phone by number, so the plan must reach an index
// on the number instead of the whole table. Sequential scans are switched off
// because a small test table would choose one whatever indexes exist.
//
// Mutation: drop idx_contact_phone_live_phone from the migrations.
func TestThePhoneLookupReadsItsIndex(t *testing.T) {
	e := setupDedupe(t)
	var plan []string
	err := e.store.tx(e.as(), func(tx pgx.Tx) error {
		if _, err := tx.Exec(e.as(), "SET LOCAL enable_seqscan = off"); err != nil {
			return err
		}
		rows, err := tx.Query(e.as(), "EXPLAIN "+contactByPhoneSQL, []string{"+4930123456"}, nil)
		if err != nil {
			return err
		}
		plan, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	})
	if err != nil {
		t.Fatalf("explaining the phone lookup: %v", err)
	}
	if text := strings.Join(plan, "\n"); !strings.Contains(text, "idx_contact_phone_live_phone") {
		t.Fatalf("the phone lookup does not use idx_contact_phone_live_phone:\n%s", text)
	}
}
