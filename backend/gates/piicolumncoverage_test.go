// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// The Art. 17 redaction, judged by COLUMN rather than by table.
//
// piicoverage_test.go asks whether erasure writes a registered PII table at
// all. That question is satisfied by any UPDATE touching it, which is why
// `comms_outbound.bounce_recipient` — a plaintext subject address — shipped
// with every gate green: the table was already written, so a new column on it
// was covered by a claim that had stopped being about the column set. The
// adversarial review caught it; the census did not. That is rule 8's
// under-recognition failure, and it will repeat on the next column added to an
// already-registered table.
//
// So this reads the SET clauses the cascade actually carries and compares them
// against the CATALOG. A text or jsonb column on a registered table is either
// cleared by the redaction, or named in the baseline below as one that was not.
// A column that is neither is new, and new is what the last one was.
//
// TEXT AND JSONB ONLY, deliberately. A timestamp, a uuid or an integer cannot
// hold a sentence somebody wrote about a subject; those are the columns a
// redaction leaves standing on purpose, and asking about them would drown the
// answer. An enum stored as text is swept in and lands in the baseline, which
// is the right cost of not having to guess.

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// erasureColumnBaseline is what each registered table's uncleared text and
// jsonb columns WERE when this gate was written.
//
// IT IS A BASELINE, NOT A BLESSING. Nothing here is a claim that the column
// holds no subject data — several of them plainly might, and judging them is
// its own piece of work (#3468). What the list does is fix the SET, so the
// next column added to one of these tables cannot arrive unremarked the way
// bounce_recipient did. A column removed from the schema must leave here too,
// or the entry starts standing for nothing.
//
// Adding a line to this list is therefore a decision to be read, not a way past
// a failing gate: the gate's message says to clear the column, or to say here
// why it stays.
var erasureColumnBaseline = map[string][]string{
	// A decision's own vocabulary: what kind of message it was, what the engine
	// and the old gate each answered, and which rollout mode was in force. Every
	// value is drawn from a closed set this repository defines — none of it is
	// anything the subject wrote or anything written about them, so the Art. 17
	// redaction leaves it standing. What it DOES clear on this table is the
	// recipient address and the subject link, which is the identifying half.
	"communication_decision": {
		"basis",
		"legacy_verdict",
		"mode",
		"phase",
		"reason_code",
		"requested_category",
		"resolved_category",
		"suppression",
		"verdict",
		"actor",
		// evidence holds RECORD IDS and nothing else — the activity, deal,
		// invoice or consent-event a decision rested on. The records themselves
		// are erased on their own terms by the statements that own them; a uuid
		// pointing at a row that has been scrubbed reveals nothing, and clearing
		// it here would destroy the controller's ability to say which evidence
		// it relied on for a send it has already made.
		"evidence",
	},
	"activity": {
		"audience",
		"audience_reason",
		"capture_label",
		// One of two words this repository defines, saying whether the message
		// asked its recipient side for something. Neither is anything the
		// subject wrote nor anything written about them — it is a reading OF the
		// text, and the text itself is cleared on its own terms.
		"owed_verdict",
		"captured_by",
		"channel_provider",
		"direction",
		"kind",
		"language",
		"meeting_status",
		// Who caused the row to exist — human, agent, or the product's own
		// remediation work. A closed enum about the WRITER, never about the
		// subject, so erasure has nothing to clear here.
		"origin",
		"source",
		"source_system",
	},
	"activity_participant": {
		"role",
	},
	"approval": {
		"diff_hash",
		"effect_failure",
		"kind",
		"proposed_by",
		"target_entity_type",
	},
	"comms_outbound": {
		// The controller lane's vocabulary: which kind of sender, and which
		// registered wording. Both are this repository's own words rather than
		// anything a subject wrote or anything written about them, so the
		// Art. 17 scrub leaves them standing.
		//
		// payload_ref is deliberately NOT here: erasure_payloads.go clears it,
		// after destroying the vault material the reference names.
		"sender_kind",
		"template_key",
		"bounce_kind",
		"bounce_reason",
		"consent_purpose",
		"from_name",
		"in_reply_to",
		"message_id",
		"provider",
		"provider_message_id",
		"reason",
		"references_chain",
		"status",
		"thread_key",
	},
	"lead": {
		"captured_by",
		"disqualify_note",
		"linkedin_url",
		"score_override_reason",
		"source",
		"source_id",
		"source_system",
		"status",
		"status_set_by",
	},
	"person": {
		"captured_by",
		"photo_object_key",
		"photo_origin",
		"source",
		"visibility",
	},
	"provider_run": {
		"last_safe_status_code",
		"provider",
		"requested_categories",
		"skip_reason",
		"state",
		"trigger",
	},
}

