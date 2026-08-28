// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind claim H1

package gates

// people.EmploymentIsCurrentSQL calls itself "the ONE spelling of 'this job is
// still theirs', and the only definition of a current employment in this
// product". That was a claim with nothing holding it, and it was false eleven
// times over.
//
// Eight statements asked whether an employment was current with a bare
// `ended_at IS NULL`, which is exactly the defect the helper's own comment
// describes: somebody serving three months' notice still works there, and
// reading the column's mere presence as "gone" took them off their employer's
// contact list the day their notice was filed. Three more hand-spelled the
// correct form, and one of those compared against a Go clock instead of the
// database's, in the same statement as a half that used the database's — so a
// single query asked its two questions on two different days whenever the
// server and Postgres disagreed about the date.
//
// This is what holds the claim now. It judges STATEMENTS that ask about an
// employment: a SQL literal naming `kind = 'employment'` (or joining the
// relationship table under an employment predicate) must not decide currency
// by testing `ended_at` itself. It must call the helper.
//
// What it deliberately does NOT judge: a relationship of another kind. A
// `deal_stakeholder` or a `partner_of` edge also carries `ended_at`, and
// whether a future end date leaves one of those current is a different
// question that nobody has answered yet. Widening this gate to cover them would
// be asserting an answer rather than holding one.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// blockedByTheModuleDAG ratifies the statements that cannot adopt the helper
// TODAY, each with the reason it cannot — which is the same reason in every
// case and is architectural, not a matter of somebody not getting round to it.
//
// EmploymentIsCurrentSQL lives in modules/people, and a module never imports a
// sibling (ADR-0054 §3). compose may reach it and does; people's own files
// reach it directly; three sibling modules cannot — FIVE statements across
// activities, projects and signals, since resolver.go carries two — and the
// predicate would have to move tier before they could. That is an architecture decision with an
// owner, so it is an issue rather than a change smuggled into this one — margince/margince#2360.
//
// Each entry is a FILE and not the whole module, so a new statement in one of
// these packages is still a finding — the ratification covers the sites that
// exist, not the topic.
var blockedByTheModuleDAG = gatekit.Waive(map[string]string{
	"internal/modules/activities/orgscope.go": "activities cannot import people (ADR-0054 §3); the predicate must move tier first",
	"internal/modules/projects/surface.go":    "projects cannot import people (ADR-0054 §3); the predicate must move tier first",
	"internal/modules/signals/resolver.go":    "signals cannot import people (ADR-0054 §3); the predicate must move tier first",
	"internal/modules/signals/warmroom.go":    "signals cannot import people (ADR-0054 §3); the predicate must move tier first",
	"internal/modules/consent/confirmcard.go": "consent cannot import people (ADR-0054 §3); the predicate must move tier first. The copy is the helper's own date comparison rather than a null check, so a person serving notice still sees their employer on their own confirm card",
})

const (
	employmentHelper = "EmploymentIsCurrentSQL"
	primaryHelper    = "CurrentPrimaryEmploymentSQL"
	employmentIssue  = "five statements in three sibling modules are ratified separately: a module may not import people (ADR-0054 §3), so the predicate has to move tier before they can adopt it; see issue 2360"
)

// employmentKind matches a statement that has scoped itself to employments.
//
// `'employment'` ANYWHERE in an IN list, not only first. The pattern used to
// anchor on the opening paren, so `kind IN ('deal_stakeholder', 'employment')`
// was not an employment statement as far as the census was concerned — and a
// hand-written currency test in one would have passed. Ordering inside an IN
// list is the author's whim, which is a poor thing for a gate to depend on.
//
// One level of nesting is allowed inside the list, because `[^)]*` stopped at
// the FIRST close-paren and an item like `(SELECT …)` ended the match before
// the literal. One level and not arbitrary depth: RE2 has no recursion, the
// deeper form does not occur here, and a bounded pattern that says what it
// does is better than an unbounded claim.
var employmentKind = regexp.MustCompile(`kind\s*=\s*'employment'|kind\s+IN\s*\((?:[^()]|\([^()]*\))*'employment'`)

// endedAtCurrency matches a hand-written currency test on ended_at — the bare
// null check that loses a notice period, and the long form that gets the
// semantics right but is still a second copy.
//
// `IS NOT NULL` is matched too. The negation is the same decision made
// backwards, and leaving it out let a statement ask "has this person left?" by
// hand while its sibling half asked "are they still here?" through the helper
// — one query, two definitions, and they disagreed on the day a notice period
// ended.
var endedAtCurrency = regexp.MustCompile(`ended_at\s+IS\s+(NOT\s+)?NULL|ended_at\s*(>|<|>=|<=)`)

