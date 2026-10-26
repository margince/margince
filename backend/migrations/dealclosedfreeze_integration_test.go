// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// A closed deal freezes the rate and the converted amount together, and the
// catalog's text can only show that the CHECK is written. This is the other
// half: that Postgres refuses the row it exists to refuse.
//
// Before the rule was tightened, a closed deal could carry a frozen rate and
// no converted amount. It then dropped out of every base-currency roll-up
// while reading as correctly closed. A null base amount is skipped by `SUM`
// rather than converted at an invented rate. That is the right behaviour, and
// it is what makes a half-frozen row invisible.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// closedDeal is the INSERT both directions use, and its pipeline_id and
// stage_id dangle by design.
//
// A `CHECK` is evaluated as the tuple is formed, while a foreign key is an
// `AFTER` trigger. So a half-frozen deal fails on the `CHECK` before either
// reference is read, and this probe needs no company, pipeline or stage. The
// whole-freeze case reads the other failure, the foreign key, as its evidence
// that the `CHECK` had nothing to say.
const closedDeal = `
	INSERT INTO deal (name, pipeline_id, stage_id, source, captured_by,
	                  status, closed_at, amount_minor, currency,
	                  fx_rate_to_base, amount_minor_base)
	VALUES ('Freeze probe', gen_random_uuid(), gen_random_uuid(), 'manual', 'human:test',
	        'won', now(), 100000, 'USD', $1, $2)`

func TestAClosedDealFreezesTheRateAndTheConvertedAmountTogether(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()

	// The rate is frozen and the converted amount is not, which is the state
	// the old rule admitted.
	_, err := conn.Exec(ctx, closedDeal, 0.92, nil)
	var half *pgconn.PgError
	if !errors.As(err, &half) {
		t.Fatalf("a closed deal with a rate and no converted amount was accepted (%v): "+
			"it would read as closed and be missing from every base-currency total", err)
	}
	if half.Code != "23514" || half.ConstraintName != "deal_closed_fx" {
		t.Fatalf("the half freeze failed with code %q constraint %q, want 23514 on deal_closed_fx",
			half.Code, half.ConstraintName)
	}

	// Both frozen. This must reach the foreign key, which is the evidence that
	// the CHECK no longer has anything to say about it.
	_, err = conn.Exec(ctx, closedDeal, 0.92, 92000)
	var whole *pgconn.PgError
	if !errors.As(err, &whole) {
		t.Fatalf("a wholly frozen deal reached no refusal at all (%v): this probe hangs "+
			"its row off references that do not exist, so one is owed", err)
	}
	if whole.Code == "23514" {
		t.Fatalf("a wholly frozen deal was refused by %q: the rule must admit the state the "+
			"freeze writes", whole.ConstraintName)
	}
	if whole.Code != "23503" {
		t.Fatalf("a wholly frozen deal failed with code %q, want 23503 from the dangling "+
			"pipeline or stage reference", whole.Code)
	}
}
