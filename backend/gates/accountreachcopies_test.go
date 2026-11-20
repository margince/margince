// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// The stored company clock and the activity walk use the same employment bounds.

import (
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/employment"
)

// last_activity_of_company repeats the employment arm in SQL
// because a trigger folds it. last_activity_of_company_activity is that arm for
// one new link. Their bound is the walk's own rendered text, so a company's
// stored clock and its timeline count the same mail.
func TestTheStoredCompanyClockBoundsEmploymentLikeTheWalk(t *testing.T) {
	t.Parallel()
	bodies := currentRecencyBodies(t)
	for _, name := range []string{"last_activity_of_company", "last_activity_of_company_activity"} {
		body, ok := bodies[name]
		if !ok {
			t.Fatalf("no migration defines %s, so this gate checks nothing", name)
		}
		arms, unbounded := employmentArmsWithoutTheBound(body)
		if arms == 0 {
			t.Errorf("%s has no employment arm the gate can find, so it checks nothing", name)
		}
		_, notCurrent := employmentArmsWithout(body, employment.CurrentMembershipSQL)
		for _, alias := range notCurrent {
			t.Errorf("%s's employment arm on %s does not carry %q, so the stored clock and the "+
				"walk disagree about who still works there. Replace the function in a new migration "+
				"with the predicate's current text.", name, alias, employment.CurrentMembershipSQL(alias))
		}
		for _, alias := range unbounded {
			t.Errorf("%s's employment arm on %s does not carry %q, so mail from a "+
				"contact's previous job sets the clock of the employer they have now. Replace the "+
				"function in a new migration with the bound's current text.",
				name, alias, employment.InPlaceForSQL(alias, "a"))
		}
	}
}

// last_activity_reach names the companies a contact's write may move. It has
// to cover every employer the stored clock counts, so it spells the same
// current-employment predicate.
func TestTheClockReachCoversTheEmployersTheClockCounts(t *testing.T) {
	t.Parallel()
	reachFunction := regexp.MustCompile(`(?i)(CREATE\s+(?:OR\s+REPLACE\s+)?|DROP\s+)FUNCTION\s+(?:IF\s+EXISTS\s+)?(last_activity_reach)`)
	body, ok := currentFunctionBodies(t, reachFunction)["last_activity_reach"]
	if !ok {
		t.Fatal("no migration defines last_activity_reach, so this gate checks nothing")
	}
	arms, notCurrent := employmentArmsWithout(body, employment.CurrentMembershipSQL)
	if arms == 0 {
		t.Error("last_activity_reach has no employment arm the gate can find, so it checks nothing")
	}
	for _, alias := range notCurrent {
		t.Errorf("last_activity_reach's employment arm on %s does not carry %q, so a contact's "+
			"write can miss an employer whose stored clock it moves", alias, employment.CurrentMembershipSQL(alias))
	}
}

func TestTheClockBoundCensusSeesEveryEmploymentArm(t *testing.T) {
	t.Parallel()
	bound := employment.InPlaceForSQL("r", "a")
	cases := []struct {
		name      string
		body      string
		arms      int
		unbounded []string
	}{
		{"bounded", "SELECT 1 FROM relationship r WHERE r.kind = 'employment' AND " + bound, 1, nil},
		{
			"second arm under another alias, unbounded",
			"SELECT 1 FROM relationship r WHERE r.kind = 'employment' AND " + bound +
				"\n UNION ALL\n SELECT 1 FROM relationship emp WHERE emp.kind='employment'", 2,
			[]string{"emp"},
		},
		{
			"bound only in a comment",
			"SELECT 1 FROM relationship r WHERE r . kind\n = 'employment' -- AND " + bound, 1,
			[]string{"r"},
		},
		{
			"bound only in a block comment",
			"SELECT 1 FROM relationship r WHERE r.kind = 'employment' /* AND " + bound + " */", 1,
			[]string{"r"},
		},
	}
	for _, tc := range cases {
		arms, unbounded := employmentArmsWithoutTheBound(tc.body)
		if arms != tc.arms || strings.Join(unbounded, ",") != strings.Join(tc.unbounded, ",") {
			t.Errorf("%s: found %d arms, unbounded %v; want %d, %v", tc.name, arms, unbounded, tc.arms, tc.unbounded)
		}
	}
}

var (
	sqlBlockComment   = regexp.MustCompile(`(?s)/\*.*?\*/`)
	sqlUnionAll       = regexp.MustCompile(`(?i)\bUNION\s+ALL\b`)
	employmentKindArm = regexp.MustCompile(`(?i)\b(\w+)\s*\.\s*kind\s*=\s*'employment'`)
)

// employmentArmsWithoutTheBound counts the employment arms of a function body
// and names the alias of each arm that lacks employment.InPlaceForSQL. Comments
// are dropped first, so a bound that is only mentioned does not count.
func employmentArmsWithoutTheBound(body string) (int, []string) {
	return employmentArmsWithout(body, func(alias string) string { return employment.InPlaceForSQL(alias, "a") })
}

// employmentArmsWithout is the census for one predicate, spelled for an alias.
func employmentArmsWithout(body string, predicate func(alias string) string) (int, []string) {
	code := stripSQLComments(sqlBlockComment.ReplaceAllString(body, " "))
	arms := 0
	var unbounded []string
	for _, part := range sqlUnionAll.Split(code, -1) {
		flat := normalizeSQL(part)
		for _, m := range employmentKindArm.FindAllStringSubmatch(flat, -1) {
			arms++
			if !strings.Contains(flat, normalizeSQL(predicate(m[1]))) {
				unbounded = append(unbounded, m[1])
			}
		}
	}
	return arms, unbounded
}

// normalizeSQL collapses the whitespace two files indent differently, so the
// comparison is about the SQL rather than about where each copy sits on the
// page.
func normalizeSQL(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
