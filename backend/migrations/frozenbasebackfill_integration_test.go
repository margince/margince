// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// The frozen-base backfill, replayed over the state it was written for.
//
// currency_minor_digits lists only the currencies that DEVIATE from ISO's two
// decimals, so an ordinary base currency — EUR, USD, GBP — has no row there.
// 1788583500 wrote its base-side scale as
//
//	(SELECT coalesce(bd.digits, 2) FROM currency_minor_digits bd WHERE ...)
//
// which defaults a NULL digits COLUMN rather than a missing ROW, and the column
// is NOT NULL so that can never happen. An absent row still returned NULL and
// carried the whole conversion with it, emptying the frozen amount of every
// closed deal on such an installation — the base-currency ones converting at a
// rate of 1 along with the foreign ones.
//
// The loss is silent: every reader treats NULL as "no frozen figure" and drops
// the row, so closed revenue and the rollups shrink without reporting an error.
//
// Both migration texts are replayed from the shipped FILES, so this cannot pass
// against SQL that no longer ships.

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

var (
	frozenFixturePipeline = ids.MustParse("01920000-0000-7000-8000-0000000000e1")
	frozenFixtureStage    = ids.MustParse("01920000-0000-7000-8000-0000000000e2")
	frozenDealEUR         = ids.MustParse("01920000-0000-7000-8000-0000000000e3")
	frozenDealVND         = ids.MustParse("01920000-0000-7000-8000-0000000000e4")
	frozenDealJPY         = ids.MustParse("01920000-0000-7000-8000-0000000000e5")
)

// replayMigration runs one shipped migration's up text over whatever the test
// seeded. The FILE, not a copy of its statements.
func replayMigration(t *testing.T, conn *pgx.Conn, name string) {
	t.Helper()
	up, err := os.ReadFile(filepath.Join("core", name))
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	if _, err := conn.Exec(context.Background(), string(up)); err != nil {
		t.Fatalf("replaying %s: %v", name, err)
	}
}

// rewindFrozenBaseMigration puts deal.amount_minor_base back the way
// 1788583500 found it — the generated column, with its scale-blind expression —
// so the test can seed closed deals and replay the migration over them.
func rewindFrozenBaseMigration(t *testing.T, conn *pgx.Conn) {
	t.Helper()
	if _, err := conn.Exec(context.Background(), `
		ALTER TABLE deal DROP COLUMN amount_minor_base;
		ALTER TABLE deal ADD COLUMN amount_minor_base bigint
			GENERATED ALWAYS AS ((round(((amount_minor)::numeric * fx_rate_to_base)))::bigint) STORED`); err != nil {
		t.Fatalf("undoing the frozen-base migration: %v", err)
	}
}

// seedFrozenBaseFixture writes a EUR-base installation and three closed deals:
// one in the base currency, one in a zero-decimal currency that HAS a
// currency_minor_digits row, and one more of the same to prove the base side
// rather than the deal side is what was broken.
func seedFrozenBaseFixture(t *testing.T, conn *pgx.Conn) {
	t.Helper()
	ctx := context.Background()
	if _, err := conn.Exec(ctx,
		`INSERT INTO setting (key, value) VALUES ('installation.base_currency', '"EUR"')
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`); err != nil {
		t.Fatalf("seeding the base currency: %v", err)
	}
	if _, err := conn.Exec(ctx,
		`INSERT INTO pipeline (id, name) VALUES ($1, 'Frozen')`, frozenFixturePipeline); err != nil {
		t.Fatalf("seeding the fixture pipeline: %v", err)
	}
	if _, err := conn.Exec(ctx,
		`INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
		 VALUES ($1, $2, 'Won', 1, 'won', 100)`,
		frozenFixtureStage, frozenFixturePipeline); err != nil {
		t.Fatalf("seeding the fixture stage: %v", err)
	}
	for _, d := range []struct {
		id       ids.UUID
		currency string
		amount   int64
		rate     string
	}{
		{frozenDealEUR, "EUR", 1800000, "1.0000000000"},
		{frozenDealVND, "VND", 2400000000, "0.0000350000"},
		{frozenDealJPY, "JPY", 15000000, "0.0060000000"},
	} {
		if _, err := conn.Exec(ctx,
			`INSERT INTO deal (id, pipeline_id, stage_id, name, status, closed_at,
			                   amount_minor, currency, fx_rate_to_base, source, captured_by)
			 VALUES ($1, $2, $3, 'Closed', 'won', now(), $4, $5, $6::numeric, 'manual', 'test')`,
			d.id, frozenFixturePipeline, frozenFixtureStage, d.amount, d.currency, d.rate); err != nil {
			t.Fatalf("seeding the %s deal: %v", d.currency, err)
		}
	}
}

