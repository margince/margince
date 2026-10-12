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
	"go/ast"
	"go/printer"
	"go/token"
	"regexp"
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

// last_activity_of_company spells the walk's employment arm in SQL, and
// last_activity_reach names the employers a write locks. Both carry the walk's
// rendered text, so the clock, its lock set and the timeline agree.
func TestTheStoredCompanyClockBoundsEmploymentLikeTheWalk(t *testing.T) {
	t.Parallel()
	bodies := newestFunctionBodies(t, "last_activity_of_company", "last_activity_reach")
	for _, fn := range []string{"last_activity_of_company", "last_activity_reach"} {
		body, ok := bodies[fn]
		if !ok {
			t.Fatalf("no migration defines %s, so this gate checks nothing", fn)
		}
		arms := employmentArms(body)
		if len(arms) == 0 {
			t.Errorf("%s has no employment arm the gate can find, so it checks nothing", fn)
		}
		for _, arm := range arms {
			if !arm.current {
				t.Errorf("%s's employment arm on %s does not carry %q, or also tests ended_at IS NULL "+
					"on its own, so a contact serving notice stops counting for the employer they still "+
					"work for. Replace the function in a new migration with the predicate's current text.",
					fn, arm.alias, employment.IsCurrentSQL(arm.alias+".ended_at"))
			}
			if fn == "last_activity_of_company" && !arm.bounded {
				t.Errorf("%s's employment arm on %s does not carry %q, so mail from a "+
					"contact's previous job sets the clock of the employer they have now. Replace the "+
					"function in a new migration with the bound's current text.",
					fn, arm.alias, employment.InPlaceAtSQL(arm.alias, "a.occurred_at"))
			}
		}
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
		{
			"bounded and current",
			"SELECT 1 FROM relationship r WHERE r.kind = 'employment' AND " + current + " AND " + bound, 1,
			nil, nil,
		},
		{
			"second arm under another alias, unbounded and bare",
			"SELECT 1 FROM relationship r WHERE r.kind = 'employment' AND " + current + " AND " + bound +
				"\n UNION ALL\n SELECT 1 FROM relationship emp WHERE emp.kind='employment' AND " + bareEndedAtTest("emp"), 2,
			[]string{"emp"},
			[]string{"emp"},
		},
		{
			"bound only in a comment",
			"SELECT 1 FROM relationship r WHERE r . kind\n = 'employment' AND " + current + " -- AND " + bound, 1,
			[]string{"r"},
			nil,
		},
		{
			"bound only in a block comment",
			"SELECT 1 FROM relationship r WHERE r.kind = 'employment' AND " + current + " /* AND " + bound + " */", 1,
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
			[]string{"r"},
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

var (
	sqlBlockComment   = regexp.MustCompile(`(?s)/\*.*?\*/`)
	sqlUnion          = regexp.MustCompile(`(?i)\bUNION(?:\s+ALL)?\b`)
	employmentKindArm = regexp.MustCompile(`(?i)\b(\w+)\s*\.\s*kind\s*=\s*'employment'`)
)

// employmentArm is one employment arm of a function body, under its alias.
type employmentArm struct {
	alias   string
	bounded bool
	current bool
}

// employmentArms reads each employment arm of a function body. Comments are
// dropped first, so a predicate that is only mentioned does not count.
func employmentArms(body string) []employmentArm {
	code := stripSQLComments(sqlBlockComment.ReplaceAllString(body, " "))
	var arms []employmentArm
	for _, part := range sqlUnion.Split(code, -1) {
		flat := normalizeSQL(part)
		for _, m := range employmentKindArm.FindAllStringSubmatch(flat, -1) {
			alias := m[1]
			current := normalizeSQL(employment.IsCurrentSQL(alias + ".ended_at"))
			bareEnd := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(alias) + `\s*\.\s*ended_at\s+IS\s+NULL\b`)
			arms = append(arms, employmentArm{
				alias:   alias,
				bounded: strings.Contains(flat, normalizeSQL(employment.InPlaceAtSQL(alias, "a.occurred_at"))),
				current: strings.Contains(flat, current) && !bareEnd.MatchString(strings.ReplaceAll(flat, current, " ")),
			})
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