// catalogColumnRe reads one column line out of the schema catalog:
// `public.<table>.<column> <type> …`. Constraint and index lines share the
// prefix and are excluded by the type token, which for them is FOREIGN /
// CHECK / UNIQUE / PRIMARY rather than a type name.
var catalogColumnRe = regexp.MustCompile(`^public\.([a-z_][a-z0-9_]*)\.([a-z_][a-z0-9_]*) (text|jsonb)\b`)

// catalogTextColumns reads the text and jsonb columns of every table out of
// the committed schema catalog, which is the same artifact the migration gates
// hold current — so a column added by a migration reaches this census through
// the file its own gate already forces to be regenerated.
func catalogTextColumns(t *testing.T) map[string]map[string]string {
	t.Helper()
	const catalog = "migrations/testdata/head_catalog.txt"
	file, err := os.Open(catalog)
	if err != nil {
		t.Fatalf("opening %s: %v", catalog, err)
	}
	//craft:ignore swallowed-errors a read-only close on a file this test is finished with; the read itself is asserted above
	defer func() { _ = file.Close() }()
	// Keyed to the column's TYPE, not merely its presence: both text and jsonb
	// are collected here, and a failure that calls a jsonb column a text one
	// sends the reader to check the wrong thing and to doubt the finding while
	// they do it.
	out := map[string]map[string]string{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		m := catalogColumnRe.FindStringSubmatch(scanner.Text())
		if m == nil {
			continue
		}
		if out[m[1]] == nil {
			out[m[1]] = map[string]string{}
		}
		out[m[1]][m[2]] = m[3]
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("reading %s: %v", catalog, err)
	}
	if len(out) == 0 {
		t.Fatalf("%s yielded no columns — the catalog format moved and this census now reads an empty schema", catalog)
	}
	return out
}

// sqlConstants collects the package-level string constants the cascade splices
// into its statements, keyed by the name the call site spells — bare inside its
// own package, qualified where it crosses one.
//
// WITHOUT THIS THE CENSUS READS HALF A STATEMENT. `UPDATE approval SET ` +
// blankStagedProposal and `UPDATE provider_run SET` +
// storekit.ScrubProviderRunColumns both carry their entire SET clause in a
// constant, so a scan of literals alone sees a redaction that assigns nothing
// — and a table whose every column then looks unaccounted for is a table this
// gate would report as needing a baseline it does not need.
type declaration struct {
	name  string
	pkg   string
	value ast.Expr
}

func sqlConstants(t *testing.T, dirs ...string) map[string]string {
	t.Helper()
	out := map[string]string{}
	var declared []declaration
	for _, dir := range dirs {
		pkg := filepath.Base(dir)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("reading %s: %v", dir, err)
		}
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".go") {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			for _, decl := range parsed.Decls {
				gen, isGen := decl.(*ast.GenDecl)
				// VAR as well as CONST. Several of the cascade's statements are
				// package-level `var name = ` + backtick strings — a raw string
				// holding a whole statement is spelled that way as often as not
				// — and reading only constants left those calls opaque.
				if !isGen || (gen.Tok != token.CONST && gen.Tok != token.VAR) {
					continue
				}
				for _, spec := range gen.Specs {
					value, isValue := spec.(*ast.ValueSpec)
					// Names and values one-to-one, or the pairing is by
					// position and an iota run hands back somebody else's text.
					if !isValue || len(value.Names) != len(value.Values) {
						continue
					}
					for i, ident := range value.Names {
						declared = append(declared, declaration{
							name: ident.Name, pkg: pkg, value: value.Values[i],
						})
					}
				}
			}
		}
	}
	// TWO PASSES, because a declaration can be built out of its neighbours: a
	// statement is often a raw string plus a fragment held in another name, or
	// plus a call that produces a predicate. The first pass takes the plain
	// literals; the second renders the rest against them, so a value whose
	// WHERE clause is assembled still yields its readable SET clause instead of
	// the whole declaration going dark.
	for pass := 0; pass < 2; pass++ {
		for _, d := range declared {
			text := renderSQL(d.value, out)
			if pass == 0 && strings.Contains(text, unresolved) {
				continue
			}
			out[d.name] = text
			out[d.pkg+"."+d.name] = text
		}
	}
	return out
}

