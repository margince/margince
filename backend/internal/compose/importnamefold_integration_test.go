// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The import's company-name fold is spelled twice and must mean once.
//
// employerKey folds the spreadsheet cell in Go; f_fold_import_name folds the
// stored name inside the index Postgres maintains. They cannot share an
// implementation and they are the two halves of one equality — the lookup
// compares a key from the first against a value from the second, so a
// disagreement is not a crash. It is a file that stops linking, silently, for
// whichever names the two disagree about, and nobody finds out because a
// missing link reads exactly like a company that is not in the CRM.
//
// So the corpus is deliberately awkward. A round of ordinary names would agree
// under almost any two implementations; what separates them is the whitespace
// nobody types on purpose.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
)

func TestTheImportNameFoldAgreesInBothLanguages(t *testing.T) {
	e := integration.Setup(t)
	names := []struct{ what, name string }{
		{"a plain name", "Terralogic"},
		{"mixed case", "TerraLogic GmbH"},
		{"leading and trailing space", "  Terralogic  "},
		{"a double space inside", "Terra  logic"},
		{"a tab", "Terra\tlogic"},
		{"a newline", "Terra\nlogic"},
		{"a carriage return", "Terra\r\nlogic"},
		{"a vertical tab", "Terra\vlogic"},
		{"a form feed", "Terra\flogic"},
		{"a non-breaking space", "Terra logic"},
		{"an em space", "Terra logic"},
		{"an ideographic space", "Terra　logic"},
		{"a name that is only space", " \t "},
		{"an accent, which is NOT folded", "Térralogic"},
		{"an apostrophe, which is NOT folded", "O'Terra"},
		{"a legal suffix, which is NOT stripped", "Terralogic Inc"},
	}

	for _, c := range names {
		t.Run(c.what, func(t *testing.T) {
			var inSQL string
			if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
				return tx.QueryRow(context.Background(),
					`SELECT coalesce(f_fold_import_name($1), '')`, c.name).Scan(&inSQL)
			}); err != nil {
				t.Fatalf("folding %q in SQL: %v", c.name, err)
			}
			if inGo := employerKey(c.name); inGo != inSQL {
				t.Errorf("%q folds to %q in Go and %q in SQL. The import compares one against the "+
					"other, so every company whose stored name folds this way stops being findable "+
					"by a file that spells it this way — and the run reports a missing link, which "+
					"reads exactly like a company nobody has",
					c.name, inGo, inSQL)
			}
		})
	}
}
