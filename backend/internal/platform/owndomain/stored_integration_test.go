// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package owndomain

// CoversSQL is the SQL copy of CoversDomain. Run against a real server over the
// stored forms, it must answer as the Go rule does on every input.

import (
	"context"
	"os"
	"testing"

	"github.com/margince/margince/backend/internal/platform/testdb"
)

func TestCoversSQLAnswersAsCoversDomain(t *testing.T) {
	dsn := os.Getenv("MARGINCE_TEST_DSN")
	if dsn == "" {
		t.Fatal("MARGINCE_TEST_DSN is not set — run `make db-up` and try again (integration tests fail loudly, they never skip)")
	}
	pool, err := testdb.OwnPool(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	own := New([]string{"acme.com", "Bücher.Example"})
	query := `SELECT EXISTS (SELECT 1 FROM unnest($2::text[]) AS own(domain) WHERE ` +
		CoversSQL("$1::text", "own.domain") + `)`
	cases := []struct {
		name    string
		address *string // nil is a row with no address, stored as NULL
		covered bool
	}{
		{"exact", ptr("rep@acme.com"), true},
		{"subdomain", ptr("rep@mail.acme.com"), true},
		{"lookalike ending", ptr("rep@notacme.com"), false},
		{"lookalike parent", ptr("rep@acme.com.evil.example"), false},
		{"trailing dot", ptr("rep@acme.com."), true},
		{"mixed-case punycode", ptr("anna@XN--Bcher-KVA.example"), true},
		{"unicode", ptr("anna@BÜCHER.example"), true},
		{"plus tag", ptr("rep+news@acme.com"), true},
		{"IP literal", ptr("rep@192.0.2.1"), false},
		{"empty", ptr(""), false},
		{"NULL", nil, false},
	}
	for _, c := range cases {
		var stored *string
		goCovers := false
		if c.address != nil {
			stored = StoredOfAddress(*c.address)
			goCovers = own.CoversDomain(OfAddress(*c.address))
		}
		var sqlCovers bool
		if err := pool.QueryRow(context.Background(), query, stored, own.Domains()).Scan(&sqlCovers); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if goCovers != c.covered {
			t.Errorf("%s: Set.CoversDomain = %v, the case expects %v", c.name, goCovers, c.covered)
		}
		if sqlCovers != goCovers {
			t.Errorf("%s: CoversSQL = %v over %v, Set.CoversDomain = %v", c.name, sqlCovers, deref(stored), goCovers)
		}
	}
}

func ptr(s string) *string { return &s }