// unresolved marks a spliced part this scan could not render, IN PLACE. Keeping
// its position is the whole point: a fragment in a WHERE predicate hides no
// assignment and the census carries on, while one inside a SET clause hides
// exactly what the census is counting, and that is a finding.
const unresolved = "\x00unresolved\x00"

// renderSQL flattens one string-building expression into the text the compiler
// would produce, resolving the constants spliced into it. A part it cannot
// resolve — a run-time value, a function call, a constant from somewhere this
// scan does not read — becomes the marker above rather than disappearing, so
// what it stood in for can still be judged by where it stood.
func renderSQL(expr ast.Expr, consts map[string]string) string {
	switch node := expr.(type) {
	case *ast.BasicLit:
		if node.Kind != token.STRING {
			return unresolved
		}
		text, err := strconv.Unquote(node.Value)
		if err != nil {
			return unresolved
		}
		return text
	case *ast.Ident:
		if text, known := consts[node.Name]; known {
			return text
		}
		return unresolved
	case *ast.SelectorExpr:
		pkg, isIdent := node.X.(*ast.Ident)
		if !isIdent {
			return unresolved
		}
		if text, known := consts[pkg.Name+"."+node.Sel.Name]; known {
			return text
		}
		return unresolved
	case *ast.ParenExpr:
		return renderSQL(node.X, consts)
	case *ast.CallExpr:
		// The statement is the FORMAT STRING. A cascade statement built with
		// Sprintf carries its columns in the format and its values in the
		// arguments, so rendering the format is rendering the statement — and
		// rendering nothing loses person's and lead's redactions entirely.
		//
		// ReplaceAll is the same shape one level along: what it substitutes is
		// a bind position, not a column or a table.
		if formatting(node) && len(node.Args) > 0 {
			return renderSQL(node.Args[0], consts)
		}
		return unresolved
	case *ast.BinaryExpr:
		if node.Op != token.ADD {
			return unresolved
		}
		return renderSQL(node.X, consts) + renderSQL(node.Y, consts)
	}
	return unresolved
}

// formatting reports whether a call merely rearranges a statement it is handed
// — Sprintf and ReplaceAll — as against one that BUILDS it out of something
// this scan cannot see. The first can be read through; the second is what the
// unplaceable-write finding is for.
func formatting(call *ast.CallExpr) bool {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name == "sprintf" || fn.Name == "Sprintf"
	case *ast.SelectorExpr:
		return fn.Sel.Name == "Sprintf" || fn.Sel.Name == "ReplaceAll"
	}
	return false
}

