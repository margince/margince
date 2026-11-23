// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

const (
	summaryReader = "00000000-0000-7000-8000-0000000000a1"
	summaryMoved  = "1795401755"
)

// summariesByKind reads the shared table as one line per row, in kind order.
const summariesByKind = `
	SELECT string_agg(kind || ':' || fingerprint || ':' || generated_by || ':' || (payload->>'n')
	                  || ':' || extract(epoch FROM generated_at)::bigint, ',' ORDER BY kind)
	  FROM generated_summary`

// The upgrade carries each cached summary into the shared table under its
// kind, and a rollback returns each to the table it came from.
func TestTheGeneratedSummariesMoveAndComeBack(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()

	core, err := migrations.Core()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}
	if _, err := dbmigrate.Up(ctx, conn, namespaceThroughVersion(t, core, "1795363336")); err != nil {
		t.Fatalf("up to the migration before: %v", err)
	}
	company, contact, deal := seedSummaryRecords(ctx, t, conn)
	at := `'2026-05-04T09:30:00Z'`
	for _, seed := range []string{
		`INSERT INTO company_brief (user_id, company_id, fingerprint, generated_at, generated_by, payload)
		 VALUES ($1, $2, 'fp-brief', ` + at + `, 'model', '{"n": 1}')`,
		`INSERT INTO company_dossier (user_id, company_id, fingerprint, generated_at, generated_by, payload)
		 VALUES ($1, $2, 'fp-dossier', ` + at + `, 'deterministic', '{"n": 2}')`,
		`INSERT INTO company_growth_fit (user_id, company_id, fingerprint, generated_at, generated_by, payload)
		 VALUES ($1, $2, 'fp-fit', ` + at + `, 'model', '{"n": 3}')`,
	} {
		if _, err := conn.Exec(ctx, seed, summaryReader, company); err != nil {
			t.Fatalf("seeding %s: %v", seed, err)
		}
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO contact_brief (user_id, contact_id, fingerprint, generated_at, generated_by, payload)
		VALUES ($1, $2, 'fp-contact', `+at+`, 'model', '{"n": 4}')`, summaryReader, contact); err != nil {
		t.Fatalf("seeding the contact brief: %v", err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO deal_status_card (user_id, deal_id, fingerprint, generated_at, generated_by, payload)
		VALUES ($1, $2, 'fp-card', `+at+`, 'deterministic', '{"n": 5}')`, summaryReader, deal); err != nil {
		t.Fatalf("seeding the status card: %v", err)
	}

	moved := namespaceThroughVersion(t, core, summaryMoved)
	if _, err := dbmigrate.Up(ctx, conn, moved); err != nil {
		t.Fatalf("migrating up: %v", err)
	}
	const carried = "company_brief:fp-brief:model:1:1777887000,company_dossier:fp-dossier:deterministic:2:1777887000," +
		"company_growth_fit:fp-fit:model:3:1777887000,contact_brief:fp-contact:model:4:1777887000," +
		"deal_status_card:fp-card:deterministic:5:1777887000"
	if got := scalar(t, conn, summariesByKind); got != carried {
		t.Errorf("the summaries moved as %s, want %s", got, carried)
	}
	if got := scalar(t, conn, `
		SELECT count(*)::text FROM generated_summary
		 WHERE (company_id = $1 AND kind LIKE 'company_%') OR (contact_id = $2 AND kind = 'contact_brief')
		    OR (deal_id = $3 AND kind = 'deal_status_card')`, company, contact, deal); got != "5" {
		t.Errorf("%s of the 5 summaries name the record they were written about", got)
	}

	if _, err := dbmigrate.Down(ctx, conn, moved, 1); err != nil {
		t.Fatalf("rolling back: %v", err)
	}
	for _, c := range []struct{ table, key, id, want string }{
		{"company_brief", "company_id", company, "fp-brief:model:1"},
		{"company_dossier", "company_id", company, "fp-dossier:deterministic:2"},
		{"company_growth_fit", "company_id", company, "fp-fit:model:3"},
		{"contact_brief", "contact_id", contact, "fp-contact:model:4"},
		{"deal_status_card", "deal_id", deal, "fp-card:deterministic:5"},
	} {
		got := scalar(t, conn, `SELECT fingerprint || ':' || generated_by || ':' || (payload->>'n') FROM `+
			c.table+` WHERE user_id = $1 AND `+c.key+` = $2`, summaryReader, c.id)
		if got != c.want {
			t.Errorf("after the rollback %s reads %s, want %s", c.table, got, c.want)
		}
	}
}

