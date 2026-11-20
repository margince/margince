// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind claim H1

package gates

// dealseat.CurrentSQL is the one spelling of "this contact sits on the deal's
// buying committee now". Readers that spelled it by hand disagreed: the
// committee card ignored the End date, and the contact page read any End date
// as already over. This census holds every statement that names the
// deal_stakeholder kind as a SQL literal to the helper.
//
// A statement that names the kind by hand is a finding, whether it tests
// ended_at or not. Leaving the End date out is the defect this gate is for.
// Some readers ask whether any live edge exists at all. Those are ratified
// below by declaration, each with its reason.
//
// What it cannot see: a kind spelled through a Go constant rather than a SQL
// literal, which the flattener renders as a blank.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	dealSeatPackage = "github.com/margince/margince/backend/internal/shared/kernel/dealseat"
	dealSeatOwner   = "internal/shared/kernel/dealseat/dealseat.go"
	dealSeatHelper  = "CurrentSQL"
)

// dealSeatKind matches a statement scoped to deal stakeholder edges, in an
// equality or anywhere in an IN list.
var dealSeatKind = regexp.MustCompile(`kind\s*=\s*'deal_stakeholder'|kind\s+IN\s*\((?:[^()]|\([^()]*\))*'deal_stakeholder'`)

// dealSeatDateBlind ratifies the declarations whose question is whether a live
// edge row exists, keyed by file and declaration name.
var dealSeatDateBlind = gatekit.Waive(map[string]string{
	"internal/compose/company360/roleproposalwrite.go:seatedNow":                "a seat somebody ended is an answer too, and a model reading must not seat that contact on the deal again",
	"internal/compose/contact360/sections.go:dealRolesSection":                  "the contact page lists past seats as well and marks each one current or not through dealseat.CurrentSQL",
	"internal/modules/contacts/demote.go:ensureNoLiveDeal":                      "demotion is blocked by any live edge row, because the edge would be stranded whatever its End date says",
	"internal/modules/contacts/promoteexistingdeal.go:seatQualifiedContact":     "the write slot under uq_rel_deal_contact_role, which holds an ended row too, so a date test would insert a duplicate",
	"internal/modules/deals/recordhandover.go:heldOnRecordSQL":                  "a handover moves every deal the contact has a live edge on, ended or not, so no record is left behind",
	"internal/modules/consent/existingcustomerroute.go:existingCustomerGround":  "asks whether the contact was a buyer when the offer was accepted, a past fact that a later End date does not undo",
	"internal/modules/capture/purgeselect.go:SelectPurgeableContactsTx":         "a contact with any live deal edge is kept out of a capture purge, so the purge never strands a seat",
	"internal/modules/privacy/retentionselectors.go:retentionSelectors":         "retention keeps a contact with any live deal edge; erasing them would strand the edge whatever its End date",
	"internal/modules/privacy/erasure_legalreview.go:subjectDealIDsSQL":         "legal review lists every deal the contact ever sat on, archived edges included",
	"internal/modules/privacy/reportingretention.go:reportingRetentionSelector": "a legal hold reaches every deal the subject ever sat on, not only the current ones",
	"internal/modules/privacy/reportingquestions.go:heldQuestionSource":         "a legal hold reaches every deal the subject ever sat on, not only the current ones",
	"internal/modules/privacy/reportingquestions.go:subjectQuestionSources":     "a subject access report covers every deal the subject ever sat on",
	"internal/modules/privacy/sarreporting.go:sarReportingSections":             "a subject access report covers every deal the subject ever sat on",
	"internal/modules/privacy/reportingredaction.go:redactSubjectReporting":     "redaction reaches every deal the subject ever sat on",
})

