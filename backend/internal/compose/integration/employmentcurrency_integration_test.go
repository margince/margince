// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
)

// The client test reads this same corpus; both sides must implement its statuses
// and date precision instead of maintaining separate expected behaviours.
func TestEmploymentCurrencySharedClientServerCases(t *testing.T) {
	e := Setup(t)
	raw, err := os.ReadFile("../../shared/kernel/employment/testdata/currency.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name, Today            string
		End, Status, Precision *string
		Current                bool
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 8 {
		t.Fatal("currency corpus unexpectedly empty")
	}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		for _, c := range cases {
			var args []any
			arg := func(v any) string { args = append(args, v); return storekit.SQLf("$%d", len(args)) }
			predicate := employment.IsCurrentSQL(arg(c.End)+"::date", arg(c.Status)+"::text", arg(c.Precision)+"::text")
			predicate = strings.ReplaceAll(predicate, "current_date", arg(c.Today)+"::date")
			var got bool
			if err := tx.QueryRow(e.Admin(), "SELECT "+predicate, args...).Scan(&got); err != nil {
				return err
			}
			if got != c.Current {
				t.Errorf("%s: got %v want %v", c.Name, got, c.Current)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
