// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// "Which activities belong to this account" is computed in two places, and they
// must agree.
//
// It has to be spelled twice: the timeline list, the account view and the
// roll-up ask it from `activities`, the context walk asks it from `search`, and
// a module never imports a sibling (ADR-0054). The tree already accepts that
// trade for the project-scope predicate, with a comment on each half saying
// "change one, change both".
//
// A comment is not a mechanism. This is: the two spellings of the ARMS — the
// account an activity is filed against, the account its deal belongs to, and
// the employer of the contact it is about — must be the same text. An arm that
// gains a condition on one side and not the other is what makes an account's
// timeline and its context walk disagree about which meetings it had, and the
// disagreement is silent on both sides.
//
// The arms rather than the shapes, deliberately: the two shapes differ for a
// reason that will not go away (a predicate takes the account as a bind, a
// producer holds the activity and needs the accounts), while the arms are the
// answer itself.

import (
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
)

// accountReachFiles are the two files that spell the walk.
var accountReachFiles = []string{
	"internal/modules/activities/companyscope.go",
	"internal/modules/search/graphcompanyreach.go",
}

// accountReachArms are the constants each of those files must spell, and spell
// identically. EVERY constant, not just the first: the arms are split across
// two of them because one is shared with a producer that stops short of the
// last arm, and holding only the shared half equal would leave the arm that
// split them free to drift — which is this gate's whole subject.
var accountReachArms = []string{"companyArms", "participantEmployerArm"}

func TestTheAccountReachWalkIsOneAnswer(t *testing.T) {
	t.Parallel()
	for _, arm := range accountReachArms {
		texts := map[string]string{}
		for _, file := range accountReachFiles {
			text, found := constText(t, file, arm)
			if !found {
				t.Fatalf("%s no longer declares %s — the account-reach walk was renamed or moved, and "+
					"this gate stopped comparing anything rather than failing", file, arm)
			}
			texts[file] = text
		}
		first := accountReachFiles[0]
		for _, other := range accountReachFiles[1:] {
			if normalizeSQL(texts[first]) != normalizeSQL(texts[other]) {
				t.Errorf("the %s arm differs between %s and %s — one of them has gained or "+
					"lost a condition, so the account's timeline and its context walk no longer agree "+
					"about which activities belong to it:\n  %s: %s\n  %s: %s",
					arm, first, other,
					first, normalizeSQL(texts[first]),
					other, normalizeSQL(texts[other]))
			}
		}
		// Both halves must actually SAY something. Two empty strings compare
		// equal, and a gate that passes over nothing is the failure mode this
		// whole file is about.
		for file, text := range texts {
			if strings.TrimSpace(text) == "" {
				t.Errorf("%s spells %s as an empty string, so the comparison above proved nothing", file, arm)
			}
		}
	}
}

// Every SQL function at head that reads employments carries the walk's rendered
// currency test, and its bound wherever it dates mail. last_activity_of_company
// and last_activity_reach are the two today; the corpus is every function.
func TestEverySQLEmploymentReachCountsLikeTheWalk(t *testing.T) {
	t.Parallel()
	dir := filepath.Join("migrations", "core")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	var migrations []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".up.sql") {
			migrations = append(migrations, entry.Name())
		}
	}
	// Filenames lead with the unix second they were written, so lexical order is apply order.
	slices.Sort(migrations)
	texts := make([]string, 0, len(migrations))
	for _, name := range migrations {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		texts = append(texts, string(raw))
	}
	bodies, err := functionBodiesAtHead(texts)
	if err != nil {
		t.Fatal(err)
	}
	judged, findings := employmentFindings(bodies)
	catalog, err := os.ReadFile(filepath.Join("migrations", "testdata", "head_catalog.txt"))
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	atHead := map[string]bool{}
	for _, m := range coreCatalogFunction.FindAllStringSubmatch(string(catalog), -1) {
		atHead[m[1]] = true
	}
	for name := range atHead {
		if _, replayed := bodies[name]; !replayed {
			t.Errorf("the head catalog holds function %s and the migration replay does not, so the census reads short", name)
		}
	}
	for name := range bodies {
		if !atHead[name] {
			t.Errorf("the migration replay holds function %s and the head catalog does not", name)
		}
	}
	for _, known := range []string{"last_activity_of_company", "last_activity_reach"} {
		if judged[known] == 0 {
			t.Errorf("the census found no employment arm in %s, so it reads a smaller tree than exists", known)
		}
	}
	for _, finding := range findings {
		t.Error(finding)
	}
}

