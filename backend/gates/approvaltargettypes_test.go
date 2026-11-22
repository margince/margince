// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// An approval's target type is the closed list approvals.TargetType declares, and
// enumsync_test.go holds the CHECK on approval.target_entity_type equal to it.
// Neither says whether a writer stages a type the list leaves out; that writer
// would fail its insert. This gate holds the entity vocabulary and the agent
// policy's record types inside the list, and names the discriminator columns
// that stay free text.

import (
	"bufio"
	"bytes"
	"go/ast"
	"go/token"
	"io/fs"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const approvalTargetTypes = "internal/modules/approvals.TargetType"

// stagerTypes are the struct types a staging is written through, whose
// TargetType and CoTargetType fields reach approval.target_entity_type.
var stagerTypes = map[string]bool{"StageInput": true, "StageInfo": true, "StageRequest": true}

// stagerCensusFloor sits just under the 16 distinct types the scan resolves. A
// scan that resolves fewer has stopped reading the tree.
const stagerCensusFloor = 12

// stringScan says where a vocabulary is written: the struct literals that carry
// it under a key, and the calls that take it as an argument.
type stringScan struct {
	literals map[string]bool
	keys     map[string]bool
	// calls maps a callee name to the position of the argument that holds the value.
	calls map[string]int
	// stageCalls also reads a target type constant passed to any call that stages.
	stageCalls bool
}

var stagerScan = stringScan{literals: stagerTypes, keys: map[string]bool{"TargetType": true, "CoTargetType": true}, stageCalls: true}

// stagedTypeValues reads every TargetType and CoTargetType set in a stager.
func stagedTypeValues(t *testing.T) (resolved map[string]string, unresolved int) {
	t.Helper()
	return scanStrings(t, stagerScan)
}

// scanStrings reads the values a scan's literals and calls hold: a string
// literal, a constant of the same package, or string(constant). A value held in
// a variable is counted apart.
func scanStrings(t *testing.T, scan stringScan) (resolved map[string]string, unresolved int) {
	t.Helper()
	byDir := map[string]map[string]map[string]bool{}
	global := map[string]map[string]bool{}
	type source struct {
		file    *ast.File
		path    string
		imports map[string]bool
	}
	var sources []source
	err := gatekit.WalkDir("internal", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") ||
			strings.Contains(p, "internal/contracts/") {
			return err
		}
		file, err := gatekit.ParseFile(p, 0)
		if err != nil {
			return err
		}
		dir := path.Dir(p)
		if byDir[dir] == nil {
			byDir[dir] = map[string]map[string]bool{}
		}
		recordStringConsts(byDir[dir], file)
		recordStringConsts(global, file)
		sources = append(sources, source{file, p, scanImportNames(file)})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	resolved = map[string]string{}
	for _, src := range sources {
		dir := path.Dir(src.path)
		ast.Inspect(src.file, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if scan.stageCalls {
					for _, value := range stagedCallArguments(call, byDir[dir]) {
						resolved[value] = src.path
					}
				}
				if at, ok := scan.calls[stagerLiteralTypeName(call.Fun)]; ok && at < len(call.Args) {
					values := resolveStringValue(call.Args[at], byDir[dir], global, src.imports)
					if len(values) == 0 {
						unresolved++
					}
					for _, value := range values {
						resolved[value] = src.path
					}
				}
				return true
			}
			lit, ok := n.(*ast.CompositeLit)
			if !ok || !scan.literals[stagerLiteralTypeName(lit.Type)] {
				return true
			}
			for _, elt := range lit.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, _ := kv.Key.(*ast.Ident)
				if key == nil || !scan.keys[key.Name] {
					continue
				}
				values := resolveStringValue(kv.Value, byDir[dir], global, src.imports)
				if len(values) == 0 {
					unresolved++
				}
				for _, value := range values {
					resolved[value] = src.path
				}
			}
			return true
		})
	}
	return resolved, unresolved
}

func stagedCallArguments(call *ast.CallExpr, local map[string]map[string]bool) []string {
	callee := ""
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		callee = fn.Name
	case *ast.SelectorExpr:
		callee = fn.Sel.Name
	}
	if !strings.Contains(strings.ToLower(callee), "stage") {
		return nil
	}
	var values []string
	for _, arg := range call.Args {
		id, ok := arg.(*ast.Ident)
		if !ok {
			continue
		}
		name := strings.ToLower(id.Name)
		if strings.HasSuffix(name, "targettype") || strings.HasSuffix(name, "recordtype") {
			values = append(values, slices.Sorted(maps.Keys(local[id.Name]))...)
		}
	}
	return values
}

