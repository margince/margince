// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// The company type check holds a SHAPE: core's seven words, or `ext:<unit>:<type>`
// with the unit and type spelled the way an extension declares them. Which pairs
// exist is the composed set's to say, so a pair nobody declared still passes here.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestCompanyTypeCheckAcceptsCoreWordsAndExtensionShapes(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()

	var companyID string
	if err := conn.QueryRow(ctx,
		`INSERT INTO company (display_name, source, captured_by) VALUES ('Typed Co', 'manual', 'test') RETURNING id`).
		Scan(&companyID); err != nil {
		t.Fatalf("seeding a company: %v", err)
	}

	cases := []struct {
		value   string
		allowed bool
	}{
		{"customer", true},
		{"ext:finance-demo:portfolio_holding", true},
		{"ext:vn:reseller", true},
		{"banana", false},
		{"ext:", false},
		{"ext:vn", false},
		{"ext:vn:", false},
		{"ext::reseller", false},
		{"ext:VN:reseller", false},
		{"ext:vn:Reseller", false},
		{"ext:-vn:reseller", false},
		{"ext:vn:re seller", false},
		{"EXT:vn:reseller", false},
	}
	for _, c := range cases {
		_, err := conn.Exec(ctx,
			`INSERT INTO company_relationship_type (company_id, relationship_type, source, captured_by)
			 VALUES ($1, $2, 'manual', 'test')`, companyID, c.value)
		if c.allowed {
			if err != nil {
				t.Errorf("type %q was refused: %v", c.value, err)
			}
			continue
		}
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23514" ||
			pgErr.ConstraintName != "company_relationship_type_relationship_type_check" {
			t.Errorf("type %q: err = %v, want 23514 on company_relationship_type_relationship_type_check", c.value, err)
		}
	}
}
