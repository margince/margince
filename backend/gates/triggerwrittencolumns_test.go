// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// A statement may not write a column its table's trigger already writes.
//
// `set_updated_at` and `set_updated_at_bump_version` are BEFORE UPDATE ... FOR
// EACH ROW triggers. A BEFORE ROW trigger runs after the statement's SET has
// been applied to NEW, so it OVERWRITES what the statement assigned: on a
// trigger-touched table, a statement's own `updated_at = now()` or
// `version = version + 1` is dead. Not wrong — dead. The same value, assigned
// twice.
//
// Dead code that looks load-bearing is worse than dead code that looks dead,
// and this shape is the second kind. company.go's manual `version = version + 1`
// read as a fourth approach to concurrent editing across the organization
// writers and was counted as one; coldstartprofile.go's overwrite arm carried
// `updated_at = now()` where its fill arm did not, and the difference read as
// deliberate. Neither was anything. The file next door already names the
// hazard in its own words, about a different trigger:
//
//	Nothing about geocoding here: a trigger marks the coordinates stale on any
//	address column that changes, so this writer neither can nor has to
//	remember. An earlier version did it in this statement — correct, and
//	something the next address writer would not have known to copy.
//
// The one honest exception is a TOUCH: a statement whose only purpose is to be
// a genuine UPDATE of the row, so the trigger fires and moves the version an
// approval pinned. It needs an assignment to exist at all, and `updated_at =
// now()` is the honest one to pick. That shape is recognized rather than
// trusted — the assignment must be the statement's ONLY assignment — and the
// site is still registered below with the reason it needs the bump, because
// "this row must move" is a claim about the product and not about SQL.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// touchTriggerLine reads one trigger out of the head catalog. The CATALOG
// rather than the migration sources, because the catalog is the schema a
// database really ends up with — it is dumped from one and gated against one
// (migrations/headcatalog_integration_test.go), so it already accounts for a
// trigger created in one migration and dropped in another, which reading the
// CREATE statements would have to re-derive and could get wrong in the
// direction of a quiet pass.
//
// UNCONDITIONAL BEFORE UPDATE only, and that is the fail-closed half of the
// pattern. `BEFORE UPDATE OF a, b` fires only when the statement writes one of
// those columns, and a `WHEN (…)` trigger only when its predicate holds — under
// either, a statement's own assignment is not always overwritten, so calling it
// dead would delete a live write. Neither is hypothetical: channel_connection's
// touch trigger carries `WHEN ((to_jsonb(old.*) - 'poll_offset') IS DISTINCT
// FROM …)`, so a poll-offset-only write does NOT fire it, and this pattern
// leaves that table alone. The gate refuses to guess rather than guessing in
// the direction that deletes code.
var touchTriggerLine = regexp.MustCompile(
	`(?i)^public\.([a-z_][a-z0-9_]*)\.[a-z_][a-z0-9_]* CREATE TRIGGER [a-z_][a-z0-9_]* BEFORE UPDATE ON public\.[a-z_][a-z0-9_]* FOR EACH ROW EXECUTE FUNCTION (set_updated_at|set_updated_at_bump_version)\(\)$`)

// The columns each trigger function assigns to NEW, read off 0001_baseline's
// definitions. Two functions, spelled out rather than parsed: the bodies are in
// the catalog too, and a parser for plpgsql assignments to buy two entries
// would be more code than the thing it read.
//
// triggerFunctionBodies below is what keeps that from being a claim nobody
// checks — it fails if either function's body stops matching this table.
var triggerWrites = map[string][]string{
	"set_updated_at":              {"updated_at"},
	"set_updated_at_bump_version": {"updated_at", "version"},
}

// writeSegment is one write inside a statement: the table, and the assignment
// list that lands on it. A statement can hold several — a CTE that updates two
// tables, an upsert whose conflict arm writes the table its INSERT named.
type writeSegment struct {
	table       string
	assignments string
}