func stagerLiteralTypeName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return e.Sel.Name
	}
	return ""
}

// recordStringConsts maps each constant name in file to the string literals it
// is declared with. One name may mean two values in one map.
func recordStringConsts(out map[string]map[string]bool, file *ast.File) {
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if i >= len(vs.Values) {
					continue
				}
				if value, ok := gatekit.StringExpr(vs.Values[i], nil, gatekit.FoldStrict); ok {
					if out[name.Name] == nil {
						out[name.Name] = map[string]bool{}
					}
					out[name.Name][value] = true
				}
			}
		}
	}
}

// scanImportNames are the package names a file can qualify an identifier with.
func scanImportNames(file *ast.File) map[string]bool {
	names := map[string]bool{}
	for _, spec := range file.Imports {
		if spec.Name != nil {
			names[spec.Name.Name] = true
			continue
		}
		names[path.Base(strings.Trim(spec.Path.Value, `"`))] = true
	}
	return names
}

// resolveStringValue reads an Ident from its own package. It reads a selector
// from the constant of that name, only when one package declares it.
func resolveStringValue(expr ast.Expr, local, global map[string]map[string]bool, imports map[string]bool) []string {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if value, ok := gatekit.StringExpr(e, nil, gatekit.FoldStrict); ok {
			return []string{value}
		}
	case *ast.Ident:
		return slices.Sorted(maps.Keys(local[e.Name]))
	case *ast.SelectorExpr:
		if pkg, qualified := e.X.(*ast.Ident); !qualified || !imports[pkg.Name] {
			return nil
		}
		if values := global[e.Sel.Name]; len(values) == 1 {
			return slices.Sorted(maps.Keys(values))
		}
	case *ast.CallExpr:
		if fn, ok := e.Fun.(*ast.Ident); ok && fn.Name == "string" && len(e.Args) == 1 {
			return resolveStringValue(e.Args[0], local, global, imports)
		}
	}
	return nil
}

func TestEveryApprovalStagerNamesAnApprovalTargetType(t *testing.T) {
	t.Parallel()
	_, typed := vocabularyCensusInputs(t)
	declared := typed[approvalTargetTypes]
	if len(declared) == 0 {
		t.Fatalf("no %s constants found: the derivation broke", approvalTargetTypes)
	}
	for _, source := range []string{"internal/shared/ports/datasource.EntityType", "internal/compose.agentRecordType"} {
		if len(typed[source]) == 0 {
			t.Fatalf("no %s constants found: the derivation broke", source)
		}
		for _, value := range typed[source] {
			if !slices.Contains(declared, value) {
				t.Errorf("%s holds %q, which approvals.TargetType does not. Staging it would fail the approval "+
					"CHECK. Add it to TargetType and to the CHECK together", source, value)
			}
		}
	}
	resolved, unresolved := stagedTypeValues(t)
	if len(resolved) < stagerCensusFloor {
		t.Fatalf("the scan resolved %d distinct staged types, floor %d: it has stopped reading the tree",
			len(resolved), stagerCensusFloor)
	}
	t.Logf("%d distinct staged types resolved, %d values held in variables", len(resolved), unresolved)
	for value, file := range resolved {
		if !slices.Contains(declared, value) && !extensionTable.MatchString(value) {
			t.Errorf("%s stages target type %q, which approvals.TargetType does not hold", file, value)
		}
	}
}

var extensionTable = regexp.MustCompile(`^ext_[a-z0-9]+(_[a-z0-9]+)*$`)

// freeTextDiscriminators are the polymorphic type columns that keep no CHECK.
// Each is a log of what a thing was about when it happened. A closed list would
// refuse the row on the day a new kind of thing appears.
var freeTextDiscriminators = gatekit.Waive(map[string]string{
	"audit_log.entity_type":        "the audit log records every entity type the product has ever audited, including ones since removed, and a refused insert would drop the audit row",
	"ai_call.subject_type":         "the AI call log records what a model call was about; the row is kept after the subject and its kind are gone",
	"ai_task_run.subject_type":     "the AI run log records what a run was about; the row is kept after the subject and its kind are gone",
	"webhook_delivery.entity_type": "the webhook log records what a delivery was about, for every event type a subscriber was ever sent",
	"webhook_delivery.event_type":  "the webhook log records the event name as sent; a new event type must not make its delivery row fail",
})

var catalogTextColumnLine = regexp.MustCompile(`^public\.([a-z_][a-z0-9_]*\.[a-z_][a-z0-9_]*) text\b`)

