// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// backend/gates/moneysum_test.go reads the catalog's TEXT: it can tell that a
// CHECK tying gross to net + tax is written, not that Postgres refuses the row it
// exists to refuse. This is that other half.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// offerRow is the INSERT both directions use. The deal_id is deliberately a
// dangling reference: a CHECK is evaluated as the tuple is formed while the
// foreign key is an AFTER trigger, so a disagreeing triple fails on the CHECK
// before the reference is ever looked at. That is what lets this prove the
// constraint without standing up a company, pipeline, stage and deal to hang one
// offer off — and the agreeing case below reads the DIFFERENT failure as its
// evidence, which is the same fact from the other side.
const offerRow = `
	INSERT INTO offer (deal_id, offer_number, currency, source, captured_by,
	                   net_minor, tax_minor, gross_minor)
	VALUES (gen_random_uuid(), 'OF-1', 'EUR', 'manual', 'human:test', $1, $2, $3)`

func TestAnOfferGrossIsItsNetPlusTax(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()

	// 100 + 19 is 119, and this row says 200.
	_, err := conn.Exec(ctx, offerRow, 100, 19, 200)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("an offer whose gross is not net + tax was accepted (%v) — the three numbers are summed "+
			"in Go and written back independently, so nothing else refuses a write-back that drifted", err)
	}
	if pgErr.Code != "23514" || pgErr.ConstraintName != "offer_gross_is_net_plus_tax" {
		t.Fatalf("a disagreeing triple failed with code %q constraint %q, want 23514 on "+
			"offer_gross_is_net_plus_tax", pgErr.Code, pgErr.ConstraintName)
	}

	// The same row with a gross that adds up gets PAST the CHECK and dies on the
	// dangling deal instead. Naming that SECOND failure is what makes this a
	// measurement: "no sum-check error" would also be true of a row that was
	// stored, or of a check that is not there at all.
	_, err = conn.Exec(ctx, offerRow, 100, 19, 119)
	if !errors.As(err, &pgErr) {
		t.Fatalf("an offer naming no deal was not refused at all (%v) — this case only "+
			"says something about the sum check while the row still fails on its deal", err)
	}
	if pgErr.Code != "23503" || pgErr.ConstraintName != "offer_deal_fkey" {
		t.Fatalf("an offer whose gross IS net + tax failed with code %q constraint %q, want "+
			"23503 on offer_deal_fkey — the sum check admitted it or something else refused it",
			pgErr.Code, pgErr.ConstraintName)
	}
}