// cascadeStatements returns every SQL statement the Art. 17 cascade executes,
// with its spliced constants resolved, plus the statements it could not
// resolve. An unresolved one that writes a registered table is a finding: this
// census cannot say what such a statement clears, and reporting a pass over it
// is the shape of failure the file's own header is about.
func cascadeStatements(t *testing.T, consts map[string]string) []string {
	t.Helper()
	var out []string
	for _, path := range erasureCascadeFiles {
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, decl := range parsed.Decls {
			fn, isFn := decl.(*ast.FuncDecl)
			if !isFn {
				continue
			}
			// The statements a HELPER is handed live at its call sites, which
			// this scan reads on their own. An argument naming one of this
			// function's own parameters is a pass-through, not a statement, and
			// reporting it unplaceable would be a finding about nothing.
			passthrough := map[string]bool{}
			if fn.Type.Params != nil {
				for _, field := range fn.Type.Params.List {
					for _, name := range field.Names {
						passthrough[name.Name] = true
					}
				}
			}
			ast.Inspect(fn, func(n ast.Node) bool {
				arg, found := executedSQL(n)
				if !found {
					return true
				}
				if ident, isIdent := arg.(*ast.Ident); isIdent && passthrough[ident.Name] {
					return true
				}
				// Rendered WHOLE rather than literal by literal: a statement
				// whose SET clause lives in a constant is one statement, and
				// reading its halves separately finds a redaction that assigns
				// nothing.
				text := renderSQL(arg, consts)
				// The whole statement rendered to nothing but the marker — a
				// query held in a name this scan does not read. It may be a
				// redaction and it may not, and not knowing is the finding: it
				// is passed on as an unplaceable write rather than dropped for
				// not looking like SQL.
				if strings.TrimSpace(text) == unresolved {
					out = append(out, "UPDATE "+unresolved+" SET "+unresolved)
					return true
				}
				if !sqlish(text) {
					return true
				}
				out = append(out, sqlStatements(text)...)
				return true
			})
		}
	}
	return out
}

// executedSQL answers the argument a pgx call EXECUTES, if the node is one.
//
// The SQL POSITION, not every argument: pgx takes the statement second, after
// the context, and looking at the rest reports a transaction handle or a
// subject id as a statement this scan could not read. Restricting to the
// position is what lets an unreadable one be a finding rather than noise.
func executedSQL(n ast.Node) (ast.Expr, bool) {
	call, isCall := n.(*ast.CallExpr)
	if !isCall || len(call.Args) < 2 {
		return nil, false
	}
	method, isMethod := call.Fun.(*ast.SelectorExpr)
	if !isMethod {
		return nil, false
	}
	switch method.Sel.Name {
	case "Exec", "Query", "QueryRow":
		return call.Args[1], true
	}
	return nil, false
}

// assignmentsAreReadable reports whether every SET clause in a statement
// rendered whole. One that did not hides the very thing this census counts.
func assignmentsAreReadable(statement string) bool {
	clauses := setClause.FindAllStringSubmatch(statement, -1)
	// NO CLAUSE AT ALL is unreadable, not readable. The caller has already
	// decided this statement assigns something; a SET the scan cannot find is
	// therefore a SET that the marker swallowed whole — `"UPDATE person " +
	// clearEverything` — and the token this pattern anchors on went with it.
	// Answering "readable" there let the loop run zero times and returned the
	// same true a fully rendered redaction gets, which is this census skipping
	// the one write it could not read.
	if len(clauses) == 0 {
		return false
	}
	for _, clause := range clauses {
		if !strings.Contains(clause[1], unresolved) {
			continue
		}
		// WHERE the marker sits decides it. A clause is a list of `column =
		// value` pairs, and this census counts the COLUMNS: a value the reader
		// could not render — a window computed by a helper, an interval built
		// from arguments — hides none of them, and refusing the whole statement
		// for one takes a table out of the census that is fully accounted for.
		//
		// On the left of the `=`, or with no `=` at all, it is another matter:
		// there the marker could be standing over an assignment, and what it
		// clears is exactly what nobody can see.
		for _, assignment := range splitSetClause(clause[1]) {
			column, _, assigns := strings.Cut(assignment, "=")
			if !assigns || strings.Contains(column, unresolved) {
				if strings.Contains(assignment, unresolved) {
					return false
				}
			}
		}
	}
	return true
}

// splitSetClause cuts a SET clause on the commas BETWEEN assignments — the ones
// outside any parentheses.
//
// A value carries commas of its own: `coalesce(a.x, ”)`, an ARRAY[…] of case
// arms, a function call with three arguments. Splitting on every comma would
// read each of those as an assignment of its own, and an assignment with no `=`
// in it is what this reader treats as a marker standing over a column.
func splitSetClause(clause string) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(clause); i++ {
		switch clause[i] {
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, clause[start:i])
				start = i + 1
			}
		}
	}
	return append(out, clause[start:])
}

