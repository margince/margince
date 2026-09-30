// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

//go:build !integration

package gates

// Every table the bump trigger is attached to has a NOT NULL version column
// for it to bump.
//
// set_updated_at_bump_version() assigns `version = OLD.version + 1`. On a table
// without that column the assignment raises `record "new" has no field
// "version"` and takes the whole transaction with it — not at CREATE TRIGGER
// time, which succeeds against any table, but on the first UPDATE the table
// ever receives. activity_review_response carried the trigger from the day it
// was created and stayed quiet for as long as it did because it is written by
// INSERT only: the first UPDATE was a migration's own data sweep, by which time
// installations held rows and the migration could not complete on any of them.
// A nullable column is the same hazard one step quieter — `NULL + 1` is NULL,
// so the counter stops moving and nothing raises.
//
// The subject set is the catalog's own trigger records, which is the schema a
// database really ends up with: a trigger dropped in a later migration, and a
// trigger that went with the table under it, are both already gone from it.
// Every timing counts, not only the BEFORE UPDATE one a bump has to be to bump
// anything, because an assignment to a field the record does not have raises
// wherever it runs.
//
// versionguard derives the same attachments from the migration sources, for a
// question this one does not ask. It is read back here as the fail-short alarm:
// a subject set that stopped seeing triggers reports a clean census, and the
// two derivations disagreeing is what says so.

import (
	"strings"
	"testing"
)

// bumpFunctionCall is the function as a trigger's EXECUTE clause spells it,
// which is the form the catalog prints.
const bumpFunctionCall = bumpFunction + "()"

// bumpTableFloor is the smallest subject count this gate may derive and still
// be believed. It sits well below the real count because its job is to catch a
// reader that broke, not to track the schema.
const bumpTableFloor = 40

func TestEveryBumpTriggeredTableHasAVersionToBump(t *testing.T) {
	t.Parallel()

	schema := parseHeadCatalog(t)

	carrying := map[string]bool{}
	for name, table := range schema.tables {
		if carriesBumpTrigger(table) {
			carrying[name] = true
		}
	}
	if len(carrying) < bumpTableFloor {
		t.Fatalf("found only %d table(s) carrying %s in the head catalog and expects at least %d — "+
			"the catalog reader is broken, not the schema", len(carrying), bumpFunctionCall, bumpTableFloor)
	}

	for _, name := range sortedKeys(carrying) {
		version, has := columnNamed(schema.tables[name], "version")
		switch {
		case !has:
			t.Errorf("%s carries %s and has no version column, so the first UPDATE it receives fails with "+
				`record "new" has no field "version"`+" — give it one, or attach set_updated_at() instead when the table counts its changes some other way",
				name, bumpFunctionCall)
		case !version.notNull:
			t.Errorf("%s.version is nullable, so a row holding NULL bumps to NULL and the counter silently stops moving", name)
		}
	}

	// A table the migrations attach the trigger to and the catalog does not
	// hold at all was dropped, taking its triggers with it.
	for _, name := range sortedKeys(versionBumpingTables(t)) {
		if _, stillExists := schema.tables[name]; stillExists && !carrying[name] {
			t.Errorf("the migrations leave %s carrying %s and the catalog records no such trigger on it: "+
				"one of the two readings is wrong, and this census is only as good as the catalog's", name, bumpFunctionCall)
		}
	}
}

func carriesBumpTrigger(table *emTable) bool {
	for _, trigger := range table.triggers {
		if strings.Contains(trigger.def, bumpFunctionCall) {
			return true
		}
	}
	return false
}

func columnNamed(table *emTable, name string) (emColumn, bool) {
	for _, column := range table.columns {
		if column.name == name {
			return column, true
		}
	}
	return emColumn{}, false
}
