// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

// A vendor verdict the site read settled with nobody asked becomes an open
// question. It enrols no reader: only current mail names who decides it. A
// human answer, an admission and a suppression stay as they were, and the
// down migration settles the question back.
func TestAMachineVendorVerdictBecomesAnOpenQuestionForItsOwner(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()

	core, err := migrations.Core()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}
	if _, err := dbmigrate.Up(ctx, conn, core); err != nil {
		t.Fatalf("up: %v", err)
	}
	if _, err := dbmigrate.Down(ctx, conn, namespaceThroughVersion(t, core, "1792779055"), 2); err != nil {
		t.Fatalf("rolling the vendor migrations back: %v", err)
	}
	var owner string
	if err := conn.QueryRow(ctx, `INSERT INTO app_user (email, display_name) VALUES ('rep@vendor.test', 'Rep') RETURNING id`).Scan(&owner); err != nil {
		t.Fatalf("seeding the owner: %v", err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO company_domain_disposition (domain, status, source, owner_id, evidence, last_evidence_at)
		VALUES ('machine-vendor.test', 'provider', 'site_read', $1, 'Classified as a mail or hosting vendor.', now()),
		       ('ownerless-vendor.test', 'provider', 'site_read', NULL, 'Classified as a mail or hosting vendor.', NULL),
		       ('human-vendor.test', 'provider', 'human', $1, NULL, NULL),
		       ('personal.test', 'personal', 'site_read', $1, NULL, NULL)`, owner); err != nil {
		t.Fatalf("seeding settled verdicts: %v", err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO company_domain_disposition (domain, status, source, owner_id, admission, admission_reason, admission_source, admission_at)
		VALUES ('admitted-vendor.test', 'provider', 'site_read', $1, 'admitted', 'a colleague let it in', 'human', now()),
		       ('suppressed-vendor.test', 'provider', 'site_read', $1, 'suppressed', 'bulk mail', 'verdict', now())`, owner); err != nil {
		t.Fatalf("seeding decided verdicts: %v", err)
	}

	if _, err := dbmigrate.Up(ctx, conn, core); err != nil {
		t.Fatalf("migrating back up: %v", err)
	}
	want := map[string]struct{ status, reason string }{
		"machine-vendor.test":    {"pending", "vendor_verdict"},
		"ownerless-vendor.test":  {"pending", "vendor_verdict"},
		"human-vendor.test":      {"provider", ""},
		"personal.test":          {"personal", ""},
		"admitted-vendor.test":   {"provider", ""},
		"suppressed-vendor.test": {"provider", ""},
	}
	for domain, expected := range want {
		var status, reason, evidence string
		var sourceNull, cursorNull bool
		if err := conn.QueryRow(ctx, `SELECT status, COALESCE(pending_reason, ''), source IS NULL, next_attempt_at IS NULL, COALESCE(evidence, '')
			FROM company_domain_disposition WHERE domain = $1`, domain).Scan(&status, &reason, &sourceNull, &cursorNull, &evidence); err != nil {
			t.Fatalf("reading %s: %v", domain, err)
		}
		if status != expected.status || reason != expected.reason {
			t.Errorf("%s reads %s/%q, want %s/%q", domain, status, reason, expected.status, expected.reason)
		}
		if expected.reason == "vendor_verdict" && (!sourceNull || !cursorNull || evidence == "") {
			t.Errorf("%s: source null=%v cursor null=%v evidence=%q; an open question has no source, no retry and keeps its evidence", domain, sourceNull, cursorNull, evidence)
		}
	}
	readers := map[string]int{}
	rows, err := conn.Query(ctx, `SELECT domain, count(*) FROM company_domain_question_reader GROUP BY domain`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var domain string
		var count int
		if err := rows.Scan(&domain, &count); err != nil {
			t.Fatal(err)
		}
		readers[domain] = count
	}
	rows.Close()
	if len(readers) != 0 {
		t.Errorf("reader rows = %v, want none: an old owner is no evidence of who decides now", readers)
	}

	if _, err := dbmigrate.Down(ctx, conn, namespaceThroughVersion(t, core, "1792779055"), 2); err != nil {
		t.Fatalf("rolling back a second time: %v", err)
	}
	var status, source string
	if err := conn.QueryRow(ctx, `SELECT status, source FROM company_domain_disposition WHERE domain = 'machine-vendor.test'`).Scan(&status, &source); err != nil {
		t.Fatal(err)
	}
	if status != "provider" || source != "site_read" {
		t.Errorf("after down the vendor question reads %s/%s, want provider/site_read", status, source)
	}
	if _, err := dbmigrate.Up(ctx, conn, core); err != nil {
		t.Fatalf("migrating up after the round trip: %v", err)
	}
}
