// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migrations_test

import (
	"maps"
	"regexp"
	"testing"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
)

// activityClockRepair re-creates the activity clock functions for a database
// whose upgrade applied an older body last.
const activityClockRepair = "1794229944"

// functionStatementRe matches one CREATE OR REPLACE FUNCTION statement through
// the `$$;` that closes its body, capturing the name and the body.
var functionStatementRe = regexp.MustCompile(`(?s)CREATE OR REPLACE FUNCTION (\w+)\([^;]*?AS \$\$(.*?)\$\$;`)

// functionStatements returns name → statement and name → body for sql.
func functionStatements(sql string) (statements, bodies map[string]string) {
	statements, bodies = map[string]string{}, map[string]string{}
	for _, m := range functionStatementRe.FindAllStringSubmatch(sql, -1) {
		statements[m[1]], bodies[m[1]] = m[0], m[2]
	}
	return statements, bodies
}

// repairAndPrior returns the repair migration and, for each function it
// defines, the last statement for that function in the migrations before it.
func repairAndPrior(t *testing.T) (repair dbmigrate.Migration, prior map[string]string) {
	t.Helper()
	core, _ := namespaces(t)
	prior = map[string]string{}
	for _, m := range core.Migrations {
		if m.Version == activityClockRepair {
			return m, prior
		}
		statements, _ := functionStatements(m.UpSQL)
		maps.Copy(prior, statements)
	}
	t.Fatalf("core ships no migration %s", activityClockRepair)
	return repair, prior
}

// The repair must set each function to the body a fresh database already has
// at that point. It then changes nothing there and only repairs an upgrade.
func TestTheActivityClockRepairCopiesTheBodiesBeforeIt(t *testing.T) {
	repair, prior := repairAndPrior(t)
	statements, _ := functionStatements(repair.UpSQL)
	if len(statements) != 3 {
		t.Fatalf("the repair defines %d functions, want the 3 activity clock functions", len(statements))
	}
	for name, statement := range statements {
		if prior[name] != statement {
			t.Errorf("%s: the repair's statement differs from the last migration before it that defines the function. "+
				"Copy that statement unchanged", name)
		}
	}
}
