// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The team's frozen outlook and the rep's are the same fact over different
// books, so they must have the same shape.
//
// They cannot be one table: the rep's hangs off weekly_review by foreign key
// and the team's off team_weekly_review. That leaves two column lists which a
// contact maintains, and a figure added to one and not the other is invisible —
// the panel for the table that has it draws it, the other draws nothing, and
// nothing fails.
//
// So the mirror is declared and this holds it, in BOTH directions: a column
// added to either side, or a type changed on one, fails here. The parent key
// and the primary key are the two deliberate differences and are named below.
//
// The corpus comes from the committed catalog, which every migration builds.
// A column added later by ALTER TABLE is there; the first CREATE TABLE lacks it.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A column line of the committed catalog: table, column and its type.
var outlookColumn = regexp.MustCompile(`^public\.(\w+)\.(\w+) (.+?)(?: NOT NULL)? gen=`)

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
	raw, err := os.ReadFile(filepath.Join("migrations", "testdata", "head_catalog.txt"))
	if err != nil {
		t.Fatalf("reading the committed catalog: %v", err)
	}
	rep := outlookColumns(string(raw), "weekly_review_outlook")
	team := outlookColumns(string(raw), "team_weekly_review_outlook")

	// Under-recognition is the failure that reports PASS: a scan that found no
	// columns would agree with itself perfectly.
	if len(rep) < 10 || len(team) < 10 {
		t.Fatalf("read %d rep and %d team outlook columns: the scan has stopped "+
			"seeing its subject, which agrees with itself and holds nothing",
			len(rep), len(team))
	}
	for _, finding := range outlookMirrorDrift(rep, team) {
		t.Error(finding)
	}
}

// A column added by a later ALTER TABLE on one side only is drift, and the
// scan must see it.
func TestALaterColumnOnOneOutlookIsDrift(t *testing.T) {
	t.Parallel()
	catalog := strings.Join([]string{
		"public.weekly_review_outlook.won_minor bigint gen=- def=-",
		"public.team_weekly_review_outlook.won_minor bigint gen=- def=-",
		"public.team_weekly_review_outlook.added_later integer NOT NULL gen=- def=0",
	}, "\n")
	drift := outlookMirrorDrift(outlookColumns(catalog, "weekly_review_outlook"),
		outlookColumns(catalog, "team_weekly_review_outlook"))
	if len(drift) != 1 || !strings.Contains(drift[0], "added_later") {
		t.Errorf("a column on the team side only reported %v, want one finding naming it", drift)
	}
}

// outlookMirrorDrift names every column on one side and not the other, and every
// column whose type differs.
func outlookMirrorDrift(rep, team map[string]string) []string {
	var out []string
	for name, repType := range rep {
		teamType, found := team[name]
		switch {
		case !found:
			out = append(out, fmt.Sprintf("weekly_review_outlook has %q and team_weekly_review_outlook does not: "+
				"the team's landing would silently lack a figure the rep's reports", name))
		case repType != teamType:
			out = append(out, fmt.Sprintf("column %q is %s on the rep's outlook and %s on the team's: "+
				"the same figure must be the same type or one of them rounds differently",
				name, repType, teamType))
		}
	}
	for name := range team {
		if _, found := rep[name]; !found {
			out = append(out, fmt.Sprintf("team_weekly_review_outlook has %q and weekly_review_outlook does not: "+
				"add it to both, or the two panels answer differently about the same week", name))
		}
	}
	return out
}

// outlookColumns reads one table's columns from the catalog, with the parent
// key and the id removed: those two differ by design.
func outlookColumns(catalog, table string) map[string]string {
	cols := map[string]string{}
	for line := range strings.SplitSeq(catalog, "\n") {
		m := outlookColumn.FindStringSubmatch(line)
		if m == nil || m[1] != table {
			continue
		}
		if name := m[2]; name != "id" && name != outlookParentKeys[table] {
			cols[name] = m[3]
		}
	}
	return cols
}
