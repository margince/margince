// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"
)

const (
	noticeCheck       = "notice_target_type_check"
	contributionCheck = "report_edition_contribution_source_type_check"
	noticeValidate    = "core/1795358785_the_notice_and_report_type_checks_are_validated.up.sql"

	// No app_user or report_edition row exists, so a row the CHECK lets by
	// meets its foreign key, which is the proof it got that far.
	insertNotice = `INSERT INTO notice (recipient_user_id, kind, subject, captured_by, target_type, target_id)
		VALUES (uuidv7(), 'automation', 's', 'system:x', $1, uuidv7())`
	insertContribution = `INSERT INTO report_edition_contribution (edition_id, metric, context_id, contribution_key, group_key, source_type, source_id, fact)
		VALUES (uuidv7(), 'm', 'c', 'k', 'g', $1, uuidv7(), '{}')`
)

func TestANoticeNamesATargetTypeAnApprovalOrTheNoticeItselfCanName(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	headSchema(t, conn)
	ctx := context.Background()
	for _, c := range []struct{ target, want string }{
		{"deal", "notice_recipient_user_id_fkey"},
		{"approval", "notice_recipient_user_id_fkey"},
		{"approval_history", "notice_recipient_user_id_fkey"},
		{"ext_visit_log_visit", "notice_recipient_user_id_fkey"},
		{"widget", noticeCheck},
		{"Deal", noticeCheck},
		{"ext:visit-log:visit", noticeCheck},
	} {
		if got := violated(ctx, t, conn, insertNotice, c.target); got != c.want {
			t.Errorf("notice target %q: constraint %q, want %q", c.target, got, c.want)
		}
	}
}

func TestAContributionNamesTheSourceOfAMetric(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	headSchema(t, conn)
	ctx := context.Background()
	for _, c := range []struct{ source, want string }{
		{"deal", "report_edition_contribution_edition_id_fkey"},
		{"forecast_projection", "report_edition_contribution_edition_id_fkey"},
		{"sdr_handoff", "report_edition_contribution_edition_id_fkey"},
		{"widget", contributionCheck},
		{"Deal", contributionCheck},
	} {
		if got := violated(ctx, t, conn, insertContribution, c.source); got != c.want {
			t.Errorf("source %q: constraint %q, want %q", c.source, got, c.want)
		}
	}
}

func reopenChecks(ctx context.Context, t *testing.T, conn *pgx.Conn) {
	t.Helper()
	for _, c := range []struct{ table, name string }{{"notice", noticeCheck}, {"report_edition_contribution", contributionCheck}} {
		var def string
		if err := conn.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname = $1`, c.name).Scan(&def); err != nil {
			t.Fatalf("reading %s: %v", c.name, err)
		}
		for _, sql := range []string{
			"ALTER TABLE " + c.table + " DROP CONSTRAINT " + c.name,
			"ALTER TABLE " + c.table + " ADD CONSTRAINT " + c.name + " " + def + " NOT VALID",
		} {
			if _, err := conn.Exec(ctx, sql); err != nil {
				t.Fatalf("%s: %v", sql, err)
			}
		}
	}
}

func runNoticeValidation(ctx context.Context, t *testing.T, conn *pgx.Conn) {
	t.Helper()
	sql, err := os.ReadFile(filepath.Clean(noticeValidate))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("running the validating migration: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestTheNoticeAndContributionChecksValidateWhenNoRowBreaksThem(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	headSchema(t, conn)
	ctx := context.Background()
	reopenChecks(ctx, t, conn)
	runNoticeValidation(ctx, t, conn)
	for _, name := range []string{noticeCheck, contributionCheck} {
		if !validated(ctx, t, conn, name) {
			t.Errorf("%s stayed NOT VALID on a clean table", name)
		}
	}
}

func TestAnOldNoticeWithAnUnknownTargetKeepsItsCheckNotValid(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	headSchema(t, conn)
	ctx := context.Background()
	var def string
	if err := conn.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname = $1`, noticeCheck).Scan(&def); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `ALTER TABLE notice DROP CONSTRAINT `+noticeCheck); err != nil {
		t.Fatal(err)
	}
	var user string
	if err := conn.QueryRow(ctx, `INSERT INTO app_user (email, display_name) VALUES ('legacy@example.test', 'Legacy') RETURNING id::text`).Scan(&user); err != nil {
		t.Fatalf("planting a recipient: %v", err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO notice (recipient_user_id, kind, subject, captured_by, target_type, target_id)
		VALUES ($1::uuid, 'automation', 's', 'system:x', 'legacy_thing', uuidv7())`, user); err != nil {
		t.Fatalf("planting a legacy notice: %v", err)
	}
	if _, err := conn.Exec(ctx, `ALTER TABLE notice ADD CONSTRAINT `+noticeCheck+` `+def+` NOT VALID`); err != nil {
		t.Fatal(err)
	}
	runNoticeValidation(ctx, t, conn)
	if validated(ctx, t, conn, noticeCheck) {
		t.Errorf("%s was validated over a row that breaks it", noticeCheck)
	}
	if !validated(ctx, t, conn, contributionCheck) {
		t.Errorf("%s was left NOT VALID though no row breaks it", contributionCheck)
	}
	if got := violated(ctx, t, conn, insertNotice, "widget"); got != noticeCheck {
		t.Errorf("a new bad write met constraint %q, want %q", got, noticeCheck)
	}
}
