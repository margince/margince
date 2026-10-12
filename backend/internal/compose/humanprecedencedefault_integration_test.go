// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
)

// Only a quoted literal on a text or enum column is a default nobody chose. A
// number, boolean, date, uuid or jsonb default is read as a value a human may
// have meant — product.active = true is a decision to sell it — so the tie goes
// to asking them. Read off real columns, because the type is the column's and
// not the rendered expression's.
func TestOnlyATextOrEnumDefaultIsNobodysEdit(t *testing.T) {
	e := integration.Setup(t)
	ctx := context.Background()
	tx, err := e.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rolling back the probe table: %v", err)
		}
	}()
	if _, err := tx.Exec(ctx, `
		CREATE TYPE pg_temp.default_probe_mood AS ENUM ('calm', 'busy');
		CREATE TEMP TABLE default_probe (
		  id int,
		  a_text text DEFAULT 'unknown',
		  a_varchar varchar(20) DEFAULT 'it''s',
		  an_enum pg_temp.default_probe_mood DEFAULT 'calm',
		  a_date date DEFAULT '2024-01-01',
		  a_uuid uuid DEFAULT '00000000-0000-0000-0000-000000000000',
		  a_jsonb jsonb DEFAULT '[]',
		  a_bool boolean DEFAULT true,
		  a_numeric numeric DEFAULT 0,
		  a_quoted_bigint bigint DEFAULT '0',
		  a_stamp timestamptz DEFAULT now(),
		  an_array text[] DEFAULT '{}'
		) ON COMMIT DROP;
		INSERT INTO default_probe (id) VALUES (1)`); err != nil {
		t.Fatalf("creating the probe table: %v", err)
	}
	for column, want := range map[string]*string{
		"a_text": new("unknown"), "a_varchar": new("it's"), "an_enum": new("calm"),
		"a_date": nil, "a_uuid": nil, "a_jsonb": nil, "a_bool": nil,
		"a_numeric": nil, "a_quoted_bigint": nil, "a_stamp": nil, "an_array": nil,
	} {
		var got *string
		if err := tx.QueryRow(ctx, `SELECT (`+constantColumnDefault+`)
			FROM default_probe t CROSS JOIN (SELECT $1::text AS key) p`, column).Scan(&got); err != nil {
			t.Fatalf("%s: %v", column, err)
		}
		if (got == nil) != (want == nil) || (got != nil && *got != *want) {
			t.Errorf("the default of %s reads as %q (absent: %v), want %q (absent: %v)",
				column, deref(got), got == nil, deref(want), want == nil)
		}
	}
}
