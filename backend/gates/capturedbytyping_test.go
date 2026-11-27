// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind shape H2

package gates

// `captured_by` records the principal, and a principal is not a user row.
//
// The value every store stamps is prefixed by kind — "human:<id>" for a
// session, "agent:<id>" for a passport — because an agent acting under
// somebody's passport has no row in app_user. A column typed `uuid`, or one
// carrying a foreign key to app_user, therefore refuses every write the
// product makes, and the refusal reaches a caller as an internal error.
//
// This is a fitness function rather than a fix, because the defect it catches
// already shipped once: migration 0262 typed the contract table's column
// `uuid REFERENCES app_user(id)` and every contract insert 500'd, with the
// whole gate suite green. Nothing derived the obligation from the tree, so
// nothing noticed that one table disagreed with the other thirty.
//
// It reads each column as the head catalog states it, not every historical
// declaration. A shipped migration is never edited: 0262 stays wrong forever and
// 0265 is what repairs it. What must hold is the shape an installation ends up with.

import (
	"testing"
)

// capturedByFloor is the fewest captured_by columns the catalog may yield and
// still be believed. It sits below the real count, to catch a reader that broke.
const capturedByFloor = 20

func TestCapturedByIsAlwaysTextAndNeverAUserForeignKey(t *testing.T) {
	t.Parallel()
	schema := parseHeadCatalog(t)
	declared := 0
	for _, name := range schema.tableNames() {
		table := schema.tables[name]
		column, ok := columnNamed(table, "captured_by")
		if !ok {
			continue
		}
		declared++
		if column.dataType != "text" {
			t.Errorf("%s leaves captured_by as %q — it holds a prefixed principal "+
				"(\"human:<id>\" / \"agent:<id>\"), so every write against a non-text column fails",
				name, column.dataType)
		}
		if table.singleColumnFKs["captured_by"].parent == "app_user" {
			t.Errorf("%s leaves captured_by with a foreign key to app_user — an agent acting under "+
				"a passport is not a row there, so the key refuses the write instead of recording who made it",
				name)
		}
	}
	if declared < capturedByFloor {
		t.Fatalf("only %d captured_by columns found in %s — the reader lost its source", declared, headCatalogPath)
	}
}