// employmentCurrencyOwner is where the definition lives. Its own statements are
// the definition rather than a copy of it.
const employmentCurrencyOwner = "internal/modules/people/employmentcurrency.go"

func TestEveryEmploymentCurrencyTestUsesTheOneDefinition(t *testing.T) {
	t.Parallel()
	// A ratification that stops matching is a ratification for a site that has
	// moved or been fixed, and leaving it in place quietly re-exempts whatever
	// takes its name next.
	defer blockedByTheModuleDAG.AssertAllMatched(t)

	fset := token.NewFileSet()
	var findings []string
	files := handWrittenGoSources(t)
	judged := 0
	for _, path := range files {
		if filepath.ToSlash(path) == employmentCurrencyOwner {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		scope := helperScope{
			qualifier: importAliasOf(file, "github.com/margince/margince/backend/internal/modules/people"),
			inside:    file.Name != nil && file.Name.Name == "people",
			names:     map[string]bool{employmentHelper: true, primaryHelper: true},
		}
		for _, decl := range file.Decls {
			if plantedProbe(path, decl) {
				continue
			}
			for _, sql := range employmentStatements(decl, scope) {
				judged++
				if !endedAtCurrency.MatchString(sql) {
					continue
				}
				if blockedByTheModuleDAG.Waived(t, filepath.ToSlash(path)) {
					continue
				}
				findings = append(findings, fmt.Sprintf("%s: %s", path, firstEmploymentLine(sql)))
			}
		}
	}
	// A census that judged nothing certifies nothing. The floor is far below the
	// real count so it catches a broken walk, not a changing tree.
	if judged < 10 {
		t.Fatalf("only %d employment statement(s) were judged, so this census covered almost nothing", judged)
	}
	if len(findings) == 0 {
		return
	}
	t.Errorf("these statements decide whether an employment is current by testing ended_at themselves:\n  %s\n\n"+
		"people.%s is the one definition, and it is a DATE comparison: somebody serving three months' "+
		"notice still works there, and reading the column's presence as \"gone\" takes them off their "+
		"employer's contact list the day their notice is filed — with no way back, because ended_at "+
		"cannot be cleared through the API. Call the helper. (%s)",
		strings.Join(findings, "\n  "), employmentHelper, employmentIssue)
}

// employmentStatements returns the SQL statements in a declaration that have
// scoped themselves to employments.
//
// A statement, not a literal. A query that calls the helper is written as
//
//	`… WHERE r.kind = 'employment' AND ` + people.EmploymentIsCurrentSQL("r.ended_at") + ` AND …`
//
// which the parser gives as three separate nodes, so judging each *ast.BasicLit
// on its own splits the question in half: the piece naming the employment kind
// no longer contains the `ended_at` test, and the gate passes over it.
//
// That is not a theoretical gap — it is the shape EVERY site adopted in this
// change now has, so the gate could not have caught a regression at any of
// them. Verified by reintroducing a bare `ended_at IS NULL` as a concatenated
// fragment: the gate reported ok.
//
// A concatenation is therefore flattened first. A call contributes its function
// NAME, which is what makes the helper exemption real rather than dead: the
// helper is called from Go, so its name never appears inside a SQL literal, and
// an exemption looking for it there could never fire.
//
// Per DECLARATION and not per file: a file may hold one query about employments
// and another about deal stakeholders, and asking whether both shapes appear
// somewhere in the same file reports a pairing nobody wrote.
func employmentStatements(decl ast.Decl, people helperScope) []string {
	var out []string
	seen := map[ast.Node]bool{}
	ast.Inspect(decl, func(n ast.Node) bool {
		if seen[n] {
			return false
		}
		text, ok := flattenSQL(n, seen, people)
		if !ok || !employmentKind.MatchString(text) {
			return true
		}
		out = append(out, text)
		return true
	})
	return out
}

// firstEmploymentLine returns the line of the statement that names the
// employment kind, so the report points at the statement rather than dumping
// it.
func firstEmploymentLine(sql string) string {
	for _, line := range strings.Split(sql, "\n") {
		if employmentKind.MatchString(line) {
			return strings.TrimSpace(line)
		}
	}
	return strings.TrimSpace(strings.Split(sql, "\n")[0])
}

// employmentProbe is one planted source file and the answer the gate must give
// for it.
type employmentProbe struct {
	name  string
	fires bool
	// mode picks the file the probe is parsed as, because the gate's answers
	// depend on it and a probe that guesses wrong asks a different question
	// than the tree does:
	//
	//   ""         package probe, importing people — an ordinary caller
	//   "people"   package people — the one place a bare call is the helper's
	//   "noimport" package probe, NOT importing people — most of the tree, and
	//              where a bare helper name is somebody else's function
	mode string
	src  string
}

// The census above is a census of ZERO: it passes identically over a clean tree
// and over a detector that has stopped detecting. These read the detector
// directly, which is the half that makes the census mean anything.
//
// Every case here exists because the gate was once green over it.
//
//gate:probe
var employmentProbes = []employmentProbe{
	{"the bare form that shipped, one literal", true, "", `
func read() string {
	return ` + "`" + `SELECT 1 FROM relationship r WHERE r.kind = 'employment' AND r.ended_at IS NULL` + "`" + `
}`},
	{"the same, split across a concatenation", true, "", `
func read() string {
	return ` + "`" + `SELECT 1 FROM relationship r WHERE r.kind = 'employment' AND ` + "`" + ` + ` + "`" + `r.ended_at IS NULL` + "`" + `
}`},
	{"the same, inside a formatter's argument", true, "", `
func read() string {
	return fmt.Sprintf(` + "`" + `SELECT 1 FROM relationship r WHERE r.kind = 'employment' AND r.ended_at IS NULL AND (%s)` + "`" + `, scope)
}`},
	{"the negation, which is the same decision backwards", true, "", `
func read() string {
	return ` + "`" + `SELECT 1 FROM relationship r WHERE r.kind = 'employment' AND r.ended_at IS NOT NULL` + "`" + `
}`},
	{"the helper AND a hand-written test beside it", true, "", `
func read() string {
	return ` + "`" + `SELECT 1 FROM relationship r WHERE r.kind = 'employment' AND ` + "`" + ` + people.EmploymentIsCurrentSQL("r.ended_at") + ` + "`" + ` AND r.ended_at IS NOT NULL` + "`" + `
}`},
	// The name alone is not the helper. markSeen claims a helper call's whole
	// subtree, so a LOOKALIKE would have had its arguments hidden and could
	// have carried a hand-written test through inside them.
	{"a lookalike helper from another package", true, "", `
func read() string {
	return ` + "`" + `SELECT 1 FROM relationship r WHERE r.kind = 'employment' AND ` + "`" + ` + other.EmploymentIsCurrentSQL("r.ended_at IS NULL") + ` + "`" + ` AND 1=1` + "`" + `
}`},

	{"the real helper, qualified", false, "", `
func read() string {
	return ` + "`" + `SELECT 1 FROM relationship r WHERE r.kind = 'employment' AND ` + "`" + ` + people.EmploymentIsCurrentSQL("r.ended_at") + ` + "`" + ` AND r.archived_at IS NULL` + "`" + `
}`},
	{"the real helper, unqualified inside people", false, "people", `
func read() string {
	return ` + "`" + `SELECT 1 FROM relationship r WHERE r.kind = 'employment' AND ` + "`" + ` + EmploymentIsCurrentSQL("r.ended_at") + ` + "`" + ` AND r.archived_at IS NULL` + "`" + `
}`},
	// A bare call outside people names something else entirely.
	{"an unqualified call outside people", true, "", `
func read() string {
	return ` + "`" + `SELECT 1 FROM relationship r WHERE r.kind = 'employment' AND ` + "`" + ` + EmploymentIsCurrentSQL("r.ended_at IS NULL") + ` + "`" + ` AND 1=1` + "`" + `
}`},
	// Another relationship kind is a different question, deliberately not this
	// gate's.
	// `'employment'` need not be FIRST in an IN list. Ordering there is the
	// author's whim, which is a poor thing for a gate to depend on.
	{"an IN list where employment is not first", true, "", `
func read() string {
	return ` + "`" + `SELECT 1 FROM relationship r WHERE r.kind IN ('deal_stakeholder', 'employment') AND r.ended_at IS NULL` + "`" + `
}`},
	// A bare call in a file that simply does not import people names something
	// else. An empty qualifier used to mean both "this file IS people" and
	// "this file does not import people", and most of the tree is the second.
	{"a bare helper name in a file that does not import people", true, "noimport", `
func read() string {
	return ` + "`" + `SELECT 1 FROM relationship r WHERE r.kind = 'employment' AND ` + "`" + ` + EmploymentIsCurrentSQL("r.ended_at IS NULL") + ` + "`" + ` AND 1=1` + "`" + `
}`},
	{"an IN list whose earlier item is a subquery", true, "", `
func read() string {
	return ` + "`" + `SELECT 1 FROM relationship r WHERE r.kind IN ('x', (SELECT k FROM t), 'employment') AND r.ended_at IS NULL` + "`" + `
}`},
	{"a deal_stakeholder edge", false, "", `
func read() string {
	return ` + "`" + `SELECT 1 FROM relationship r WHERE r.kind = 'deal_stakeholder' AND r.ended_at IS NULL` + "`" + `
}`},
	{"an employment query that never asks about currency", false, "", `
func read() string {
	return ` + "`" + `SELECT count(*) FROM relationship r WHERE r.kind = 'employment'` + "`" + `
}`},
}

func TestTheEmploymentDetectorSeesWhatItClaimsTo(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	for _, tc := range employmentProbes {
		t.Run(tc.name, func(t *testing.T) {
			head := "package probe\n"
			names := map[string]bool{employmentHelper: true, primaryHelper: true}
			scope := helperScope{qualifier: "people", names: names}
			switch tc.mode {
			case "people":
				head, scope = "package people\n", helperScope{inside: true, names: names}
			case "noimport":
				scope = helperScope{names: names}
			default:
				head += "import (\n\t\"fmt\"\n\n\t\"github.com/margince/margince/backend/internal/modules/people\"\n)\n"
			}
			file, err := parser.ParseFile(fset, "probe.go", head+tc.src, 0)
			if err != nil {
				t.Fatalf("the probe does not parse, so it proves nothing: %v", err)
			}
			hit := false
			for _, decl := range file.Decls {
				for _, sql := range employmentStatements(decl, scope) {
					if endedAtCurrency.MatchString(sql) {
						hit = true
					}
				}
			}
			if tc.fires && !hit {
				t.Errorf("the detector missed a hand-written currency test — the census would read green over this:\n%s", tc.src)
			}
			if !tc.fires && hit {
				t.Errorf("the detector reported a statement that asks the one definition, or asks nothing:\n%s", tc.src)
			}
		})
	}
}

// The OTHER question about is_current_primary: which row holds the slot
// uq_rel_current_primary_employer keeps unique. It is date-BLIND, so it cannot
// share EmploymentIsCurrentSQL — a guard that asked "are they still employed"
// would read a person serving notice as having freed a slot the index still
// holds, and the write behind it would 409 instead of skipping.
//
// Six statements asked it by hand. The census above could not see any of them:
// it judges a hand-written test on `ended_at`, and these spell neither the
// helper nor that column. So this is a second detector over the same tree, for
// the FRAGMENT — the flag AND-ed with an archived test — which is the shape
// that hid six copies from a census that only knew whole predicates.

// currentPrimarySlotPredicate names the helper this census requires, so the
// report can point at it.
const currentPrimarySlotPredicate = "CurrentPrimarySlotSQL"

// slotBlockedByTheModuleDAG ratifies the statement that cannot adopt the
// helper today, for the architectural reason above and not for want of
// somebody getting round to it: projects may not import people (ADR-0054 §3).
//
// Its own declaration and not a share of blockedByTheModuleDAG: a Waivers set
// records what reached it, and AssertAllMatched belongs to exactly one census —
// two sweeping the same set would make whichever ran first report false
// staleness.
var slotBlockedByTheModuleDAG = gatekit.Waive(map[string]string{
	"internal/modules/projects/surface.go": "projects cannot import people (ADR-0054 §3); the predicate must move tier first",
})

// spellsSlotPredicate reports whether a statement tests the flag and an
// archived test IN ONE CONJUNCTION — which is the slot predicate, however it is
// spelled.
//
// It works on conjunctive CHUNKS rather than on a two-term pattern, because
// every shape a two-term pattern misses is a respelling somebody writes without
// noticing: the two halves in the other order, another conjunct sitting between
// them (`b.is_current_primary AND b.id <> a.id AND b.archived_at IS NULL` — the
// merge copy was one edit from that), `= true` or `IS TRUE` instead of the bare
// flag, and lower-case keywords. A text comparison loses to an equivalent
// spelling, and there are four of them here.
//
// Splitting on OR first is what keeps the create path out. Its guard is
// deliberately WIDER than the slot — `archived_at IS NULL AND (still employed
// OR is_current_primary)` — and there the flag and the archived test are in
// different groups, which is a different question rather than a copy of this
// one. Brackets are TRIMMED from a term rather than treated as separators: a
// bracket is where a conjunction nests, not where it ends, and a split on them
// missed `is_current_primary AND (archived_at IS NULL AND person_id = $1)`.
func spellsSlotPredicate(sql string) bool {
	for _, group := range slotOr.Split(strings.ToLower(sql), -1) {
		flag, archived := false, false
		for _, term := range slotAnd.Split(group, -1) {
			if asksTheFlag(term) {
				flag = true
			}
			if slotArchivedTerm.MatchString(term) {
				archived = true
			}
		}
		if flag && archived {
			return true
		}
	}
	return false
}

// asksTheFlag reports whether a conjunct TESTS the flag, which is the only
// mention of it that makes a statement a guard. Two other mentions exist and
// neither is one: an INSERT's column list NAMES it, and a merge relink ASSIGNS
// it (`SET is_current_primary = a.is_current_primary AND NOT EXISTS (…)`) —
// there the flag is the value being carried across, and the guard is the
// EXISTS beside it.
//
// A test is what a bracketed SEGMENT of the conjunct ends with — the segment
// and not the whole conjunct, because the statement text is a fragment and a
// predicate's last term carries whatever closes and follows it. A column list
// puts a comma after the name; an assignment puts an equals sign before it; a
// NEGATION asks the opposite question, which is "who does NOT hold the slot".
//
// One shape is knowingly over-reported: an INSERT whose column list is exactly
// `(is_current_primary)`, where the closing bracket leaves a segment that reads
// as a bare test. Distinguishing it would mean deciding whether an opening
// bracket follows a keyword or a name, and getting THAT wrong drops the
// contents of `NOT EXISTS (…)` — where five of the six statements this census
// judges live. Over-reporting is a finding somebody dismisses; under-reporting
// is a census that reads green over the defect and says nothing.
func asksTheFlag(term string) bool {
	for _, segment := range slotBrackets.Split(term, -1) {
		trimmed := strings.TrimRight(segment, " \t\n")
		at := slotFlagTail.FindStringIndex(trimmed)
		if at == nil || at[1] != len(trimmed) {
			continue
		}
		before := strings.TrimRight(trimmed[:at[0]], " \t\n")
		if strings.HasSuffix(before, ",") || strings.HasSuffix(before, "=") || strings.HasSuffix(before, "not") {
			continue
		}
		return true
	}
	return false
}

var (
	// slotOr ends a conjunction; slotAnd separates the terms inside one. Both
	// are bounded so `or` does not match inside `organization_id`.
	slotOr  = regexp.MustCompile(`\bor\b`)
	slotAnd = regexp.MustCompile(`\band\b`)

	// slotBrackets ends a segment inside a conjunct — see asksTheFlag.
	slotBrackets = regexp.MustCompile(`[()]`)

	// slotFlagTail is the flag asked as a boolean, in every spelling this
	// dialect offers, anchored to the END of a conjunct — see asksTheFlag.
	slotFlagTail = regexp.MustCompile(`(\w+\.)?is_current_primary(\s*=\s*true|\s+is\s+true)?$`)

	// slotArchivedTerm is unanchored, because the statement text a census reads
	// is a fragment: a conjunct carries whatever surrounds it — a SELECT before
	// it, an ON CONFLICT after. There is no column-list ambiguity to guard
	// against here the way there is for the flag, since a column list names
	// `archived_at` and never tests it.
	slotArchivedTerm = regexp.MustCompile(`(\w+\.)?archived_at\s+is\s+null\b`)
)

// probeMarker exempts ONE DECLARATION from the censuses above, for the only
// reason a gate's own file needs exempting: it plants deliberate defects as
// evidence, and judging them would report the gate's proof as a finding.
//
// A declaration and not a file, which is the whole change. This file used to be
// skipped whole — and it is the file most likely to attract a real hand-written
// currency test, because it is where somebody working on this rule is already
// editing. The one place the rule did not apply was the one place it was most
// needed.
//
// Not "_test.go" either, for the reason the file skip already gave: a real test
// that hand-writes an employment currency test is still a finding.
const probeMarker = "//gate:probe"

// probeOwner is this gate's own file, where both probe tables live. By PATH and
// not by basename: a basename would name any future file so called, anywhere in
// the tree.
const probeOwner = gateDir + "/employmentcurrency_test.go"

// plantedProbe reports whether a declaration in path carries the marker.
//
// Honoured in the PROBE OWNER only, which is what keeps the marker from being a
// way past these censuses. Written tree-wide it would be exactly that: a doc
// comment on any declaration anywhere would exempt it from both, and the file
// teaching the marker is the one somebody copies from. Refusing it elsewhere
// closes that by construction rather than by a test that notices afterwards —
// and TestNoStrayProbeMarker reports one anyway, so somebody who writes it is
// told it does nothing instead of believing it worked.
//
// The doc comment specifically, not any comment in the declaration's span: a
// marker inside a function body would exempt the function it sits in, which is
// a way to silence a finding rather than to declare evidence.
func plantedProbe(path string, decl ast.Decl) bool {
	if filepath.ToSlash(path) != probeOwner {
		return false
	}
	var doc *ast.CommentGroup
	switch node := decl.(type) {
	case *ast.GenDecl:
		doc = node.Doc
	case *ast.FuncDecl:
		doc = node.Doc
	}
	if doc == nil {
		return false
	}
	for _, line := range doc.List {
		if strings.TrimSpace(line.Text) == probeMarker {
			return true
		}
	}
	return false
}

// slotColumn is what makes a statement a candidate at all, and it is the floor
// this census counts against: `is_current_primary` is a relationship column and
// nothing else in this schema carries it, so no kind-scoping is needed and none
// is done — scoping to `kind = 'employment'` would have dropped every adopted
// site, since the kind now lives inside the helper.
var slotColumn = regexp.MustCompile(`(?i)is_current_primary`)

func TestEveryCurrentPrimarySlotGuardUsesTheOneSpelling(t *testing.T) {
	t.Parallel()
	// A ratification that stops matching describes a site that has moved or
	// been fixed, and leaving it quietly re-exempts whatever takes its name.
	defer slotBlockedByTheModuleDAG.AssertAllMatched(t)

	fset := token.NewFileSet()
	var findings []string
	judged := 0
	for _, path := range handWrittenGoSources(t) {
		if filepath.ToSlash(path) == employmentCurrencyOwner {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		scope := helperScope{
			qualifier: importAliasOf(file, "github.com/margince/margince/backend/internal/modules/people"),
			inside:    file.Name != nil && file.Name.Name == "people",
			names:     map[string]bool{currentPrimarySlotPredicate: true},
		}
		for _, decl := range file.Decls {
			if plantedProbe(path, decl) {
				continue
			}
			for _, sql := range slotStatements(decl, scope) {
				judged++
				if !spellsSlotPredicate(sql) {
					continue
				}
				if slotBlockedByTheModuleDAG.Waived(t, filepath.ToSlash(path)) {
					continue
				}
				findings = append(findings, fmt.Sprintf("%s: %s", path, firstSlotLine(sql)))
			}
		}
	}
	if judged < 5 {
		t.Fatalf("only %d statement(s) naming is_current_primary were judged, so this census covered almost nothing", judged)
	}
	if len(findings) == 0 {
		return
	}
	t.Errorf("these statements spell the current-primary slot predicate by hand:\n  %s\n\n"+
		"people.%s is the one spelling, and it mirrors uq_rel_current_primary_employer's own "+
		"predicate — including the kind, which is part of that index. Call it.",
		strings.Join(findings, "\n  "), currentPrimarySlotPredicate)
}

// slotStatements returns the SQL statements in a declaration that name the
// slot column, flattened so a helper call contributes its NAME and a
// concatenated fragment is judged with the statement it belongs to.
func slotStatements(decl ast.Decl, people helperScope) []string {
	var out []string
	seen := map[ast.Node]bool{}
	ast.Inspect(decl, func(n ast.Node) bool {
		if seen[n] {
			return false
		}
		text, ok := flattenSQL(n, seen, people)
		if !ok || !slotColumn.MatchString(text) {
			return true
		}
		out = append(out, text)
		return true
	})
	return out
}

// firstSlotLine points the report at the offending line rather than dumping
// the whole statement. The fragment itself may straddle a line break — that is
// how two of the six copies were written — so the flag's own line is what is
// reported when no single line carries the whole match.
func firstSlotLine(sql string) string {
	lines := strings.Split(sql, "\n")
	for _, line := range lines {
		if slotColumn.MatchString(line) {
			return strings.TrimSpace(line)
		}
	}
	return strings.TrimSpace(lines[0])
}

// slotProbe is one planted statement and the answer the detector must give
// for it. Every case is a shape the census would read green over if the
// detector stopped seeing it — the census itself passes identically over a
// clean tree and over a detector that has stopped detecting.
//
//gate:probe
var slotProbes = []struct {
	name  string
	fires bool
	mode  string
	src   string
}{
	{"the bare form that shipped", true, "", "\nfunc read() string {\n\treturn `SELECT 1 FROM relationship WHERE kind = 'employment' AND person_id = $1 AND is_current_primary AND archived_at IS NULL`\n}"},
	{"the aliased form", true, "", "\nfunc read() string {\n\treturn `SELECT 1 FROM relationship b WHERE b.is_current_primary AND b.archived_at IS NULL`\n}"},
	// The mirrored conjunction is the same predicate written the other way
	// round, and a census that reads one direction lets it through.
	{"the two halves in the other order", true, "", "\nfunc read() string {\n\treturn `SELECT 1 FROM relationship b WHERE b.archived_at IS NULL AND b.is_current_primary`\n}"},
	{"the fragment split across a concatenation", true, "", "\nfunc read() string {\n\treturn `SELECT 1 FROM relationship WHERE is_current_primary AND ` + `archived_at IS NULL`\n}"},
	// A helper call claims its whole subtree, so a lookalike from another
	// package would have hidden a hand-written fragment inside its arguments.
	{"a lookalike helper from another package", true, "", "\nfunc read() string {\n\treturn `SELECT 1 FROM relationship b WHERE ` + other.CurrentPrimarySlotSQL(\"b.is_current_primary AND b.archived_at IS NULL\")\n}"},
	{"a bare helper name in a file that does not import people", true, "noimport", "\nfunc read() string {\n\treturn `SELECT 1 FROM relationship b WHERE ` + CurrentPrimarySlotSQL(\"b.is_current_primary AND b.archived_at IS NULL\")\n}"},

	{"the real helper, qualified", false, "", "\nfunc read() string {\n\treturn `SELECT 1 FROM relationship b WHERE b.person_id = $1 AND ` + people.CurrentPrimarySlotSQL(\"b\")\n}"},
	{"the real helper, unqualified inside people", false, "people", "\nfunc read() string {\n\treturn `SELECT 1 FROM relationship WHERE person_id = $1 AND ` + CurrentPrimarySlotSQL(\"\")\n}"},
	// The create path's guard is deliberately WIDER than the slot: it refuses
	// the flag when the person has any employment that is current OR flagged,
	// which is not the index's predicate and must not be rewritten as it.
	{"the wider create-path guard, where the flag sits inside an OR", false, "", "\nfunc read() string {\n\treturn `SELECT 1 FROM relationship WHERE kind = 'employment' AND person_id = $2 AND archived_at IS NULL AND (` + EmploymentIsCurrentSQL(\"ended_at\") + ` OR is_current_primary)`\n}"},
	{"the flag with no archived test beside it", false, "", "\nfunc read() string {\n\treturn `UPDATE relationship SET is_current_primary = coalesce($3, is_current_primary)`\n}"},

	// Four spellings a two-term pattern missed, each verified green against it
	// before this detector was rewritten to work on conjunctive chunks.
	{"another conjunct sitting between the two halves", true, "", "\nfunc read() string {\n\treturn `SELECT 1 FROM relationship b WHERE b.is_current_primary AND b.id <> $1 AND b.archived_at IS NULL`\n}"},
	{"the flag compared to true rather than asked", true, "", "\nfunc read() string {\n\treturn `SELECT 1 FROM relationship WHERE is_current_primary = true AND archived_at IS NULL`\n}"},
	{"the flag asked with IS TRUE", true, "", "\nfunc read() string {\n\treturn `SELECT 1 FROM relationship WHERE is_current_primary IS TRUE AND archived_at IS NULL`\n}"},
	{"lower-case keywords", true, "", "\nfunc read() string {\n\treturn `select 1 from relationship where is_current_primary and archived_at is null`\n}"},
	// The archived test belongs to the OTHER side of an OR, so the flag is not
	// AND-ed with it and this is not the slot predicate.
	{"the two halves on opposite sides of an OR", false, "", "\nfunc read() string {\n\treturn `SELECT 1 FROM relationship WHERE is_current_primary OR archived_at IS NULL`\n}"},
	// A bracket is where a conjunction NESTS, not where it ends. A detector
	// that ended a group at every bracket read this as two groups and missed
	// the guard sitting whole inside them.
	{"the second half inside a bracketed conjunction", true, "", "\nfunc read() string {\n\treturn `SELECT 1 FROM relationship WHERE is_current_primary AND (archived_at IS NULL AND person_id = $1)`\n}"},
	// The merge relinks CARRY the flag across rather than testing it; the
	// guard beside them is the EXISTS, which calls the helper.
	{"the flag as the value of an assignment", false, "", "\nfunc read() string {\n\treturn `UPDATE relationship a SET is_current_primary = a.is_current_primary AND NOT EXISTS (SELECT 1 FROM relationship b WHERE b.person_id = $2) WHERE a.person_id = $1 AND a.archived_at IS NULL`\n}"},
	// A statement that merely WRITES the column names it in a list; naming is
	// not asking.
	{"the flag in an INSERT's column list", false, "", "\nfunc read() string {\n\treturn `INSERT INTO relationship (kind, person_id, is_current_primary, archived_at) SELECT 'employment', $1, true, NULL FROM person WHERE archived_at IS NULL`\n}"},
	// The negation asks who does NOT hold the slot, which is the opposite
	// question and not a second spelling of this one.
	{"the flag negated", false, "", "\nfunc read() string {\n\treturn `SELECT 1 FROM relationship WHERE NOT is_current_primary AND archived_at IS NULL`\n}"},
	// A guard inside NOT EXISTS, where five of the six judged statements live:
	// the flag sits behind an opening bracket, and a detector that decided
	// brackets by what precedes them would drop it.
	{"a guard inside a NOT EXISTS subquery", true, "", "\nfunc read() string {\n\treturn `UPDATE relationship a SET x = 1 WHERE NOT EXISTS (SELECT 1 FROM relationship b WHERE b.is_current_primary AND b.archived_at IS NULL)`\n}"},
}

func TestTheSlotDetectorSeesWhatItClaimsTo(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	for _, tc := range slotProbes {
		t.Run(tc.name, func(t *testing.T) {
			head := "package probe\n"
			names := map[string]bool{currentPrimarySlotPredicate: true}
			scope := helperScope{qualifier: "people", names: names}
			switch tc.mode {
			case "people":
				head, scope = "package people\n", helperScope{inside: true, names: names}
			case "noimport":
				scope = helperScope{names: names}
			default:
				head += "import \"github.com/margince/margince/backend/internal/modules/people\"\n"
			}
			file, err := parser.ParseFile(fset, "probe.go", head+tc.src, 0)
			if err != nil {
				t.Fatalf("the probe does not parse, so it proves nothing: %v", err)
			}
			hit := false
			for _, decl := range file.Decls {
				for _, sql := range slotStatements(decl, scope) {
					if spellsSlotPredicate(sql) {
						hit = true
					}
				}
			}
			if tc.fires && !hit {
				t.Errorf("the detector missed a hand-spelled slot predicate — the census would read green over this:\n%s", tc.src)
			}
			if !tc.fires && hit {
				t.Errorf("the detector reported a statement that calls the one spelling, or asks a different question:\n%s", tc.src)
			}
		})
	}
}

// TestTheProbeMarkerExemptsTheProbesAndNothingElse is what makes the narrowing
// real rather than a differently-spelled whole-file skip.
//
// Both censuses above used to skip this file entirely, and the consequence was
// the opposite of what a gate is for: the one file where the rule did not apply
// was the file most likely to attract a violation, because it is where somebody
// working on this rule is already editing. Skipping declarations closes that —
// but only while the marker stays on the evidence and off everything else.
//
// So: this file must be JUDGED, exactly two declarations may carry the marker,
// and they must be the probe tables.
func TestTheProbeMarkerExemptsTheProbesAndNothingElse(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, probeOwner, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing this gate's own file: %v", err)
	}

	var marked []string
	judged := 0
	for _, decl := range file.Decls {
		if plantedProbe(probeOwner, decl) {
			marked = append(marked, declaredName(decl))
			continue
		}
		judged++
	}

	if judged == 0 {
		t.Fatal("every declaration in this file is marked as a probe, which is the whole-file skip " +
			"this test exists to prevent, spelled differently")
	}
	want := []string{"employmentProbes", "slotProbes"}
	sort.Strings(marked)
	if !slices.Equal(marked, want) {
		t.Errorf("the probe marker is on %v, want exactly %v — it exempts a declaration from BOTH "+
			"censuses, so a marker anywhere else is a finding silenced rather than evidence declared",
			marked, want)
	}
}

// declaredName names a declaration for a failure message.
func declaredName(decl ast.Decl) string {
	switch node := decl.(type) {
	case *ast.FuncDecl:
		if node.Name != nil {
			return node.Name.Name
		}
	case *ast.GenDecl:
		for _, spec := range node.Specs {
			switch value := spec.(type) {
			case *ast.ValueSpec:
				if len(value.Names) > 0 {
					return value.Names[0].Name
				}
			case *ast.TypeSpec:
				if value.Name != nil {
					return value.Name.Name
				}
			}
		}
	}
	return "an unnamed declaration"
}

// TestNoStrayProbeMarker reports a marker written where it does nothing.
//
// plantedProbe honours the marker in its own file only, so a stray one cannot
// exempt anything — the vulnerability is closed by construction. What is left is
// the reader who wrote one and believes it worked, and a silent no-op is the
// worst answer to give them: they think a declaration is declared evidence, and
// it is being judged.
//
// It is also the arm that keeps the construction honest. If plantedProbe were
// ever widened to honour the marker tree-wide, this is what would already be
// standing there to catch the first stray.
func TestNoStrayProbeMarker(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	var stray []string
	for _, path := range handWrittenGoSources(t) {
		if filepath.ToSlash(path) == probeOwner {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		for _, group := range file.Comments {
			for _, line := range group.List {
				if strings.TrimSpace(line.Text) == probeMarker {
					stray = append(stray,
						fmt.Sprintf("%s:%d", filepath.ToSlash(path), fset.Position(line.Pos()).Line))
				}
			}
		}
	}
	if len(stray) > 0 {
		t.Errorf("%s appears in %v, where it exempts nothing. The marker is honoured in %s alone, "+
			"which is what stops it becoming a way past these censuses — so a copy elsewhere is a "+
			"declaration somebody believes is evidence while it is being judged. Delete it, or fix the "+
			"finding it was written to silence", probeMarker, stray, probeOwner)
	}
}
