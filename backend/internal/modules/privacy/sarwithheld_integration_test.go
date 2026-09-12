// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

// No withheld column reaches the Art. 15 package, whatever the SQL spelling.
//
// This asks Postgres what each assembled section RETURNS, rather than asking a
// regexp what its text says. The distinction is not academic: the export's keys
// come straight from pgx's FieldDescriptions (sar.go's rowMaps), so the result
// columns ARE the package, and a name-matching check answers a different
// question than the one that matters.
//
// It was a real gap, not a hypothetical. The first version of this withholding
// searched the SELECT list for the literal `token_hash`, and four ordinary
// spellings walked straight past it — `SELECT ct.*`, `to_jsonb(ct)`,
// `row_to_json(ct)`, and a `/* from */` comment that made the text-splitter cut
// the SELECT list short. Every one of them ships the credential. Asking the
// database removes the whole class: a star expands to real column names before
// this ever sees them, and a whole-row constructor returns one composite column
// whose type is the table, which is checked for separately below.

import (
	"context"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// withheldFromTheExport names, per table, the columns an Art. 15 package must
// never carry. It mirrors sarWithholds in gates/piicoverage_test.go, which is
// the registration a reader is pointed at; this is the runtime half that
// actually holds the line.
//
// Two copies because backend/gates carries no non-test file and so is not an
// importable package. TestTheTwoWithholdingRegistriesAgree reads the gate's
// source and fails when they diverge — without it, registering a column there
// and forgetting it here would leave the airtight half not looking.
var withheldFromTheExport = map[string][]string{
	"confirm_token": {"token_hash", "delivered_to"},
}

// The gate's registration and this one describe the same rule, so they must name
// the same tables and columns.
//
// Read from the gate's SOURCE rather than restated, because a hand-kept mirror
// is what this test exists to catch. The direction that matters most is a
// column registered in the gate and missing here: the gate's own check is a name
// deny-list four SQL spellings walk past, so a column only IT knows about is one
// nothing effectively guards.
func TestTheTwoWithholdingRegistriesAgree(t *testing.T) {
	raw, err := os.ReadFile("../../../gates/piicoverage_test.go")
	if err != nil {
		t.Fatalf("reading the withholding registration: %v", err)
	}

	// Each entry reads `sarWithholds: []string{"a", "b"}` inside the piiTables
	// literal for a table, so the table name is the nearest preceding key.
	entry := regexp.MustCompile(`(?s)"([a-z_]+)":\s*\{[^}]*?sarWithholds:\s*\[\]string\{([^}]*)\}`)
	matches := entry.FindAllStringSubmatch(string(raw), -1)
	if len(matches) == 0 {
		t.Fatal("no sarWithholds registration found in the gate, so this comparison has no " +
			"subject and would pass however far the two had drifted")
	}

	registered := map[string][]string{}
	for _, m := range matches {
		var columns []string
		for _, c := range regexp.MustCompile(`"([a-z_]+)"`).FindAllStringSubmatch(m[2], -1) {
			columns = append(columns, c[1])
		}
		registered[m[1]] = columns
	}

	for table, columns := range registered {
		for _, column := range columns {
			if !slices.Contains(withheldFromTheExport[table], column) {
				t.Errorf("the gate registers %s.%s as withheld from the Art. 15 export, but "+
					"withheldFromTheExport does not name it — the gate's own check is a name "+
					"deny-list that several SQL spellings walk past, so this column is currently "+
					"guarded by nothing that holds", table, column)
			}
		}
	}
	for table, columns := range withheldFromTheExport {
		for _, column := range columns {
			if !slices.Contains(registered[table], column) {
				t.Errorf("this check probes %s.%s, which the gate no longer registers as withheld. "+
					"Either the withholding was lifted — and this probe should go with it — or the "+
					"registration was dropped by accident", table, column)
			}
		}
	}
}

func TestNoWithheldColumnReachesTheAssembledExport(t *testing.T) {
	ownerDSN := os.Getenv("MARGINCE_TEST_DSN")
	if ownerDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing owner connection: %v", err)
		}
	})
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}

	var pkg SARPackage
	sections := sarSections(&pkg, ids.New[ids.ContactKind](), []string{"subject@sar.test"}, []ids.UUID{ids.NewV7()}, []ids.UUID{ids.NewV7()})

	var probed int
	for _, section := range sections {
		table, columns := withheldTableIn(section.query)
		if table == "" {
			continue
		}
		probed++

		// What the top plan node RETURNS, which is the only complete answer.
		//
		// Reading the SQL text cannot settle this: `ct.token_hash AS outcome`
		// hides the column behind a harmless alias, `format('%%s', ct)`
		// serializes the whole row into ordinary text, and a `/* from */`
		// comment cuts a text-splitter short. Reading the result TYPES misses
		// all three too, because each returns a plain scalar.
		//
		// The planner has already resolved every alias, expanded every star and
		// looked inside every function argument, and it reports what the node
		// emits. Asking it removes the whole class rather than the four
		// spellings somebody thought of.
		for _, referenced := range topLevelOutput(ctx, t, owner, section.query) {
			for _, withheld := range columns {
				if !referencesColumn(referenced, withheld) {
					continue
				}
				t.Errorf("the assembled export returns %s from %s, however it is spelled or "+
					"aliased. It is a live bearer credential, or the address it was mailed to, "+
					"and an Art. 15 package assembled by an admin carries neither:\n%s",
					referenced, table, strings.TrimSpace(section.query))
			}
		}
	}

	if probed == 0 {
		t.Fatal("no assembled section reads a table with withheld columns, so this check examined " +
			"nothing — either the section was dropped or withheldFromTheExport names a table the " +
			"export no longer touches")
	}
}