// segmentOpener matches the head of a write: `UPDATE t [AS] [alias] SET`, or
// the `INSERT INTO t … ON CONFLICT … DO UPDATE SET` whose written table is the
// one the INSERT named. The upsert arm is not an afterthought — a BEFORE UPDATE
// trigger fires on the conflict arm exactly as it does on a plain UPDATE, and
// the tree writes that shape (people/projectcompany.go).
//
// The alias is optional and SET is a legal identifier, so `UPDATE t SET x = 1`
// could read as table `t` with alias `SET` and then find no SET clause. It does
// not: the alternation is preference-ordered, the alias branch fails on the SET
// that has to follow it, and the match falls back to the aliasless reading.
//
// The upsert arm may not cross a statement separator, or an INSERT and an
// unrelated later UPDATE would read as one write on the INSERT's table.
var segmentOpener = regexp.MustCompile(
	`(?is)\bINSERT\s+INTO\s+([a-z_][a-z0-9_]*)\b[^;]*?\bON\s+CONFLICT\b[^;]*?\bDO\s+UPDATE\s+SET\b` +
		`|\bUPDATE\s+(?:ONLY\s+)?([a-z_][a-z0-9_]*)(?:\s+(?:AS\s+)?[a-z_][a-z0-9_]*)?\s+SET\b`)

// setClauseEnd ends an assignment list. Matched at PAREN DEPTH ZERO by the
// scan below rather than with a lazy regex: `SET fields = ARRAY(SELECT c FROM
// unnest(…) WHERE c IS NOT NULL), version = version + 1` is a shape this tree
// writes, and a regex ending at the first FROM stops inside that subquery — so
// every assignment after it becomes invisible and reordering the list, which
// nobody reviews as a schema change, turns the check off.
var setClauseEnd = regexp.MustCompile(`(?i)^(from|where|returning)\b`)

// writeSegments pairs every write in a statement with the assignment list that
// lands on it.
//
// It is not gates/piicoverage_test.go's setAssignments, which answers a
// narrower question for a different census: the first SET clause of a statement
// that has ALREADY been split, with no table attached. Here the table is half
// the answer — `activity` and `activity_participant` both carry updated_at and
// only one of them is trigger-touched — and a statement holding two writes has
// to be read as two.
func writeSegments(statement string) []writeSegment {
	var out []writeSegment
	for _, loc := range segmentOpener.FindAllStringSubmatchIndex(statement, -1) {
		table := ""
		for _, group := range [2]int{1, 2} {
			if loc[2*group] >= 0 {
				table = strings.ToLower(statement[loc[2*group]:loc[2*group+1]])
			}
		}
		if table == "" {
			continue
		}
		rest := statement[loc[1]:]
		depth, end := 0, len(rest)
		for pos := 0; pos < len(rest); pos++ {
			switch rest[pos] {
			case '(':
				depth++
			case ')':
				if depth == 0 {
					end = pos
					pos = len(rest)
					continue
				}
				depth--
			}
			if depth == 0 && (pos == 0 || !isWordByte(rest[pos-1])) && setClauseEnd.MatchString(rest[pos:]) {
				end = pos
				break
			}
		}
		out = append(out, writeSegment{table: table, assignments: strings.TrimSpace(rest[:end])})
	}
	return out
}