func TestTheClockBoundCensusSeesEveryEmploymentArm(t *testing.T) {
	t.Parallel()
	bound := employment.InPlaceAtSQL("r", "a.occurred_at")
	current := employment.IsCurrentSQL("r.ended_at")
	cases := []struct {
		name       string
		body       string
		arms       int
		unbounded  []string
		notCurrent []string
	}{
		{"no employment arm", "SELECT 1 FROM activity_link l WHERE l.company_id = cid", 0, nil, nil},
		{"a trigger's row, not a relationship read", "IF NEW.kind = 'employment' OR OLD.kind = 'employment' THEN", 0, nil, nil},
		{
			"bounded and current",
			"SELECT a.occurred_at FROM relationship r JOIN activity a ON true WHERE r.kind = 'employment' AND " +
				current + " AND " + bound, 1,
			nil, nil,
		},
		{
			"second arm under another alias, unbounded and bare",
			"SELECT 1 FROM relationship r WHERE r.kind = 'employment' AND " + current + " AND " + bound +
				"\n UNION ALL\n SELECT a.occurred_at FROM relationship emp JOIN activity a ON true" +
				" WHERE emp.kind='employment' AND " + bareEndedAtTest("emp"), 2,
			[]string{"emp"},
			[]string{"emp"},
		},
		{
			"bound only in a comment",
			"SELECT a.occurred_at FROM relationship r WHERE r . kind\n = 'employment' AND " + current + " -- AND " + bound, 1,
			[]string{"r"},
			nil,
		},
		{
			"bound only in a block comment",
			"SELECT a.occurred_at FROM relationship r WHERE r.kind = 'employment' AND " + current +
				" /* AND " + bound + " */", 1,
			[]string{"r"},
			nil,
		},
		{
			"a bare null test of the end date in place of the predicate",
			"SELECT 1 FROM relationship r WHERE r.kind = 'employment'\n AND " + bareEndedAtTest("r") + " AND " + bound, 1,
			nil,
			[]string{"r"},
		},
		{
			"predicate beside a bare null test of the end date",
			"SELECT 1 FROM relationship r WHERE r.kind = 'employment' AND " + current +
				"\n AND r . ended_at  is null AND " + bound, 1,
			nil,
			[]string{"r"},
		},
		{
			"predicate only in a comment",
			"SELECT 1 FROM relationship r WHERE r.kind = 'employment'\n -- AND " + current + "\n AND " + bound, 1,
			nil,
			[]string{"r"},
		},
		{
			"reach branch without the predicate",
			"SELECT 'contact', named FROM unnest(contact_ids) named\n" +
				" UNION SELECT 'company', r.company_id FROM relationship r WHERE r.contact_id = ANY (contact_ids)" +
				" AND r.kind = 'employment' AND " + bareEndedAtTest("r") + " AND r.archived_at IS NULL", 1,
			nil,
			[]string{"r"},
		},
	}
	for _, tc := range cases {
		arms := employmentArms(tc.body)
		var unbounded, notCurrent []string
		for _, arm := range arms {
			if !arm.bounded {
				unbounded = append(unbounded, arm.alias)
			}
			if !arm.current {
				notCurrent = append(notCurrent, arm.alias)
			}
		}
		if len(arms) != tc.arms || strings.Join(unbounded, ",") != strings.Join(tc.unbounded, ",") ||
			strings.Join(notCurrent, ",") != strings.Join(tc.notCurrent, ",") {
			t.Errorf("%s: found %d arms, unbounded %v, not current %v; want %d, %v, %v",
				tc.name, len(arms), unbounded, notCurrent, tc.arms, tc.unbounded, tc.notCurrent)
		}
	}
}

// The corpus is replayed from the migrations, so a function added later is
// judged. A dropped function is not, and a renamed one is judged by its new name.
func TestTheClockBoundCensusReadsEveryFunctionAtHead(t *testing.T) {
	t.Parallel()
	current := employment.IsCurrentSQL("r.ended_at")
	planted := []string{
		"CREATE FUNCTION standard(lang text) RETURNS int LANGUAGE sql IMMUTABLE RETURN 1;\n" +
			"CREATE FUNCTION kept() RETURNS int LANGUAGE sql AS $$ SELECT 1 FROM relationship r WHERE r.kind = 'employment' AND " +
			current + " $$;\n-- CREATE FUNCTION mentioned() is only a comment\n" +
			"CREATE FUNCTION retired() RETURNS int LANGUAGE sql AS $$ SELECT 1 FROM relationship r" +
			" WHERE r.kind = 'employment' AND " + bareEndedAtTest("r") + " $$;",
		"DROP FUNCTION IF EXISTS retired();\n" +
			"CREATE OR REPLACE FUNCTION public.added_later(cid uuid) RETURNS int LANGUAGE plpgsql AS $body$\n" +
			"BEGIN PERFORM 1 FROM relationship AS emp WHERE emp.kind = 'employment' AND " + bareEndedAtTest("emp") +
			"; RETURN 1; END $body$;\nALTER FUNCTION kept() RENAME TO renamed;",
	}
	bodies, err := functionBodiesAtHead(planted)
	if err != nil {
		t.Fatal(err)
	}
	judged, findings := employmentFindings(bodies)
	if judged["renamed"] != 1 || judged["added_later"] != 1 || len(judged) != 2 {
		t.Errorf("judged %v; want one arm each in renamed and added_later, and nothing else", judged)
	}
	if len(findings) != 1 || !strings.HasPrefix(findings[0], "added_later's employment arm on emp ") {
		t.Errorf("findings %q; want one, for added_later's bare arm", findings)
	}
	if _, err := functionBodiesAtHead([]string{"CREATE FUNCTION quoted() RETURNS int LANGUAGE sql AS 'SELECT 1';"}); err == nil {
		t.Error("a function body the census cannot read was skipped rather than refused")
	}
}