func TestAFreeTextDiscriminatorHasNoVocabularyCheck(t *testing.T) {
	t.Parallel()
	raw, err := gatekit.ReadFile(vocabularyCatalog)
	if err != nil {
		t.Fatal(err)
	}
	textColumns := map[string]bool{}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		if m := catalogTextColumnLine.FindStringSubmatch(scanner.Text()); m != nil {
			textColumns[m[1]] = true
		}
	}
	vocabularies, err := catalogVocabularies()
	if err != nil {
		t.Fatal(err)
	}
	for _, column := range freeTextDiscriminators.Subjects() {
		freeTextDiscriminators.Waived(t, column)
		if !textColumns[column] {
			t.Errorf("%s is not a text column in %s: remove its entry", column, vocabularyCatalog)
		}
		if _, hasCheck := vocabularies[column]; hasCheck {
			t.Errorf("%s now carries a vocabulary CHECK: remove its entry, and bind it in enumBindings", column)
		}
	}
	freeTextDiscriminators.AssertAllMatched(t)
}

// noticeTargetScan is where a notice's target type is written: the Target
// literal every notice carries.
var noticeTargetScan = stringScan{literals: map[string]bool{"Target": true}, keys: map[string]bool{"Type": true}}

// factSourceScan is where a metric names its source: the fact literal, and the
// readers every metric reads its rows through, which take the source fifth.
var factSourceScan = stringScan{
	literals: map[string]bool{"Fact": true},
	keys:     map[string]bool{"SourceType": true},
	calls:    map[string]int{"readGroupedFacts": 4, "readGroupedFactsVia": 4, "readOutcomeFacts": 4, "scanOutcomeFacts": 4, "readColleagueWork": 4},
}

const (
	noticeOwnTargetTypes  = "internal/modules/notices.OwnTargetType"
	reportingSourceTypes  = "internal/modules/reporting.SourceType"
	noticeCensusFloor     = 4
	factSourceCensusFloor = 5
	noticeTargetColumn    = "notice.target_type"
)

// A notice points at anything an approval can name, or at one of the three things
// only a notice has. The CHECK is held to that union, so neither list is typed a
// second time and a value dropped from either fails here.
func TestNoticeTargetTypeIsTheApprovalListAndTheNoticeOwn(t *testing.T) {
	t.Parallel()
	vocabularies, typed := vocabularyCensusInputs(t)
	want := asSet(append(slices.Clone(typed[approvalTargetTypes]), typed[noticeOwnTargetTypes]...))
	if len(typed[approvalTargetTypes]) == 0 || len(typed[noticeOwnTargetTypes]) == 0 {
		t.Fatalf("a source list is empty (%d approval, %d notice own): the derivation broke",
			len(typed[approvalTargetTypes]), len(typed[noticeOwnTargetTypes]))
	}
	got, ok := vocabularies[noticeTargetColumn]
	if !ok {
		t.Fatalf("no CHECK vocabulary found for %s", noticeTargetColumn)
	}
	if setKey(asSet(got)) != setKey(want) {
		t.Errorf("%s: the CHECK admits %v but approvals.TargetType plus notices.OwnTargetType hold %v. Change both together",
			noticeTargetColumn, got, want)
	}
	resolved, _ := scanStrings(t, noticeTargetScan)
	if len(resolved) < noticeCensusFloor {
		t.Fatalf("the scan resolved %d distinct notice target types, floor %d: it has stopped reading the tree",
			len(resolved), noticeCensusFloor)
	}
	for value, file := range resolved {
		if !slices.Contains(want, value) && !extensionTable.MatchString(value) {
			t.Errorf("%s writes notice target type %q, which the CHECK refuses", file, value)
		}
	}
}

func TestEveryMetricSourceNamesAReportingSourceType(t *testing.T) {
	t.Parallel()
	_, typed := vocabularyCensusInputs(t)
	declared := typed[reportingSourceTypes]
	if len(declared) == 0 {
		t.Fatalf("no %s constants found: the derivation broke", reportingSourceTypes)
	}
	resolved, _ := scanStrings(t, factSourceScan)
	if len(resolved) < factSourceCensusFloor {
		t.Fatalf("the scan resolved %d distinct metric sources, floor %d: it has stopped reading the tree",
			len(resolved), factSourceCensusFloor)
	}
	for value, file := range resolved {
		if !slices.Contains(declared, value) {
			t.Errorf("%s names metric source %q, which reporting.SourceType does not hold", file, value)
		}
	}
}