// writesSomething matches the keyword of a statement that ASSIGNS, across any
// whitespace: `UPDATE\n  person SET …` is one write laid out over two lines,
// and a check for the literal "UPDATE " discards it before its target is ever
// read.
var writesSomething = regexp.MustCompile(`(?is)\b(?:update|merge\s+into)\s`)

// sqlish reports whether a rendered string is a statement this census has
// anything to say about. It asks for an assignment, so a WHERE fragment or a
// column list is correctly not one.
//
// Text carrying the unresolved marker counts as well, whatever else it holds:
// what a fragment this scan could not render turns out to be is exactly what it
// cannot know, and dropping it here is the silent pass the file refuses. The
// caller places it, and fails when it cannot.
func sqlish(text string) bool {
	if strings.Contains(text, unresolved) {
		return true
	}
	return writesSomething.MatchString(text) && strings.Contains(strings.ToUpper(text), "SET")
}

// write is one table a statement writes, with the text that writes it.
type write struct {
	// table is empty where the target could not be read — a name assembled at
	// run time. The caller FAILS on that rather than skipping it: an
	// unplaceable write is a finding, never a pass.
	table string
	text  string
}

// unreadableTarget matches a write whose TABLE NAME did not render — the shape
// `"UPDATE " + tableConst + " SET …"` leaves behind. writeTarget cannot match
// it, so without this the statement simply falls out of the census and says
// nothing, which is the silent pass this whole file exists to refuse.
var unreadableTarget = regexp.MustCompile(
	`(?is)\b(?:update|insert\s+into|merge\s+into)\s+` + regexp.QuoteMeta(unresolved))

// statementWrites splits one statement at its write targets.
//
// A COMPOUND STATEMENT IS THE TWO WRITES IT IS. The approval redaction is one
// CTE that updates approval and then workflow_run; read whole, the first
// target owns every SET clause in it — so workflow_run's assignments are
// invisible, and approval is credited with columns it never touches. Each
// segment runs from one target to the next, so its SET clauses are its own.
func statementWrites(statement string) []write {
	targets := writeTarget.FindAllStringSubmatchIndex(statement, -1)
	// `DO UPDATE SET` NAMES NO TABLE. An upsert's conflict clause reuses the
	// UPDATE keyword with the target left implicit, so the pattern reads `set`
	// as the table — and the segment before it, the one that IS the insert's
	// table, then ends at that false target and loses the very SET clause the
	// upsert redacts through. Dropped here rather than in the pattern because
	// RE2 has no lookahead, and because a keyword is not a table anywhere.
	targets = slices.DeleteFunc(targets, func(match []int) bool {
		return strings.EqualFold(statement[match[2]:match[3]], "set")
	})
	if len(targets) == 0 {
		if unreadableTarget.MatchString(statement) {
			return []write{{text: statement}}
		}
		return nil
	}
	out := make([]write, 0, len(targets))
	for i, match := range targets {
		end := len(statement)
		if i+1 < len(targets) {
			end = targets[i+1][0]
		}
		out = append(out, write{
			table: strings.ToLower(statement[match[2]:match[3]]),
			text:  statement[match[0]:end],
		})
	}
	// A target this scan could not read sits BETWEEN two it could, so the
	// segments above hide it. Reported as its own unplaceable write.
	if unreadableTarget.MatchString(statement) {
		out = append(out, write{text: statement})
	}
	return out
}