func frozenBaseOf(t *testing.T, conn *pgx.Conn, deal ids.UUID) *int64 {
	t.Helper()
	var v *int64
	if err := conn.QueryRow(context.Background(),
		`SELECT amount_minor_base FROM deal WHERE id = $1`, deal).Scan(&v); err != nil {
		t.Fatalf("reading the frozen base amount: %v", err)
	}
	return v
}

func TestTheFrozenBaseBackfillSurvivesATwoDecimalBaseCurrency(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)

	rewindFrozenBaseMigration(t, conn)
	seedFrozenBaseFixture(t, conn)

	replayMigration(t, conn, "1788583500_a_frozen_base_amount_knows_both_scales.up.sql")
	replayMigration(t, conn, "1788610648_a_two_digit_base_currency_is_not_a_missing_one.up.sql")

	// EUR against a EUR base at a rate of 1: the amount is already in the base
	// currency and both scales are two, so the conversion must be the identity.
	// This is the case that shows the defect was never about foreign currency —
	// a base-side NULL empties the plainest row in the book.
	for _, want := range []struct {
		deal   ids.UUID
		label  string
		amount int64
	}{
		{frozenDealEUR, "a EUR deal against a EUR base", 1800000},
		// VND 2,400,000,000 (zero decimals = ₫2,400,000,000) at 0.000035
		// EUR/VND is €84,000, which is 8,400,000 EUR minor units.
		{frozenDealVND, "a VND deal against a EUR base", 8400000},
		// JPY 15,000,000 (zero decimals = ¥15,000,000) at 0.006 EUR/JPY is
		// €90,000 = 9,000,000 EUR minor units.
		{frozenDealJPY, "a JPY deal against a EUR base", 9000000},
	} {
		got := frozenBaseOf(t, conn, want.deal)
		if got == nil {
			t.Fatalf("%s froze NULL, want %d — a base currency absent from currency_minor_digits is an ordinary two-decimal currency, not a missing scale, and every closed deal on such an installation drops out of closed revenue and the rollups",
				want.label, want.amount)
		}
		if *got != want.amount {
			t.Fatalf("%s froze %d, want %d", want.label, *got, want.amount)
		}
	}
}

func TestTheFrozenBaseBackfillLeavesARowItCannotConvert(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()

	rewindFrozenBaseMigration(t, conn)
	seedFrozenBaseFixture(t, conn)

	// No base-currency setting: the destination scale is genuinely unknown, and
	// guessing two is the assumption both migrations exist to remove.
	if _, err := conn.Exec(ctx, `DELETE FROM setting WHERE key = 'installation.base_currency'`); err != nil {
		t.Fatal(err)
	}

	replayMigration(t, conn, "1788583500_a_frozen_base_amount_knows_both_scales.up.sql")
	replayMigration(t, conn, "1788610648_a_two_digit_base_currency_is_not_a_missing_one.up.sql")

	if got := frozenBaseOf(t, conn, frozenDealVND); got != nil {
		t.Fatalf("with no base currency configured the frozen amount is %d, want NULL — a converted figure against an unknown destination scale is a guess presented as a stored fact", *got)
	}
}
