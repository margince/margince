// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// The language table holds every stored language, and a locale keeps its own shape.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

func sqlState(err error) string {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pgErr.Code
	}
	return ""
}

func TestAConsentNoticeCannotBeStoredUnderATypoOfItsLanguage(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()

	for locale, wantState := range map[string]string{"de": "", "dee": "23503", "de-DE": "23503", "": "23503"} {
		_, err := conn.Exec(ctx,
			`INSERT INTO consent_text_version (key, version, body, locale) VALUES ('k-'||$1::text, 'v1', 'Body', $1)`, locale)
		if got := sqlState(err); got != wantState {
			t.Errorf("locale %q: SQLSTATE %q (%v), want %q", locale, got, err, wantState)
		}
	}
}

func TestAnOfferTemplateKeepsItsRegionButNotAFreeWord(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()

	for locale, wantState := range map[string]string{"de-DE": "", "de-AT": "", "de": "", "deutsch": "23514", "de-at": "23514", "DE": "23514"} {
		_, err := conn.Exec(ctx, `INSERT INTO offer_template (name, locale, layout) VALUES ('t-'||$1::text, $1, '{}')`, locale)
		if got := sqlState(err); got != wantState {
			t.Errorf("locale %q: SQLSTATE %q (%v), want %q", locale, got, err, wantState)
		}
	}
}

// An installation that stored a language the table does not hold upgrades
// without losing the row. The key over such a column stays unvalidated.
// Only the app_user key is validated, because that table is small.
func TestTheLanguageKeysApplyOverAStoredUnknownCode(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()

	core, err := migrations.CoreHistory()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}
	through := namespaceThroughVersion(t, core, "1795713761")
	if _, err := dbmigrate.Up(ctx, conn, through); err != nil {
		t.Fatalf("up: %v", err)
	}
	if _, err := dbmigrate.Down(ctx, conn, through, 2); err != nil {
		t.Fatalf("rolling the language keys back: %v", err)
	}
	if _, err := conn.Exec(ctx,
		`INSERT INTO consent_text_version (key, version, body, locale) VALUES ('old-notice', 'v1', 'Body', 'fr')`); err != nil {
		t.Fatalf("seeding a notice in a language the table will not hold: %v", err)
	}

	if _, err := dbmigrate.Up(ctx, conn, through); err != nil {
		t.Fatalf("the upgrade stopped on a stored unknown language: %v", err)
	}
	var kept int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM consent_text_version WHERE locale = 'fr'`).Scan(&kept); err != nil || kept != 1 {
		t.Fatalf("the notice in fr: %d rows, %v; the upgrade must keep it", kept, err)
	}
	for constraint, want := range map[string]bool{
		"consent_text_version_locale_fkey": false, "activity_language_fkey": false, "app_user_locale_fkey": true,
	} {
		var validated bool
		if err := conn.QueryRow(ctx, `SELECT convalidated FROM pg_constraint WHERE conname = $1`, constraint).Scan(&validated); err != nil {
			t.Fatalf("reading %s: %v", constraint, err)
		}
		if validated != want {
			t.Errorf("%s validated = %v, want %v", constraint, validated, want)
		}
	}
}