func TestEveryDealSeatCurrencyTestUsesTheOneDefinition(t *testing.T) {
	t.Parallel()
	defer dealSeatDateBlind.AssertAllMatched(t)
	var findings []string
	judged := 0
	for _, path := range handWrittenGoSources(t) {
		slash := filepath.ToSlash(path)
		// A test reads rows to check what a writer did, which is not a
		// reader deciding who sits on a committee.
		if slash == dealSeatOwner || strings.HasSuffix(slash, "_test.go") {
			continue
		}
		file, err := gatekit.ParseFile(path, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		scope := helperScope{
			qualifier: importAliasOf(file, dealSeatPackage),
			inside:    file.Name != nil && file.Name.Name == "dealseat",
			names:     map[string]bool{dealSeatHelper: true},
		}
		for _, decl := range file.Decls {
			for _, sql := range dealSeatStatements(decl, scope) {
				judged++
				subject := slash + ":" + declName(decl)
				if dealSeatDateBlind.Waived(t, subject) {
					continue
				}
				findings = append(findings, subject+": "+firstDealSeatLine(sql))
			}
		}
	}
	if judged < 10 {
		t.Fatalf("only %d deal_stakeholder statement(s) were judged, so this census covered almost nothing", judged)
	}
	if len(findings) > 0 {
		t.Errorf("these statements decide by hand whether a contact sits on a deal:\n  %s\n\n"+
			"Call dealseat.CurrentSQL, which keeps a future End date current until that day and takes "+
			"an ended seat off the committee. If the statement asks whether any live edge row exists, "+
			"ratify it in dealSeatDateBlind with the reason.", strings.Join(findings, "\n  "))
	}
}

// dealSeatStatements returns the flattened statements in decl that name the
// deal_stakeholder kind as a literal.
func dealSeatStatements(decl ast.Decl, scope helperScope) []string {
	var out []string
	seen := map[ast.Node]bool{}
	ast.Inspect(decl, func(n ast.Node) bool {
		// A map literal flattens to blanks, so each value is judged on its own.
		if pair, ok := n.(*ast.KeyValueExpr); ok {
			n = pair.Value
		}
		if seen[n] {
			return false
		}
		text, ok := flattenSQL(n, seen, scope)
		if ok && dealSeatKind.MatchString(text) {
			out = append(out, text)
		}
		return true
	})
	return out
}

func firstDealSeatLine(sql string) string {
	for line := range strings.SplitSeq(sql, "\n") {
		if dealSeatKind.MatchString(line) {
			return strings.TrimSpace(line)
		}
	}
	return strings.TrimSpace(strings.Split(sql, "\n")[0])
}

// The census passes the same way over a clean tree and over a detector that
// stopped detecting, so these read the detector directly.
func TestTheDealSeatDetectorSeesWhatItClaimsTo(t *testing.T) {
	t.Parallel()
	const bt = "`"
	probes := []struct {
		name  string
		fires bool
		src   string
	}{
		{"a seat read that ignores the End date", true,
			`func read() string { return ` + bt + `SELECT 1 FROM relationship r WHERE r.kind = 'deal_stakeholder' AND r.archived_at IS NULL` + bt + ` }`},
		{"a seat read that ends a seat on any End date", true,
			`func read() string { return ` + bt + `SELECT 1 FROM relationship r WHERE r.kind = 'deal_stakeholder' AND r.ended_at IS NULL` + bt + ` }`},
		{"the kind inside an IN list", true,
			`func read() string { return ` + bt + `SELECT 1 FROM relationship r WHERE r.kind IN ('works_with', 'deal_stakeholder')` + bt + ` }`},
		{"the kind inside a formatter's argument", true,
			`func read() string { return fmt.Sprintf(` + bt + `SELECT 1 FROM relationship r WHERE r.kind = 'deal_stakeholder' AND (%s)` + bt + `, scope) }`},
		{"the kind inside a map value", true,
			`var selectors = map[string]string{"x": ` + bt + `SELECT 1 FROM relationship r WHERE r.kind = 'deal_stakeholder'` + bt + `}`},
		{"the helper", false,
			`func read() string { return ` + bt + `SELECT 1 FROM relationship r WHERE ` + bt + ` + dealseat.CurrentSQL("r") }`},
		{"another kind", false,
			`func read() string { return ` + bt + `SELECT 1 FROM relationship r WHERE r.kind = 'project_stakeholder' AND r.ended_at IS NULL` + bt + ` }`},
	}
	head := "package probe\n\nimport (\n\t\"fmt\"\n\n\t\"" + dealSeatPackage + "\"\n)\n\n"
	scope := helperScope{qualifier: "dealseat", names: map[string]bool{dealSeatHelper: true}}
	for _, probe := range probes {
		t.Run(probe.name, func(t *testing.T) {
			t.Parallel()
			file, err := parser.ParseFile(token.NewFileSet(), "probe.go", head+probe.src, 0)
			if err != nil {
				t.Fatalf("the probe does not parse, so it proves nothing: %v", err)
			}
			hit := false
			for _, decl := range file.Decls {
				hit = hit || len(dealSeatStatements(decl, scope)) > 0
			}
			if hit != probe.fires {
				t.Errorf("detector answered %v, want %v, over:\n%s", hit, probe.fires, probe.src)
			}
		})
	}
}
