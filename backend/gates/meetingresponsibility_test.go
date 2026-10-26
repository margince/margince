// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

// Every reader of "whose meeting is this" asks storekit.MeetingResponsibilitySQL
// or UnclaimedMeetingSQL, or the meeting credit rule built on it. A reader that compares the host column by itself
// credits a captured meeting to whoever imported the calendar. The Worklist,
// follow-ups and weekly figures credit the colleagues who took part.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

var meetingRuleRenderers = []string{"MeetingResponsibilitySQL", "UnclaimedMeetingSQL", "MeetingCreditSQL", "MeetingCreditedSeatSQL", "MeetingCreditedSeatsSQL", "MeetingCreditedSeatsAtChangeSQL", "MeetingCreditedSeatAtChangeSQL"}

const meetingRuleFile = "internal/platform/database/storekit/meetingresponsibility.go"

// activityAlias captures the alias a statement gives the activity table. The
// `\s` after the name keeps activity_participant and its siblings out.
var activityAlias = regexp.MustCompile(`(?i)\b(?:FROM|JOIN)\s+activity\s+(?:AS\s+)?([a-z_][a-z0-9_]*)`)

var sqlKeywordsAfterTable = map[string]bool{
	"where": true, "join": true, "left": true, "inner": true, "on": true, "order": true,
	"group": true, "limit": true, "union": true, "cross": true, "set": true, "using": true,
}

// unqualifiedHostPredicate is the host column compared without an alias, read
// only in a literal that itself selects from the activity table.
var unqualifiedHostPredicate = regexp.MustCompile(`(?i)(?:^|[^.\w])host_user_id\s*(?:=|<>|!=|\bIS\b|\bIN\b)`)

var activityTableRead = regexp.MustCompile(`(?i)\b(?:FROM|JOIN)\s+activity(?:\s|$)`)

// hostColumnExpression is the column handed on whole, as a report dimension or
// as the tail of `alias + ".host_user_id = "`, for a comparison built elsewhere.
var hostColumnExpression = regexp.MustCompile(`^\s*(?:[a-z_][a-z0-9_]*)?\.host_user_id\b`)

// attendanceRoles is the rule's own evidence list spelled by hand.
var attendanceRoles = regexp.MustCompile(`(?i)role\s+IN\s*\(\s*'(?:organizer'\s*,\s*'attendee|attendee'\s*,\s*'organizer)'\s*\)`)

// meetingHostReaders are the files allowed to read the host column or the
// attendance roles without the rule, each with the question it asks instead.
var meetingHostReaders = gatekit.Waive(map[string]string{
	"internal/platform/auth/activitywritescope.go": "asks who may change the row; the host may edit a meeting " +
		"whatever the attendance evidence says",
	"internal/modules/activities/scheduling.go": "asks which slots the host's booking calendar already holds; " +
		"busy time belongs to the calendar, not to attendance",
	"internal/modules/activities/scheduling_occupancy.go": "asks which slots the host's booking calendar already " +
		"holds; busy time belongs to the calendar, not to attendance",
	"internal/modules/capture/participantpastcap.go": "writes the attendance rows the rule reads, and skips a " +
		"seat that already has one",
	"internal/modules/activities/activityprojection.go": "reads the host column out so a screen can show who " +
		"hosted; it decides nothing about whose work the meeting is",
})

func TestEveryMeetingOwnershipReaderAsksTheAttendanceRule(t *testing.T) {
	t.Parallel()
	defer meetingHostReaders.AssertAllMatched(t)
	var withRule int
	ruleDirs := map[string]bool{}
	for _, pkg := range goPackagesUnder(t, ".") {
		for _, file := range pkg.files {
			if strings.HasSuffix(file.relPath, meetingRuleFile) {
				continue
			}
			for _, name := range meetingRuleRenderers {
				if gatekit.References(file.ast, storekitImportPath, name) {
					withRule++
					ruleDirs[file.relPath[:strings.LastIndex(file.relPath, "/")]] = true
					break
				}
			}
			for _, finding := range hostOwnershipReadsIn(file) {
				if meetingHostReaders.Waived(t, file.relPath) {
					continue
				}
				t.Errorf("%s %s. Whose meeting it is comes from storekit.MeetingResponsibilitySQL (or "+
					"UnclaimedMeetingSQL for nobody's), which reads the host a human logged, the provider's "+
					"organizer and attendee rows, and the seat's own invitations. If this file asks another "+
					"question, say which in meetingHostReaders under %q", file.relPath, finding, file.relPath)
			}
		}
	}
	// The rule's readers span the worklist, follow-ups, deals and the weekly
	// figures; a walk that found few of them read a smaller tree.
	if withRule < 6 || len(ruleDirs) < 4 {
		t.Fatalf("only %d files in %d packages ask the meeting attendance rule; the census has stopped seeing its readers", withRule, len(ruleDirs))
	}
}