var (
	sqlBlockComment     = regexp.MustCompile(`(?s)/\*.*?\*/`)
	sqlUnion            = regexp.MustCompile(`(?i)\bUNION(?:\s+ALL)?\b`)
	employmentKindArm   = regexp.MustCompile(`(?i)\b(\w+)\s*\.\s*kind\s*=\s*'employment'`)
	relationshipAlias   = regexp.MustCompile(`(?i)\brelationship\s+(?:AS\s+)?(\w+)`)
	occurredAtRead      = regexp.MustCompile(`(?i)\b(\w+)\s*\.\s*occurred_at\b`)
	sqlDollarQuote      = regexp.MustCompile(`\$(\w*)\$`)
	sqlStandardReturn   = regexp.MustCompile(`(?i)\bRETURN\b`)
	coreCatalogFunction = regexp.MustCompile(`(?m)^public\.(\w+)\(.*\) secdef=`)
	sqlFunctionDDL      = regexp.MustCompile(`(?i)\bCREATE\s+(?:OR\s+REPLACE\s+)?FUNCTION\s+(?:public\.)?(\w+)` +
		`|\bDROP\s+FUNCTION\s+(?:IF\s+EXISTS\s+)?(?:public\.)?(\w+)` +
		`|\bALTER\s+FUNCTION\s+(?:public\.)?(\w+)\s*\([^)]*\)\s*RENAME\s+TO\s+(\w+)`)
)

// functionBodiesAtHead replays the function DDL of migrations, in apply order,
// and answers each function's body at head. Comments are dropped first.
func functionBodiesAtHead(migrations []string) (map[string]string, error) {
	bodies := map[string]string{}
	for _, migration := range migrations {
		sql := stripSQLComments(sqlBlockComment.ReplaceAllString(migration, " "))
		for at := 0; ; {
			m := sqlFunctionDDL.FindStringSubmatchIndex(sql[at:])
			if m == nil {
				break
			}
			name := func(group int) string { return sql[at+m[2*group] : at+m[2*group+1]] }
			end := at + m[1]
			switch {
			case m[2] >= 0:
				body, consumed, err := migratedFunctionBody(sql[end:])
				if err != nil {
					return nil, fmt.Errorf("function %s: %w", name(1), err)
				}
				bodies[name(1)] = body
				end += consumed
			case m[4] >= 0:
				delete(bodies, name(2))
			default:
				bodies[name(4)] = bodies[name(3)]
				delete(bodies, name(3))
			}
			at = end
		}
	}
	return bodies, nil
}

// migratedFunctionBody answers the body after a function header and how far it ran:
// dollar-quoted, or a SQL-standard RETURN. Any other form is refused, because
// reading past it would judge the next function's text under this one's name.
func migratedFunctionBody(rest string) (string, int, error) {
	statementEnd := strings.Index(rest, ";")
	if statementEnd < 0 {
		statementEnd = len(rest)
	}
	open := sqlDollarQuote.FindStringSubmatchIndex(rest)
	if open == nil || open[0] > statementEnd {
		if standard := sqlStandardReturn.FindStringIndex(rest[:statementEnd]); standard != nil {
			return rest[standard[0]:statementEnd], statementEnd, nil
		}
		return "", 0, fmt.Errorf("its body is neither dollar-quoted nor a RETURN, so the census cannot read it")
	}
	tag := rest[open[0]:open[1]]
	closing := strings.Index(rest[open[1]:], tag)
	if closing < 0 {
		return "", 0, fmt.Errorf("its %s body never closes", tag)
	}
	return rest[open[1] : open[1]+closing], open[1] + closing + len(tag), nil
}

