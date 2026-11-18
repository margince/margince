// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package rowvalue

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAnExportCellSpellsEachDriverValueAsTheValueItHolds(t *testing.T) {
	member := ids.MustParse("0195f3a0-0000-7000-8000-000000000001")
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"a numeric with a trailing zero", pgtype.Numeric{Int: big.NewInt(10), Exp: -1, Valid: true}, "1.0"},
		{"a fractional numeric", pgtype.Numeric{Int: big.NewInt(92), Exp: -2, Valid: true}, "0.92"},
		{"a negative numeric keeps its sign unquoted", pgtype.Numeric{Int: big.NewInt(-55), Exp: -1, Valid: true}, "-5.5"},
		{"a numeric NaN", pgtype.Numeric{NaN: true, Valid: true}, "NaN"},
		{"a numeric infinity", pgtype.Numeric{InfinityModifier: pgtype.Infinity, Valid: true}, "Infinity"},
		{"a numeric minus infinity", pgtype.Numeric{InfinityModifier: pgtype.NegativeInfinity, Valid: true}, "-Infinity"},
		{"a null numeric", pgtype.Numeric{}, ""},
		{"an interval", pgtype.Interval{Months: 14, Days: 3, Microseconds: 3_600_000_000, Valid: true}, "14 mon 3 day 01:00:00"},
		{"a null interval", pgtype.Interval{}, ""},
		{"a jsonb object", map[string]any{"score": 0.5, "tags": []any{"a"}}, `{"score":0.5,"tags":["a"]}`},
		{"a uuid array", []any{[16]byte(member)}, `["` + member.String() + `"]`},
		{"a jsonb string that opens a formula", []any{"=1+1"}, `["=1+1"]`},
		{"a float", 0.92, "0.92"},
		{"a large float is not in exponent form", 1e21, "1000000000000000000000"},
		{"a jsonb integer above 2^53 keeps its digits", map[string]any{"n": json.Number("9007199254740993")}, `{"n":9007199254740993}`},
		{"a negative jsonb number stays unquoted", json.Number("-5"), "-5"},
		{"a negative int32 stays unquoted", int32(-7), "-7"},
		{"an ampersand in jsonb stays as stored", map[string]any{"name": "A&B <x>"}, `{"name":"A&B <x>"}`},
		{"a Stringer of user text is guarded", typedText("=1+1"), "'=1+1"},
	}
	for _, c := range cases {
		got, err := CSVCell(c.in)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: CSVCell = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestAJSONCellThatOpensAFormulaIsGuarded(t *testing.T) {
	// A jsonb scalar string arrives as a Go string. A jsonb array is guarded as
	// one cell, so only a lead at the start of the cell counts.
	got, err := CSVCell("=HYPERLINK(1)")
	if err != nil || got != "'=HYPERLINK(1)" {
		t.Fatalf("CSVCell = %q, %v; want the guarded literal", got, err)
	}
}

// typedText stands for a driver type csvCell has no case for, whose text a
// user typed.
type typedText string

func (t typedText) String() string { return string(t) }
