// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind claim H1

package gates

// dealseat.CurrentSQL is the one spelling of "this contact sits on the deal's
// buying committee". This census holds every hand-written statement that names
// the deal_stakeholder kind to the helper.
//
// A statement that names the kind is a finding, whether it tests ended_at or
// not. Leaving the End date out is the defect this gate is for. Some statements
// ask whether any edge row exists at all. Those are ratified below, each with
// its reason.
//
// The kind is read as a SQL literal, through a Go string constant of the
// module, or as a `$N` parameter bound to one. A constant of the generated
// contract is not read, because type-checking it costs most of the walk.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	dealSeatPackage = modulePath + "/internal/shared/kernel/dealseat"
	dealSeatOwner   = "internal/shared/kernel/dealseat/dealseat.go"
	dealSeatHelper  = "CurrentSQL"
	dealSeatValue   = "'deal_stakeholder'"
)

// dealSeatKind matches a statement scoped to deal stakeholder edges, in an
// equality or anywhere in an IN list. `'+` admits the doubled quote a constant
// rendered inside a quoted literal leaves.
var dealSeatKind = regexp.MustCompile(`kind\s*=\s*'+deal_stakeholder'|kind\s+IN\s*\((?:[^()]|\([^()]*\))*'+deal_stakeholder'`)

// dealSeatParam matches a kind bound as a parameter. The statement names the
// seat kind when the constant it is bound to renders beside it.
var dealSeatParam = regexp.MustCompile(`kind\s*=\s*\$\d+`)

// dealSeatDateBlind ratifies the statements that ask whether any edge row
// exists. A key is file, declaration and how many such statements it holds, so
// a second statement in a ratified declaration is judged on its own.
var dealSeatDateBlind = gatekit.Waive(map[string]string{
	"internal/compose/company360/roleproposalwrite.go:seatedNow#1":                   "a seat somebody ended is an answer too, and a model reading must not seat that contact on the deal again",
	"internal/compose/contact360/sections.go:dealRolesSection#1":                     "the contact page lists past seats as well and marks each one current or not through dealseat.CurrentSQL",
	"internal/compose/csvdealupdate.go:stakeholderToSeat#1":                          "the write slot under uq_rel_deal_contact_role, which holds an ended row too, so a date test would insert a duplicate",
	"internal/modules/contacts/demote.go:ensureNoLiveDeal#1":                         "demotion is blocked by any live edge row, because the edge would be stranded whatever its End date says",
	"internal/modules/contacts/promoteexistingdeal.go:seatQualifiedContact#1":        "the write slot under uq_rel_deal_contact_role, which holds an ended row too, so a date test would insert a duplicate",
	"internal/modules/deals/recordhandover.go:heldOnRecordSQL#1":                     "a handover moves every deal the contact has a live edge on, ended or not, so no record is left behind",
	"internal/modules/capture/purgeselect.go:SelectPurgeableContactsTx#1":            "a contact with any live deal edge is kept out of a capture purge, so the purge never strands a seat",
	"internal/modules/privacy/retentionselectors.go:retentionSelectors#1":            "retention keeps a contact with any live deal edge; erasing them would strand the edge whatever its End date",
	"internal/modules/privacy/erasure_legalreview.go:subjectDealIDsSQL#1":            "legal review lists every deal the contact ever sat on, archived edges included",
	"internal/modules/privacy/erasure_legalreview.go:subjectLiveDealIDsSQL#1":        "erasure review weighs every deal the contact has a live edge on, because erasing them would strand it",
	"internal/modules/privacy/erasure_legalreview.go:redactSubjectLegalReviewText#1": "redaction reaches every deal the subject ever sat on",
	"internal/modules/privacy/sarlegalreview.go:sarLegalReviewSection#1":             "a subject access report covers every deal the subject has a live edge on, ended or not",
	"internal/modules/privacy/sarsections.go:sarRecordSections#1":                    "a subject access report covers every deal the subject has a live edge on, ended or not",
	"internal/modules/privacy/reportingretention.go:reportingRetentionSelector#1":    "a legal hold reaches every deal the subject ever sat on, not only the current ones",
	"internal/modules/privacy/reportingquestions.go:heldQuestionSource#1":            "a legal hold reaches every deal the subject ever sat on, not only the current ones",
	"internal/modules/privacy/reportingquestions.go:subjectQuestionSources#1":        "a subject access report covers every deal the subject ever sat on",
	"internal/modules/privacy/sarreporting.go:sarReportingSections#1":                "a subject access report covers every deal the subject ever sat on",
	"internal/modules/privacy/reportingredaction.go:redactSubjectReporting#1":        "redaction reaches every deal the subject ever sat on",
})