// erasureRedactedColumns names, per table, every column the Art. 17 cascade
// ASSIGNS. Only assignments count: a column named in a WHERE predicate or
// returned by a SELECT is not cleared by the statement that mentions it.
func erasureRedactedColumns(t *testing.T, columns map[string]map[string]string, statements []string) map[string]map[string]bool {
	t.Helper()
	out := map[string]map[string]bool{}
	for _, statement := range statements {
		for _, w := range statementWrites(statement) {
			known, registered := columns[w.table]
			if !registered {
				continue
			}
			// An ASSIGNING write, which writeTarget alone does not say: it
			// matches `INSERT INTO` too, because an insert's ON CONFLICT can
			// carry an UPDATE. One that does not carry a SET clause assigns
			// nothing, and registering it made an empty judged entry — which
			// then demanded a baseline for every text column of a table the
			// cascade may never redact at all.
			if !setClause.MatchString(w.text) {
				continue
			}
			// REGISTERED ON SIGHT of the clause, before any column matches. A
			// redaction that clears only non-text columns would otherwise leave
			// its table out of the judged set entirely — and a table nobody
			// judges is a table whose text columns go unasked about, which is
			// the whole defect this census is for.
			if out[w.table] == nil {
				out[w.table] = map[string]bool{}
			}
			for column := range known {
				if assignsColumn(w.text, column) {
					out[w.table][column] = true
				}
			}
		}
	}
	return out
}

// assignsColumn reports whether a statement's SET clauses assign one column,
// through the same reading dealforecastmovement_test.go uses: anchored on the
// whole identifier, so `body` is not found inside `html_body`, and the
// parenthesised multi-column form counts.
func assignsColumn(statement, column string) bool {
	pattern := regexp.MustCompile(fmt.Sprintf(assignmentFmt, regexp.QuoteMeta(column)))
	for _, clause := range setClause.FindAllStringSubmatch(statement, -1) {
		if pattern.MatchString(clause[1]) {
			return true
		}
	}
	return false
}

// redactedTables are the registered PII tables the cascade REDACTS — the ones
// this census has something to say about. A table the cascade only deletes
// from loses the whole row, so there is no column set to account for.
func redactedTables(redacted map[string]map[string]bool) []string {
	out := make([]string, 0, len(redacted))
	for table := range redacted {
		if _, registered := piiTables[table]; registered {
			out = append(out, table)
		}
	}
	sort.Strings(out)
	return out
}

// The reader that places a write, driven over the shapes the cascade actually
// contains and the two it must not wave through. A census is only as good as
// the statement it read, and both failures here are silent by construction.
func TestAWriteIsPlacedByItsOwnTargetOrNotAtAll(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		sql  string
		want []write
	}{
		{
			name: "a plain update is one write",
			sql:  "UPDATE person SET full_name = '' WHERE id = $1",
			want: []write{{table: "person", text: "UPDATE person SET full_name = '' WHERE id = $1"}},
		}, {
			// The approval redaction's shape. Read whole, the first target
			// owns every SET clause: workflow_run's assignments become
			// invisible and approval is credited with columns it never
			// touches, which is wrong in both directions at once.
			name: "a compound statement is the two writes it is",
			sql:  "WITH x AS (UPDATE approval SET summary = '' RETURNING id) UPDATE workflow_run SET detail = '{}'",
			want: []write{
				{table: "approval", text: "UPDATE approval SET summary = '' RETURNING id) "},
				{table: "workflow_run", text: "UPDATE workflow_run SET detail = '{}'"},
			},
		}, {
			// A table name assembled at run time. writeTarget cannot match it,
			// so without the unreadable arm the statement falls out of the
			// census entirely and reports nothing — while the floor on how
			// many tables were judged still passes, because the OTHER writes
			// are all still there.
			name: "an unreadable target is a write with no table",
			sql:  "UPDATE " + unresolved + " SET body = ''",
			want: []write{{table: "", text: "UPDATE " + unresolved + " SET body = ''"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := statementWrites(tc.sql)
			if len(got) != len(tc.want) {
				t.Fatalf("placed %d write(s), want %d: %+v", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i].table != tc.want[i].table {
					t.Errorf("write %d names table %q, want %q", i, got[i].table, tc.want[i].table)
				}
				if !strings.Contains(got[i].text, "SET") {
					t.Errorf("write %d carries no SET clause of its own: %q", i, got[i].text)
				}
			}
		})
	}
	// And the segments really do separate the assignments, which is the whole
	// point of splitting: approval's segment must not answer for the column
	// only workflow_run assigns.
	compound := "WITH x AS (UPDATE approval SET summary = '' RETURNING id) UPDATE workflow_run SET detail = '{}'"
	writes := statementWrites(compound)
	if len(writes) != 2 {
		t.Fatalf("the compound statement placed %d writes", len(writes))
	}
	if assignsColumn(writes[0].text, "detail") {
		t.Error("approval's segment answers for a column only workflow_run assigns — the split did not separate them")
	}
	if !assignsColumn(writes[1].text, "detail") {
		t.Error("workflow_run's segment lost the assignment it is the segment for")
	}
}

