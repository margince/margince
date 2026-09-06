// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The team's frozen outlook and the rep's are the same fact over different
// books, so they are the same SHAPE or one of them is lying.
//
// They cannot be one table: the rep's hangs off weekly_review by foreign key
// and the team's off team_weekly_review. That leaves two column lists which a
// person maintains, and a figure added to one and not the other is invisible —
// the panel for the table that has it draws it, the other draws nothing, and
// nothing fails.
//
// So the mirror is declared and this holds it, in BOTH directions: a column
// added to either side, or a type changed on one, fails here. The parent key
// and the primary key are the two deliberate differences and are named below.
//
// The corpus is DERIVED from the migrations rather than listed: a gate that
// hard-codes the columns it protects stops protecting the next one somebody
// adds, and reports PASS while doing it.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A column line inside a CREATE TABLE body: a name and the type that follows.
var outlookColumn = regexp.MustCompile(`^\s{4}(\w+)\s+([\w()]+)`)

// gatekit:fixture each table's own parent column, which is the one difference
// between the two and cannot be read off either side
//
// The two columns that MUST differ, and nothing else may.
//
// Each table names its own parent, which is the whole reason there are two.
// Everything else — every figure, every window bound, every snapshot pointer —
// is the same question asked of a different book.
var outlookParentKeys = map[string]string{
	"weekly_review_outlook":      "weekly_review_id",
	"team_weekly_review_outlook": "team_weekly_review_id",
}

func TestTheTeamOutlookMirrorsTheRepOutlook(t *testing.T) {
	t.Parallel()

	rep := outlookColumns(t, "weekly_review_outlook")
	team := outlookColumns(t, "team_weekly_review_outlook")

	// Under-recognition is the failure that reports PASS: a scan that found no
	// columns would agree with itself perfectly.
	if len(rep) < 10 || len(team) < 10 {
		t.Fatalf("read %d rep and %d team outlook columns: the scan has stopped "+
			"seeing its subject, which agrees with itself and holds nothing",
			len(rep), len(team))
	}

	for name, repType := range rep {
		teamType, found := team[name]
		if !found {
			t.Errorf("weekly_review_outlook has %q and team_weekly_review_outlook does not: "+
				"the team's landing would silently lack a figure the rep's reports", name)
			continue
		}
		if repType != teamType {
			t.Errorf("column %q is %s on the rep's outlook and %s on the team's: "+
				"the same figure must be the same type or one of them rounds differently",
				name, repType, teamType)
		}
	}
	for name := range team {
		if _, found := rep[name]; !found {
			t.Errorf("team_weekly_review_outlook has %q and weekly_review_outlook does not: "+
				"add it to both, or the two panels answer differently about the same week", name)
		}
	}
}

// outlookColumns reads one table's columns out of the migration that creates
// it, with the parent key and the id removed — those are the two deliberate
// differences, and comparing them would fail every run.
func outlookColumns(t *testing.T, table string) map[string]string {
	t.Helper()
	body := createTableBody(t, table)
	cols := map[string]string{}
	for _, line := range strings.Split(body, "\n") {
		m := outlookColumn.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := m[1]
		if name == "id" || name == outlookParentKeys[table] {
			continue
		}
		// CONSTRAINT lines start with the keyword, not a column name.
		if strings.EqualFold(name, "constraint") {
			continue
		}
		cols[name] = m[2]
	}
	return cols
}

// createTableBody is the text between CREATE TABLE <name> ( and its closing
// paren, from whichever migration creates it.
func createTableBody(t *testing.T, table string) string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("migrations", "core", "*.up.sql"))
	if err != nil {
		t.Fatalf("listing migrations: %v", err)
	}
	open := "CREATE TABLE " + table + " ("
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("reading %s: %v", file, err)
		}
		text := string(raw)
		start := strings.Index(text, open)
		if start < 0 {
			continue
		}
		rest := text[start+len(open):]
		end := strings.Index(rest, "\n);")
		if end < 0 {
			t.Fatalf("%s: CREATE TABLE %s is not closed", file, table)
		}
		return rest[:end]
	}
	t.Fatalf("no migration creates %s: the gate's subject is gone, which is not a pass", table)
	return ""
}
