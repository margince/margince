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
// The migration sources are replayed here too, as the fail-short alarm. A
// subject set that stopped seeing triggers reports a clean census, and the two
// derivations disagreeing is what says so. This cross-check is why the replay
// stays while the other schema gates read the catalog alone.

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
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

var (
	// triggerAttachment captures a trigger's name, its event list and its table.
	// It is applied per statement, never per line. Those parts and the execute
	// clause sit on different lines, so a line match would find no attachment.
	triggerAttachment = regexp.MustCompile(`(?is)\bCREATE\s+(?:OR\s+REPLACE\s+)?(?:CONSTRAINT\s+)?TRIGGER\s+([a-z_][a-z0-9_]*)\s+(BEFORE|AFTER|INSTEAD\s+OF)\s+(.*?)\bON\s+([a-z_][a-z0-9_]*)`)

	// triggerExecutes captures the function the execute clause names, the only
	// part of a trigger statement that says what the trigger runs.
	triggerExecutes = regexp.MustCompile(`(?is)\bEXECUTE\s+(?:FUNCTION|PROCEDURE)\s+([a-z_][a-z0-9_]*)\s*\(`)

	// triggerDrop captures the trigger name and table a drop detaches. Postgres
	// scopes a trigger name to its table, so the pair is what the drop removes.
	triggerDrop = regexp.MustCompile(`(?is)\bDROP\s+TRIGGER\s+(?:IF\s+EXISTS\s+)?([a-z_][a-z0-9_]*)\s+ON\s+([a-z_][a-z0-9_]*)`)
)

// attachedTrigger identifies one trigger as Postgres does: its name, scoped to
// the table it is attached to.
type attachedTrigger struct{ name, table string }

// triggerBody is the function a trigger executes, and whether it fires before
// an update.
type triggerBody struct {
	function     string
	beforeUpdate bool
}

// versionBumpingTables derives the tables the migrations leave with a live
// attachment of the bump function, fired before an update. Both migration
// namespaces are read, because the fork seam may attach its own.
func versionBumpingTables(t *testing.T) map[string]bool {
	t.Helper()
	live := map[attachedTrigger]triggerBody{}
	// dbmigrate applies namespaces in this order, and inside each orders
	// versions as strings, so the walk's lexical order is the apply order.
	for _, root := range []string{"migrations/core", "migrations/custom"} {
		err := gatekit.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".up.sql") {
				return err
			}
			raw, err := gatekit.ReadFile(path) // #nosec G304 G122 -- path is a *.up.sql file from walking the trusted migrations tree
			if err != nil {
				return err
			}
			applyTriggerStatements(sqlLineComment.ReplaceAllString(withCurrentNames(string(raw)), ""), live)
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}

	bumped := map[string]bool{}
	for trigger, body := range live {
		if body.beforeUpdate && body.function == bumpFunction {
			bumped[trigger.table] = true
		}
	}
	return bumped
}

// applyTriggerStatements folds one migration's trigger statements into the live
// state: an attachment records what the trigger executes, a drop removes it. A
// drop of one trigger leaves every other trigger on the same table standing.
func applyTriggerStatements(text string, live map[attachedTrigger]triggerBody) {
	for stmt := range strings.SplitSeq(text, ";") {
		if m := triggerDrop.FindStringSubmatch(stmt); m != nil {
			delete(live, attachedTrigger{name: m[1], table: m[2]})
			continue
		}
		m := triggerAttachment.FindStringSubmatch(stmt)
		if m == nil {
			continue
		}
		// An execute clause the scan cannot read is recorded as executing
		// nothing, replacing what the same trigger executed before.
		body := triggerBody{beforeUpdate: strings.EqualFold(m[2], "BEFORE") && updateEvent.MatchString(m[3])}
		if executes := triggerExecutes.FindStringSubmatch(stmt); executes != nil {
			body.function = executes[1]
		}
		live[attachedTrigger{name: m[1], table: m[4]}] = body
	}
}
