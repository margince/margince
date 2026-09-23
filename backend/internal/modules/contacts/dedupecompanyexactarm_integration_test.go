// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// The company candidate query's exact-name arm, asked directly.
//
// It cannot be asked through the query: the arms are ORed and a trigram one
// answers first for every pair that can be constructed, so a pair reaches the
// scorer whether or not this arm admits it. What the arm exists for is that the
// lane NOT DEPEND on a similarity threshold to find a name it already calls the
// same name — and only a direct question holds that.

import (
	"testing"

	"github.com/jackc/pgx/v5"
)

// Pairs companyNamesAreTheSame calls one name. Company-shaped rather than
// borrowed from the contact table: a registered name carries the punctuation
// and the casing a filing office wrote, and it is what arrives from a register
// import beside a name somebody typed.
var goEqualCompanyNames = []struct {
	name          string
	stored, fresh string
}{
	{"identical", "Baqend GmbH", "Baqend GmbH"},
	{"shouted", "Baqend GmbH", "BAQEND GMBH"},
	{"untrimmed", "Baqend GmbH", "  Baqend GmbH  "},
	{"accents", "Zürich Versicherung", "Zurich Versicherung"},
	{"sharp s", "Straßen Logistik", "Strassen Logistik"},
	// Reflowed internal whitespace: how a name read off a register page or a
	// crawled imprint arrives beside the same name typed by hand.
	{"internal whitespace", "Baqend GmbH", "Baqend  GmbH"},
}

func TestTheCompanyNameKeyArmAdmitsEveryGoEqualPair(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	for _, tc := range goEqualCompanyNames {
		t.Run(tc.name, func(t *testing.T) {
			// FATAL rather than a skip. The guard asks the comparison the lane
			// decides on, and a row it does not call equal asserts nothing
			// about the arm while looking exactly like a row that passed.
			if !companyNamesAreTheSame(tc.stored, tc.fresh) {
				t.Fatalf("companyNamesAreTheSame does not call %q and %q one name, so this row asserts "+
					"nothing about the arm — fix the comparison or drop the row", tc.stored, tc.fresh)
			}
			var admitted bool
			if err := e.store.tx(ctx, func(tx pgx.Tx) error {
				return tx.QueryRow(ctx,
					`SELECT `+exactNameKeySQL("$1")+` = `+exactNameKeySQL("$2"),
					tc.stored, tc.fresh).Scan(&admitted)
			}); err != nil {
				t.Fatalf("ask the arm about %q and %q: %v", tc.stored, tc.fresh, err)
			}
			if !admitted {
				t.Errorf("the exact-name arm does not admit %q against %q, which the company comparison "+
					"calls one name — the lane is left depending on the trigram arms' threshold for it",
					tc.fresh, tc.stored)
			}
		})
	}
}

// The suffix strip must NOT reach the exact arm. searchAxes folds "Baqend
// GmbH" and "Baqend Inc" onto one trigram axis on purpose, because they are
// worth SCORING against each other; calling them the same name would offer a
// reviewer a merge of two legal entities.
func TestTheExactArmDoesNotFoldAwayALegalForm(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	var admitted bool
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT `+exactNameKeySQL("$1")+` = `+exactNameKeySQL("$2"),
			"Baqend GmbH", "Baqend Inc").Scan(&admitted)
	}); err != nil {
		t.Fatalf("ask the arm about two legal forms: %v", err)
	}
	if admitted {
		t.Error("the exact-name arm folded two legal forms onto one name — a reviewer would be shown " +
			"a name collision between two companies that are not one company")
	}
}
