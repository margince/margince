// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestSourceAssessmentDowngradeRefusesToDiscardEvidence(t *testing.T) {
	for _, state := range []string{"source_review", "assessed_indirect", "assessed_direct"} {
		t.Run(state, func(t *testing.T) {
			dsn, _ := dsns(t)
			conn := connect(t, dsn)
			headSchema(t, conn)
			ctx := context.Background()
			id := archivedWithDuty(ctx, t, conn, "Source Evidence Example", "human:fixture")
			var update string
			switch state {
			case "source_review":
				update = `UPDATE privacy_notice_case SET state = 'source_review', rule = 'unassessed', due_at = NULL, allowed_routes = '{}' WHERE id = @id`
			case "assessed_indirect":
				update = `UPDATE privacy_notice_case SET source_kind = 'referral', source_review_note = 'Verified referral' WHERE id = @id`
			case "assessed_direct":
				update = `UPDATE privacy_notice_case SET state = 'exempt_with_reason', rule = 'none', due_at = NULL, allowed_routes = '{}', completed_at = now(), resolution_note = 'Direct collection', source_kind = 'subject_initiated', source_review_note = 'Original incoming message' WHERE id = @id`
			}
			if _, err := conn.Exec(ctx, update, pgx.NamedArgs{"id": id}); err != nil {
				t.Fatal(err)
			}
			sql, err := os.ReadFile("core/1793514621_notice_source_assessment.down.sql")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := conn.Exec(ctx, string(sql)); err == nil || !strings.Contains(err.Error(), "retain evidence") {
				t.Fatalf("downgrade=%v", err)
			}
			var kept bool
			if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM privacy_notice_case WHERE id = @id AND (source_kind IS NOT NULL OR rule = 'unassessed'))`, pgx.NamedArgs{"id": id}).Scan(&kept); err != nil || !kept {
				t.Fatalf("evidence kept=%t err=%v", kept, err)
			}
		})
	}
}