func isWordByte(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// assignedColumns returns the columns an assignment list writes — the targets
// at paren depth zero, so a column named inside a subquery's own WHERE is not
// mistaken for one being written.
func assignedColumns(assignments string) []string {
	var out []string
	depth, start := 0, 0
	for pos := 0; pos <= len(assignments); pos++ {
		if pos == len(assignments) || (assignments[pos] == ',' && depth == 0) {
			if column := assignmentTarget(assignments[start:pos]); column != "" {
				out = append(out, column)
			}
			start = pos + 1
			continue
		}
		switch assignments[pos] {
		case '(':
			depth++
		case ')':
			depth--
		}
	}
	return out
}

// assignmentTargetRe reads the column an assignment writes. Anchored at both
// ends of the target so `updated_at_source = $1` is not read as a write of
// `updated_at`, and a qualified target (`c.status = $2`, which UPDATE … FROM
// allows) is read as the column it names.
var assignmentTargetRe = regexp.MustCompile(`(?is)^\s*(?:[a-z_][a-z0-9_]*\.)?([a-z_][a-z0-9_]*)\s*=`)

func assignmentTarget(assignment string) string {
	m := assignmentTargetRe.FindStringSubmatch(assignment)
	if m == nil {
		return ""
	}
	return strings.ToLower(m[1])
}

// touchStatements are the ratified deliberate touches: a statement whose whole
// job is to BE an update, so the trigger fires and moves a version somebody
// pinned. Keyed by "path:declaration".
//
// A touch is recognized by shape as well as registered — the assignment has to
// be the statement's only one — so an entry here cannot wave through a second
// dead assignment that arrives in the same function later. What the register
// adds is the product claim the shape cannot carry: WHY this row has to move.
var touchStatements = gatekit.Waive(map[string]string{
	"internal/modules/activities/relinkbatch.go:relinkActivityRow":  "a staged approval pins activity.version, and that pin is the only thing between an approved \"send this body on this conversation\" and the conversation being repointed before the approval redeems; a relink that changes who the activity reaches must move the version the pin re-checks",
	"internal/modules/capture/sinkproject.go:linkActivityToProject": "the automatic half of the same relink: filing under a project changes who the activity reaches, so it must move the version a staged approval pinned, for the reason the human path gives",
	"internal/modules/people/linkedinmatchapply.go:touchPerson":     "a LinkedIn handle decision changes what the contact record says without writing a person column, and the contact's own version is what an editor's If-Match is checked against; the row is locked first, so the bump is not a blind write",
})

// touchTriggerFloor is the smallest number of trigger-touched tables this gate
// may derive and still be believed. It sits well below the real count because
// its job is to catch a catalog that stopped being read — a renamed file, a
// changed dump format — rather than to track the schema.
const touchTriggerFloor = 40

// judgedWriteFloor is the same alarm one level in: the number of statements
// this gate reads and finds writing a trigger-touched table. A reader that
// stops seeing statements produces no findings, which is indistinguishable from
// a clean tree.
const judgedWriteFloor = 120

// touchTriggerTables derives, per table, the columns a BEFORE UPDATE touch
// trigger already writes.
func touchTriggerTables(t *testing.T) map[string]map[string]bool {
	t.Helper()
	raw, err := os.ReadFile("migrations/testdata/head_catalog.txt")
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	tables := map[string]map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		m := touchTriggerLine.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		columns, known := triggerWrites[strings.ToLower(m[2])]
		if !known {
			t.Fatalf("trigger function %s touches tables and this gate does not know what it writes — add it to triggerWrites", m[2])
		}
		if tables[m[1]] == nil {
			tables[m[1]] = map[string]bool{}
		}
		for _, column := range columns {
			tables[m[1]][column] = true
		}
	}
	if len(tables) < touchTriggerFloor {
		t.Fatalf("derived only %d trigger-touched table(s) from the head catalog and expects at least %d — "+
			"the catalog reader is broken, not the schema", len(tables), touchTriggerFloor)
	}
	return tables
}