// topLevelOutput asks the planner what the statement's OUTERMOST node emits.
//
// The top node only: an inner scan lists every column it reads, including ones
// the projection discards, so reading the whole plan would refuse a statement
// that merely joins the table. What the outermost node outputs is what reaches
// pgx, and what reaches pgx is the package.
//
// Explained rather than executed, for the reason the schema check beside this
// prepares rather than executing: this is a question about the statement, and
// answering it must not depend on rows existing.
func topLevelOutput(ctx context.Context, t *testing.T, owner *pgx.Conn, query string) []string {
	t.Helper()

	var plan []map[string]any
	if err := owner.QueryRow(ctx,
		"EXPLAIN (VERBOSE, COSTS OFF, FORMAT JSON) "+withPlaceholdersBound(query)).Scan(&plan); err != nil {
		t.Fatalf("explaining a section: %v — this check cannot tell what the statement returns "+
			"without a plan, so it must not pass by default:\n%s", err, strings.TrimSpace(query))
	}
	if len(plan) == 0 {
		t.Fatalf("the planner returned no plan for:\n%s", strings.TrimSpace(query))
	}

	node, ok := plan[0]["Plan"].(map[string]any)
	if !ok {
		t.Fatalf("the plan carries no top node for:\n%s", strings.TrimSpace(query))
	}
	output, ok := node["Output"].([]any)
	if !ok {
		t.Fatalf("the top plan node reports no Output for:\n%s", strings.TrimSpace(query))
	}

	var referenced []string
	for _, item := range output {
		if text, ok := item.(string); ok {
			referenced = append(referenced, text)
		}
	}
	return referenced
}

// withPlaceholdersBound replaces $N with a typed NULL so EXPLAIN can plan the
// statement without a PREPARE round trip. The values never matter — the question
// is what the projection names, which no argument changes.
func withPlaceholdersBound(query string) string {
	return regexp.MustCompile(`\$\d+`).ReplaceAllString(query, "NULL")
}

// referencesColumn reports whether a plan output item carries the given column —
// as `alias.column`, as a bare `column`, or inside a whole-row reference, which
// carries every column including this one.
//
// The table is not a parameter: the caller has already established that this
// statement reads the withholding table, and a plan node's output is not
// qualified consistently enough to re-derive it here. Asking about the column
// alone is what makes the whole-row case answerable at all — that spelling names
// no column, and no table either.
func referencesColumn(output, column string) bool {
	if regexp.MustCompile(`(?i)(?:^|[^a-z0-9_.])(?:[a-z_][a-z0-9_]*\.)?` +
		regexp.QuoteMeta(column) + `\b`).MatchString(output) {
		return true
	}
	// A whole-row reference names no column at all and carries every one of
	// them. The planner spells it `ct.*` when an alias survives into the plan
	// and a bare `*` when it does not — a single-relation plan drops the
	// qualifier, so matching only the qualified form let `format('%s', ct)`
	// through. Both spellings are refused, and a bare `*` is never a legitimate
	// projection in a section over this table: every column the subject is owed
	// here is named.
	return regexp.MustCompile(`(?i)(?:^|[^a-z0-9_.])(?:[a-z_][a-z0-9_]*\.)?\*`).
		MatchString(output)
}

// withheldTableIn reports the withheld-column table a statement reads, if any.
func withheldTableIn(query string) (string, []string) {
	for _, m := range fromJoin.FindAllStringSubmatch(query, -1) {
		if columns, ok := withheldFromTheExport[m[1]]; ok {
			return m[1], columns
		}
	}
	return "", nil
}