// hostOwnershipReadsIn names each way one file decides whose meeting it is
// without the rule.
func hostOwnershipReadsIn(file goFile) []string {
	var literals []string
	ast.Inspect(file.ast, func(node ast.Node) bool {
		if expr, isExpr := node.(ast.Expr); isExpr {
			if text, isText := gatekit.LiteralText(expr); isText {
				literals = append(literals, text)
			}
		}
		return true
	})
	qualified := qualifiedHostPredicate(activityAliasesIn(literals))
	var out []string
	for _, text := range literals {
		switch {
		case qualified.MatchString(text):
			out = append(out, "compares or credits the activity's host column")
		case activityTableRead.MatchString(text) && unqualifiedHostPredicate.MatchString(text):
			out = append(out, "compares the activity's host column without an alias")
		case attendanceRoles.MatchString(text):
			out = append(out, "spells the organizer and attendee roles by hand")
		case hostColumnExpression.MatchString(text):
			out = append(out, "names the host column as an expression another statement compares or groups by")
		}
	}
	return out
}

// activityAliasesIn collects every alias the file's statements give the
// activity table. A statement is often split over several literals, so the
// alias is read from the whole file, not from the literal that uses it.
func activityAliasesIn(literals []string) []string {
	aliases := map[string]bool{"activity": true}
	for _, text := range literals {
		for _, match := range activityAlias.FindAllStringSubmatch(text, -1) {
			if alias := strings.ToLower(match[1]); !sqlKeywordsAfterTable[alias] {
				aliases[alias] = true
			}
		}
	}
	out := make([]string, 0, len(aliases))
	for alias := range aliases {
		out = append(out, regexp.QuoteMeta(alias))
	}
	return out
}

func qualifiedHostPredicate(aliases []string) *regexp.Regexp {
	column := `(?:\b(?:` + strings.Join(aliases, "|") + `)|%(?:\[\d+\])?s)\.host_user_id\b`
	return regexp.MustCompile(`(?i)` + column + `\s*(?:=|<>|!=|\bIS\b|\bIN\b)` +
		`|(?:=|<>|!=)\s*` + column +
		`|\b(?:THEN|ELSE|COALESCE\s*\()\s*` + column)
}

func TestTheMeetingOwnershipCensusSeesAPlantedReader(t *testing.T) {
	t.Parallel()
	for name, source := range map[string]string{
		"aliased comparison":         "package planted\nvar q = `SELECT 1 FROM activity m WHERE m.kind = 'meeting' AND m.host_user_id = $1`",
		"alias in a sibling literal": "package planted\nvar from = `FROM activity a`\nvar owner = \"CASE WHEN x THEN a.host_user_id ELSE y END\"",
		"reversed comparison":        "package planted\nvar q = `SELECT 1 FROM activity a JOIN app_user u ON u.id = a.host_user_id`",
		"unqualified comparison":     "package planted\nvar q = `SELECT 1 FROM activity WHERE host_user_id = $1 AND kind = 'meeting'`",
		"hand-spelled roles":         "package planted\nvar q = `SELECT 1 FROM activity_participant p WHERE p.role IN ('organizer', 'attendee')`",
		"formatted alias":            "package planted\nvar q = `(%[1]s.host_user_id = %[2]s)`",
		"report dimension":           "package planted\nvar dims = map[string]string{\"host\": \"t.host_user_id\"}",
		"concatenated column":        "package planted\nfunc q(alias string) string { return alias + \".host_user_id = $1\" }",
	} {
		parsed, err := parser.ParseFile(token.NewFileSet(), "planted.go", source, 0)
		if err != nil {
			t.Fatal(err)
		}
		if got := hostOwnershipReadsIn(goFile{relPath: "planted.go", ast: parsed}); len(got) == 0 {
			t.Errorf("the census misses a planted %s", name)
		}
	}
}

func TestTheMeetingOwnershipCensusLeavesOtherHostColumnsAlone(t *testing.T) {
	t.Parallel()
	for name, source := range map[string]string{
		"invitation host":  "package planted\nvar q = `SELECT 1 FROM meeting_invitation m JOIN activity a ON a.id = m.activity_id WHERE m.host_user_id = $1`",
		"booking page":     "package planted\nvar q = `SELECT id FROM booking_page WHERE host_user_id = $1`",
		"projected column": "package planted\nvar q = `SELECT a.id, a.host_user_id FROM activity a WHERE a.id = $1`",
		"wider role list":  "package planted\nvar q = `SELECT 1 FROM activity_participant p WHERE p.role IN ('organizer', 'attendee', 'to', 'cc')`",
	} {
		parsed, err := parser.ParseFile(token.NewFileSet(), "planted.go", source, 0)
		if err != nil {
			t.Fatal(err)
		}
		if got := hostOwnershipReadsIn(goFile{relPath: "planted.go", ast: parsed}); len(got) != 0 {
			t.Errorf("the census flags a planted %s: %v", name, got)
		}
	}
}