func TestTheArt17RedactionAccountsForEveryTextColumn(t *testing.T) {
	t.Parallel()
	columns := catalogTextColumns(t)
	consts := sqlConstants(t, "internal/modules/privacy", "internal/platform/database/storekit")
	statements := cascadeStatements(t, consts)
	for _, statement := range statements {
		for _, w := range statementWrites(statement) {
			// The TABLE could not be read. Nothing places this write, so
			// nothing can say what it clears — and skipping it is the silent
			// pass this file refuses, whether or not the table turns out to be
			// registered.
			if w.table == "" {
				t.Fatalf("the Art. 17 cascade writes a table this census cannot name — the target is assembled "+
					"from something sqlConstants does not read, so the write is unplaceable and every column of "+
					"whatever it writes would go unjudged:\n\t%s", collapsedSQL(statement))
			}
			if _, registered := piiTables[w.table]; !registered {
				continue
			}
			if assignmentsAreReadable(w.text) {
				continue
			}
			t.Fatalf("the Art. 17 redaction of %s builds its SET clause from something this census cannot render, "+
				"so what it clears is unknown and every column of the table would be reported as unaccounted for. "+
				"Teach sqlConstants that source rather than letting the scan judge the half it can read:\n\t%s",
				w.table, collapsedSQL(w.text))
		}
	}
	redacted := erasureRedactedColumns(t, columns, statements)
	tables := redactedTables(redacted)
	// The floor. If the SQL reading rots — a quoting form it cannot follow, a
	// helper renamed — every table falls out of `redacted` and this census
	// reports a clean pass over nothing, which is the failure the file above
	// it is entirely about.
	if len(tables) < 5 {
		t.Fatalf("the cascade scan placed redactions on %d registered PII tables (%v), want at least five — "+
			"the SQL reading has rotted and this census is now judging almost nothing", len(tables), tables)
	}

	for _, table := range tables {
		baseline := map[string]bool{}
		for _, column := range erasureColumnBaseline[table] {
			baseline[column] = true
		}
		known := columns[table]
		for column := range known {
			if redacted[table][column] || baseline[column] {
				continue
			}
			t.Errorf("%s.%s is a %s column on a PII table the Art. 17 redaction rewrites, and the redaction "+
				"neither clears it nor accounts for it.\n"+
				"\tIf it can hold what a subject wrote or what was written about them, clear it in the redaction.\n"+
				"\tIf it cannot, add it to erasureColumnBaseline[%q] — which fixes the column set rather than "+
				"claiming the column is safe.", table, column, known[column], table)
		}
		// And the baseline may not rot. An entry naming a column that has
		// since been dropped, or one the redaction has since started clearing,
		// stands for a decision nobody is making any more — and a list that
		// accumulates those stops being readable as the set it is meant to fix.
		for _, column := range erasureColumnBaseline[table] {
			if known[column] == "" {
				t.Errorf("erasureColumnBaseline[%q] names %q, which is no longer a text or jsonb column on "+
					"that table — drop the entry rather than leaving a stale one standing", table, column)
			}
			if redacted[table][column] {
				t.Errorf("erasureColumnBaseline[%q] names %q, which the redaction now clears — "+
					"drop the entry, so the list goes on meaning \"not cleared\"", table, column)
			}
		}
	}

	// A baseline for a table this census does not judge is a line nobody reads
	// and nothing checks.
	judged := map[string]bool{}
	for _, table := range tables {
		judged[table] = true
	}
	for table := range erasureColumnBaseline {
		if !judged[table] {
			t.Errorf("erasureColumnBaseline names %q, which the Art. 17 cascade does not redact — "+
				"either the redaction moved, or the entry was never about a table this gate judges", table)
		}
	}
}