// A summary leaves with the record it is about, whichever of the three that is.
func TestADeletedRecordTakesItsGeneratedSummaries(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Errorf("rolling back: %v", err)
		}
	}()
	company, contact, deal := seedSummaryRecords(ctx, t, tx)
	for _, row := range []struct{ kind, key, id string }{
		{"company_brief", "company_id", company},
		{"company_dossier", "company_id", company},
		{"contact_brief", "contact_id", contact},
		{"deal_status_card", "deal_id", deal},
	} {
		if _, err := tx.Exec(ctx, `
			INSERT INTO generated_summary (user_id, kind, `+row.key+`, fingerprint, generated_by, payload)
			VALUES ($1, $2, $3, 'fp', 'model', '{}')`, summaryReader, row.kind, row.id); err != nil {
			t.Fatalf("storing a %s: %v", row.kind, err)
		}
	}
	for _, c := range []struct {
		delete, id string
		left       int
	}{
		{`DELETE FROM deal WHERE id = $1`, deal, 3},
		{`DELETE FROM contact WHERE id = $1`, contact, 2},
		{`DELETE FROM company WHERE id = $1`, company, 0},
	} {
		if _, err := tx.Exec(ctx, c.delete, c.id); err != nil {
			t.Fatalf("%s: %v", c.delete, err)
		}
		var left int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM generated_summary WHERE user_id = $1`,
			summaryReader).Scan(&left); err != nil {
			t.Fatal(err)
		}
		if left != c.left {
			t.Errorf("after %s the reader holds %d summaries, want %d", c.delete, left, c.left)
		}
	}
}

// A kind is stored only against the record type it summarises, once per reader.
func TestAGeneratedSummaryNamesTheRecordOfItsKind(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Errorf("rolling back: %v", err)
		}
	}()
	company, contact, deal := seedSummaryRecords(ctx, t, tx)
	store := func(kind string, companyID, contactID, dealID *string) error {
		if _, err := tx.Exec(ctx, `SAVEPOINT attempt`); err != nil {
			t.Fatal(err)
		}
		_, stored := tx.Exec(ctx, `
			INSERT INTO generated_summary (user_id, kind, company_id, contact_id, deal_id, fingerprint, generated_by, payload)
			VALUES ($1, $2, $3, $4, $5, 'fp', 'model', '{}')`, summaryReader, kind, companyID, contactID, dealID)
		if stored != nil {
			if _, err := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT attempt`); err != nil {
				t.Fatal(err)
			}
		}
		return stored
	}
	if err := store("company_brief", &company, nil, nil); err != nil {
		t.Fatalf("a company brief of a company was refused: %v", err)
	}
	if err := store("company_dossier", &company, nil, nil); err != nil {
		t.Errorf("a second kind about the same company was refused: %v", err)
	}
	for name, refused := range map[string]error{
		"a second company brief for the same reader": store("company_brief", &company, nil, nil),
		"a company brief of a contact":               store("company_brief", nil, &contact, nil),
		"a contact brief of a deal":                  store("contact_brief", nil, nil, &deal),
		"a status card of two records":               store("deal_status_card", &company, nil, &deal),
		"a summary of no record":                     store("deal_status_card", nil, nil, nil),
		"a kind outside the list":                    store("deal_brief", nil, nil, &deal),
	} {
		if refused == nil {
			t.Errorf("%s was stored", name)
		}
	}
}

// seedSummaryRecords writes a reader and one company, contact and deal.
func seedSummaryRecords(
	ctx context.Context, t *testing.T, db interface {
		QueryRow(context.Context, string, ...any) pgx.Row
	},
) (company, contact, deal string) {
	t.Helper()
	if err := db.QueryRow(ctx, `
		WITH reader AS (
		  INSERT INTO app_user (id, email, display_name) VALUES ($1, 'reader@summary.test', 'Reader')
		), co AS (
		  INSERT INTO company (display_name, source, captured_by) VALUES ('Summary Co', 'manual', 'test') RETURNING id
		), ct AS (
		  INSERT INTO contact (full_name, source, captured_by) VALUES ('Summary Contact', 'manual', 'test') RETURNING id
		), p AS (
		  INSERT INTO pipeline (name) VALUES ('Summary') RETURNING id
		), s AS (
		  INSERT INTO stage (pipeline_id, name, position, semantic, win_probability)
		  SELECT id, 'Open', 1, 'open', 10 FROM p RETURNING id, pipeline_id
		), d AS (
		  INSERT INTO deal (pipeline_id, stage_id, company_id, name, source, captured_by)
		  SELECT s.pipeline_id, s.id, co.id, 'Summary deal', 'manual', 'test' FROM s, co RETURNING id
		)
		SELECT co.id::text, ct.id::text, d.id::text FROM co, ct, d`, summaryReader).
		Scan(&company, &contact, &deal); err != nil {
		t.Fatalf("seeding the records: %v", err)
	}
	return company, contact, deal
}