// employmentFindings judges every employment arm of bodies, answering the arm
// count per function and one message per arm that counts employment its own way.
func employmentFindings(bodies map[string]string) (map[string]int, []string) {
	judged := map[string]int{}
	var findings []string
	names := make([]string, 0, len(bodies))
	for name := range bodies {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, fn := range names {
		for _, arm := range employmentArms(bodies[fn]) {
			judged[fn]++
			if !arm.current {
				findings = append(findings, fmt.Sprintf("%s's employment arm on %s does not carry %q, or also "+
					"tests ended_at IS NULL on its own, so a contact serving notice stops counting for the "+
					"employer they still work for. Replace the function in a new migration with the predicate's text.",
					fn, arm.alias, employment.IsCurrentSQL(arm.alias+".ended_at")))
			}
			if !arm.bounded {
				findings = append(findings, fmt.Sprintf("%s's employment arm on %s does not carry %q, so mail "+
					"from a contact's previous job counts for the employer they have now. Replace the function "+
					"in a new migration with the bound's text.",
					fn, arm.alias, employment.InPlaceAtSQL(arm.alias, arm.mailAt)))
			}
		}
	}
	return judged, findings
}

// employmentArm is one employment arm of a function body, under its alias.
// bounded holds when the arm dates no mail, or bounds the mail it dates.
type employmentArm struct {
	alias   string
	mailAt  string
	bounded bool
	current bool
}

// employmentArms reads each employment arm of a function body: a relationship
// read under an alias and scoped to employments. Comments are dropped first, so
// a predicate that is only mentioned does not count.
func employmentArms(body string) []employmentArm {
	code := stripSQLComments(sqlBlockComment.ReplaceAllString(body, " "))
	var arms []employmentArm
	for _, part := range sqlUnion.Split(code, -1) {
		flat := normalizeSQL(part)
		aliases := map[string]bool{}
		for _, m := range relationshipAlias.FindAllStringSubmatch(flat, -1) {
			aliases[strings.ToLower(m[1])] = true
		}
		for _, m := range employmentKindArm.FindAllStringSubmatch(flat, -1) {
			alias := m[1]
			if !aliases[strings.ToLower(alias)] {
				continue
			}
			arm := employmentArm{alias: alias, bounded: true}
			if read := occurredAtRead.FindStringSubmatch(flat); read != nil {
				arm.mailAt = read[1] + ".occurred_at"
				arm.bounded = strings.Contains(flat, normalizeSQL(employment.InPlaceAtSQL(alias, arm.mailAt)))
			}
			current := normalizeSQL(employment.IsCurrentSQL(alias + ".ended_at"))
			bareEnd := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(alias) + `\s*\.\s*ended_at\s+IS\s+NULL\b`)
			arm.current = strings.Contains(flat, current) && !bareEnd.MatchString(strings.ReplaceAll(flat, current, " "))
			arms = append(arms, arm)
		}
	}
	return arms
}

// bareEndedAtTest is the defect the census plants, declared apart so the
// employment currency census does not read a planted body as a real query.
func bareEndedAtTest(alias string) string {
	return alias + ".ended_at IS NULL"
}

// constText answers the string literal a named package-level constant holds.
func constText(t *testing.T, file, name string) (string, bool) {
	t.Helper()
	fset := gatekit.SourceFileSet()
	parsed, err := gatekit.ParseFile(file, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", file, err)
	}
	for _, decl := range parsed.Decls {
		gen, isGen := decl.(*ast.GenDecl)
		// VAR as well as CONST: an arm that calls the shared employment
		// predicate cannot be a constant, and reading only constants would
		// leave this gate comparing nothing on the day the arms adopted it.
		if !isGen || (gen.Tok != token.CONST && gen.Tok != token.VAR) {
			continue
		}
		for _, spec := range gen.Specs {
			value, isValue := spec.(*ast.ValueSpec)
			if !isValue {
				continue
			}
			// A spec whose names and values are not one-to-one is not read by
			// POSITION — a `const a, b = f()` or an iota run pairs a name with
			// an expression that is not its own, and reading it by index is how
			// a refactor quietly hands this gate the wrong text to compare.
			if len(value.Names) != len(value.Values) {
				continue
			}
			for i, ident := range value.Names {
				if ident.Name != name {
					continue
				}
				// The expression AS WRITTEN, not the string it resolves to.
				//
				// An arm that calls the shared employment predicate is a
				// concatenation rather than a literal, and a reader that folded
				// it would have to put something in the call's place — every
				// fold available renders two DIFFERENT calls as the same
				// placeholder, so the gate would stop being able to tell
				// `IsCurrentSQL("r.ended_at")` from `IsCurrentSQL("emp.ended_at")`.
				// That distinction is this gate's whole subject.
				//
				// Source text keeps it: two declarations that are identical
				// character for character are identical however they are built,
				// which is the claim being made.
				var written strings.Builder
				if err := printer.Fprint(&written, fset, value.Values[i]); err != nil {
					t.Fatalf("printing %s in %s: %v", name, file, err)
				}
				return written.String(), true
			}
		}
	}
	return "", false
}

// normalizeSQL collapses the whitespace two files indent differently, so the
// comparison is about the SQL rather than about where each copy sits on the
// page.
func normalizeSQL(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