// TestNoStatementWritesAColumnItsTriggerAlreadyWrites is the census.
func TestNoStatementWritesAColumnItsTriggerAlreadyWrites(t *testing.T) {
	t.Parallel()
	defer touchStatements.AssertAllMatched(t)
	touched := touchTriggerTables(t)
	judged := 0
	for _, root := range []string{"internal/modules", "internal/compose", "internal/platform", "internal/shared"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") ||
				strings.HasSuffix(path, "_test.go") || isIntegrationTagged(path) {
				return err
			}
			path = filepath.ToSlash(path)
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return err
			}
			for _, decl := range file.Decls {
				name := declarationName(decl)
				for _, statement := range gatekit.SQLStatementsOf(decl) {
					for _, segment := range writeSegments(statement) {
						columns := touched[segment.table]
						if columns == nil {
							continue
						}
						judged++
						written := assignedColumns(segment.assignments)
						var dead []string
						for _, column := range written {
							if columns[column] {
								dead = append(dead, column)
							}
						}
						if len(dead) == 0 {
							continue
						}
						// A TOUCH: nothing else is assigned, so the
						// assignment is not a second answer to a question
						// the trigger already answers — it is how the
						// statement exists at all.
						if len(written) == len(dead) {
							if touchStatements.Waived(t, path+":"+name) {
								continue
							}
							t.Errorf("%s: %s runs `UPDATE %s SET %s` to fire the touch trigger — that is a real shape, but it has to say what version it is moving and why; ratify it in touchStatements",
								path, name, segment.table, strings.Join(dead, ", "))
							continue
						}
						t.Errorf("%s: %s assigns %s on %s, which that table's BEFORE UPDATE trigger already writes — the trigger runs after the SET is applied to NEW and overwrites it, so the assignment is dead. Delete it; the next writer of this table would otherwise copy it, or read a sibling statement without it as a deliberate difference",
							path, name, strings.Join(dead, ", "), segment.table)
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if judged < judgedWriteFloor {
		t.Fatalf("this census read %d write(s) to a trigger-touched table and expects at least %d — "+
			"the reader has stopped seeing statements rather than the tree having lost them", judged, judgedWriteFloor)
	}
}

// declarationName names the declaration a statement was found in, for the
// message and the register key. Package-level statement tables are as much a
// site as a function is — aiactivity/store.go holds its upsert in a const.
func declarationName(decl ast.Decl) string {
	switch typed := decl.(type) {
	case *ast.FuncDecl:
		return typed.Name.Name
	case *ast.GenDecl:
		for _, spec := range typed.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if ok && len(value.Names) > 0 {
				return value.Names[0].Name
			}
		}
	}
	return "the file"
}

// TestTheTouchTriggerFunctionsStillWriteWhatThisGateSays holds triggerWrites,
// which is the one thing above that is asserted rather than derived. A trigger
// function that stops bumping version — or starts — makes every ruling here
// wrong in the direction of deleting a live assignment.
func TestTheTouchTriggerFunctionsStillWriteWhatThisGateSays(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("migrations/testdata/head_catalog.txt")
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	catalog := string(raw)
	for function, columns := range triggerWrites {
		body := functionBody(catalog, function)
		if body == "" {
			t.Fatalf("the head catalog holds no definition of %s, which this gate rules from", function)
		}
		for _, column := range []string{"updated_at", "version"} {
			assigns := strings.Contains(body, "NEW."+column+" =")
			declared := false
			for _, c := range columns {
				declared = declared || c == column
			}
			if assigns != declared {
				t.Errorf("%s %s NEW.%s, and triggerWrites says it %s — every deletion this gate ruled on that column was ruled from the wrong list",
					function, boolWord(assigns, "assigns", "does not assign"), column,
					boolWord(declared, "does", "does not"))
			}
		}
	}
}

func boolWord(b bool, yes, no string) string {
	if b {
		return yes
	}
	return no
}

// functionBody returns the body of one function as the catalog holds it: a
// `public.<name>() secdef=…` header line, the plpgsql block, then a blank line.
// The parentheses are part of the search or set_updated_at would find
// set_updated_at_bump_version's header and rule from the wrong body.
func functionBody(catalog, function string) string {
	start := strings.Index(catalog, "\npublic."+function+"() ")
	if start < 0 {
		return ""
	}
	rest := catalog[start+1:]
	header := strings.Index(rest, "\n")
	if header < 0 {
		return ""
	}
	body := rest[header+1:]
	if end := strings.Index(body, "\n\n"); end >= 0 {
		return body[:end]
	}
	return body
}