func TestEveryDealSeatCurrencyTestUsesTheOneDefinition(t *testing.T) {
	t.Parallel()
	defer dealSeatDateBlind.AssertAllMatched(t)
	fset, files := parseGoFilesUnder(t, ".")
	infos := typeCheckModule(fset, files)
	var findings []string
	judged := 0
	for _, file := range files {
		path := filepath.ToSlash(fset.Position(file.Pos()).Filename)
		if path == dealSeatOwner || strings.HasSuffix(path, "_gen.go") {
			continue
		}
		scope := dealSeatScope(file, infos[file])
		for _, decl := range file.Decls {
			statements := dealSeatStatements(decl, scope)
			for _, sql := range statements {
				judged++
				subject := path + ":" + declName(decl) + "#" + strconv.Itoa(len(statements))
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
			"an ended seat off the committee. If the statement asks whether any edge row exists, "+
			"ratify it in dealSeatDateBlind with the reason.", strings.Join(findings, "\n  "))
	}
}

// typeCheckModule checks every package in import order, so a constant is read
// with its value wherever it is declared in the module.
//
// It is not typedModule: it also reads integration-tagged files and tools/,
// and an outside import stands in empty, so a contract constant stays unvalued.
func typeCheckModule(fset *token.FileSet, files []*ast.File) map[*ast.File]*types.Info {
	byDir := map[string][]*ast.File{}
	for _, file := range files {
		name := filepath.ToSlash(fset.Position(file.Pos()).Filename)
		if !strings.HasSuffix(name, "_gen.go") {
			byDir[filepath.Dir(name)] = append(byDir[filepath.Dir(name)], file)
		}
	}
	out := map[*ast.File]*types.Info{}
	checked := sharedImports{}
	for _, group := range packagesInImportOrder(byDir) {
		dir := filepath.ToSlash(filepath.Dir(fset.Position(group[0].Pos()).Filename))
		info := &types.Info{Uses: map[*ast.Ident]types.Object{}}
		conf := types.Config{Importer: checked, Error: func(error) {}}
		pkg, _ := conf.Check(modulePath+"/"+dir, fset, group, info)
		checked[pkg.Path()] = pkg
		for _, file := range group {
			out[file] = info
		}
	}
	return out
}

// dealSeatScope is the walk's view of one file: where the helper is, and the
// value of every string constant a statement names.
func dealSeatScope(file *ast.File, info *types.Info) helperScope {
	scope := helperScope{
		qualifier: importAliasOf(file, dealSeatPackage),
		inside:    file.Name != nil && file.Name.Name == "dealseat",
		names:     map[string]bool{dealSeatHelper: true},
	}
	if info != nil {
		scope.resolve = func(expr ast.Expr) (string, bool) {
			text := constantText(expr, info, nil)
			return "'" + text + "'", text != ""
		}
	}
	return scope
}

// dealSeatStatements returns the flattened statements in decl that name the
// deal_stakeholder kind.
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
		if ok && namesDealSeat(text) {
			out = append(out, text)
		}
		return true
	})
	return out
}

func namesDealSeat(sql string) bool {
	return dealSeatKind.MatchString(sql) || (dealSeatParam.MatchString(sql) && strings.Contains(sql, dealSeatValue))
}

func firstDealSeatLine(sql string) string {
	for line := range strings.SplitSeq(sql, "\n") {
		if namesDealSeat(line) {
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
		{
			"a seat read that ignores the End date", true,
			`func read() string { return ` + bt + `SELECT 1 FROM relationship r WHERE r.kind = 'deal_stakeholder' AND r.archived_at IS NULL` + bt + ` }`,
		},
		{
			"a seat read that ends a seat on any End date", true,
			`func read() string { return ` + bt + `SELECT 1 FROM relationship r WHERE r.kind = 'deal_stakeholder' AND r.ended_at IS NULL` + bt + ` }`,
		},
		{
			"the kind inside an IN list", true,
			`func read() string { return ` + bt + `SELECT 1 FROM relationship r WHERE r.kind IN ('works_with', 'deal_stakeholder')` + bt + ` }`,
		},
		{
			"the kind inside a formatter's argument", true,
			`func read() string { return fmt.Sprintf(` + bt + `SELECT 1 FROM relationship r WHERE r.kind = 'deal_stakeholder' AND (%s)` + bt + `, scope) }`,
		},
		{
			"the kind inside a map value", true,
			`var selectors = map[string]string{"x": ` + bt + `SELECT 1 FROM relationship r WHERE r.kind = 'deal_stakeholder'` + bt + `}`,
		},
		{
			"the kind through a constant", true,
			"const seatKind = \"deal_stakeholder\"\nfunc read() string { return " + bt + `SELECT 1 FROM relationship r WHERE r.kind = '` + bt + ` + seatKind + ` + bt + `'` + bt + ` }`,
		},
		{
			"the kind bound as a parameter", true,
			"const seatKind = \"deal_stakeholder\"\nfunc read(q func(string, ...any)) { q(" + bt + `SELECT 1 FROM relationship r WHERE r.kind = $1` + bt + `, seatKind) }`,
		},
		{
			"the helper", false,
			`func read() string { return ` + bt + `SELECT 1 FROM relationship r WHERE ` + bt + ` + dealseat.CurrentSQL("r") }`,
		},
		{
			"another kind", false,
			`func read() string { return ` + bt + `SELECT 1 FROM relationship r WHERE r.kind = 'project_stakeholder' AND r.ended_at IS NULL` + bt + ` }`,
		},
	}
	head := "package probe\n\nimport (\n\t\"fmt\"\n\n\t\"" + dealSeatPackage + "\"\n)\n\nvar _ = fmt.Sprint\nvar _ = dealseat.CurrentSQL\nvar scope string\n\n"
	for _, probe := range probes {
		t.Run(probe.name, func(t *testing.T) {
			t.Parallel()
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "probe.go", head+probe.src, 0)
			if err != nil {
				t.Fatalf("the probe does not parse, so it proves nothing: %v", err)
			}
			info, _ := typeCheckLoosely(fset, "probe", []*ast.File{file})
			scope := dealSeatScope(file, info)
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
