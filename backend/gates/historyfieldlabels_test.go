// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every field name the History tab can print has a word for it.
//
// The record page renders one row per changed field, and the name it prints is
// a key out of the audit image the writer built. Two kinds of key reach it. A
// CONTRACT column — `amount_minor`, `owner_id` — is covered by
// frontend/src/lib/historyfieldlabels.test.ts, which derives its set from
// what an `Update<Type>Request` writes. A HAND-BUILT image is not: those keys
// are Go string literals, the contract has never heard of them, and a census on
// the other side of the wire cannot see them by construction.
//
// That is why historyfieldlabels.ts carries a second map, and until this gate
// landed nothing held it to anything. Its own doc comment named "the three
// consent-module writers" while that package held five; a key added to a new
// writer rendered raw as its own column name until somebody noticed; a key
// whose writer was deleted looked like coverage and answered nothing.
//
// It walks the same audit doors auditbeforeimage_test.go does, for the reason
// that gate states about its own corpus: a second way of finding a write is a
// second thing to keep current, and the copy that stopped finding them reads
// green over whatever it no longer sees.
//

import (
	"fmt"
	"go/ast"
	"go/types"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// mergeAuditVerb is the audit action a merge writes.
const mergeAuditVerb = "merge"

const (
	historyLabelSource = "../frontend/src/lib/historyfieldlabels.ts"
	syntheticLabelMap  = "const SYNTHETIC_AUDIT_FIELD_LABELS = new Map<string, MessageKey>(["
	contractLabelMap   = "const HISTORY_FIELD_LABELS = new Map<string, MessageKey>(["
	// projectedKeyFloor guards against a vacuous pass. Roughly fifty distinct
	// keys reach the projection today; the floor sits low enough that retiring a
	// writer does not drag it along, and high enough that a walk that resolved
	// nothing is reported rather than read as a tree with a word for every field.
	projectedKeyFloor = 30
)

// TestEveryProjectedAuditKeyHasALabel holds the two label maps to the writers,
// in both directions.
func TestEveryProjectedAuditKeyHasALabel(t *testing.T) {
	t.Parallel()
	defer runtimeAuditImages.AssertAllMatched(t)
	defer sharedHistoryWords.AssertAllMatched(t)

	actions, entities := fieldHistoryProjection(t)
	evidenceFields, reachEntities := historyEvidenceFields(t), historyReachEntities(t)
	callsByPackage := map[string]callIndex{}
	emitted := map[string]string{}
	needsRatification := map[string]bool{}
	for _, site := range auditDoorSitesOf(t) {
		verbs, known := site.verbs()
		// The record's history prints a merge's images beside an update's
		// fields, on the archived record and on the survivor.
		readAsALine := known && slices.Contains(verbs, mergeAuditVerb)
		types, typeKnown := site.values(site.door.entity)
		if strings.HasSuffix(site.use.pkg.path, "/platform/settings") {
			types, typeKnown = historySettingObjects(t), true
		}
		if !typeKnown {
			types, typeKnown = (historyImageReader{index: site.index, evidenceFields: evidenceFields, reachEntities: reachEntities}).strings(historyImageArg{pkg: site.use.pkg, decl: site.use.decl}, site.argAt(site.door.entity), 0)
		}
		// An unread verb or record type is read as projected rather than
		// skipped, so a site this walk cannot place still has its keys judged.
		if (known && !anyProjected(verbs, actions) && !readAsALine) || (typeKnown && !anyProjected(types, entities)) {
			continue
		}
		calls, indexed := callsByPackage[site.use.pkg.path]
		if !indexed {
			calls = callsWithin(site.use.pkg)
			callsByPackage[site.use.pkg.path] = calls
		}
		for _, key := range site.imageKeys(t, calls) {
			if site.door.before < 0 || readAsALine {
				needsRatification[key] = true
			}
			if _, seen := emitted[key]; !seen {
				emitted[key] = site.key()
			}
		}
	}
	if len(emitted) < projectedKeyFloor {
		t.Fatalf("resolved only %d field name(s) across every projected audit image, expected at least %d — a walk this short would agree with any map",
			len(emitted), projectedKeyFloor)
	}
	contract, synthetic := historyLabelMaps(t)
	for _, finding := range labelFindings(emitted, contract, synthetic, func(key string) bool {
		return !needsRatification[key] || sharedHistoryWords.Waived(t, key)
	}) {
		t.Error(finding)
	}
}

// labelFindings reports a key with no word, a word no writer emits, and a
// hand-built key the contract map already claims. It returns rather than fails,
// so historyfieldlabelsfalsify_test.go can plant each of the three.
func labelFindings(emitted map[string]string, contract, synthetic map[string]bool, ratified func(string) bool) []string {
	var findings []string
	for _, key := range sortedKeysOf(emitted) {
		switch {
		case contract[key]:
			// The contract map answers first, so a writer naming a contract
			// column shows that column's own word. Safe only while the two MEAN
			// the same thing to somebody reading one record's history, which is
			// a judgement no parser makes — so it is made once, per key.
			if !ratified(key) {
				findings = append(findings, fmt.Sprintf(
					"%s writes %q, a name the contract-derived map already claims, so the reader is shown the COLUMN's word for this writer's field.\n"+
						"\tRatify it in sharedHistoryWords with why the two read the same, or give the writer a key of its own.", emitted[key], key))
			}
		case !synthetic[key]:
			findings = append(findings, fmt.Sprintf(
				"%s writes %q into an audit image the History tab projects, and no map names it — the reader is shown %q.\n"+
					"\tAdd it to SYNTHETIC_AUDIT_FIELD_LABELS in %s, with an i18n key beside it.",
				emitted[key], key, strings.ReplaceAll(key, "_", " "), historyLabelSource))
		}
	}
	for _, key := range sortedKeysOf(synthetic) {
		if emitted[key] == "" {
			findings = append(findings, fmt.Sprintf(
				"SYNTHETIC_AUDIT_FIELD_LABELS names %q and no projected audit image carries it — a stale entry looks like coverage and answers nothing. Delete it.", key))
		}
	}
	return findings
}

// sharedHistoryWords ratifies each hand-built key that a contract column
// already claims.
var sharedHistoryWords = gatekit.Waive(map[string]string{
	"phones":              "the contact's complete phone array, assembled by recordContactSetChange in the same shape as the contract field",
	"address_city":        "the merge and name-fill writers name the same stored contact column that the record editor and History label name",
	"address_country":     "the merge and name-fill writers name the same stored contact column that the record editor and History label name",
	"address_line1":       "the merge and name-fill writers name the same stored contact column that the record editor and History label name",
	"address_line2":       "the merge and name-fill writers name the same stored contact column that the record editor and History label name",
	"address_postal_code": "the merge and name-fill writers name the same stored contact column that the record editor and History label name",
	"address_region":      "the merge and name-fill writers name the same stored contact column that the record editor and History label name",
	"first_name":          "the merge and name-fill writers name the same stored contact column that the record editor and History label name",
	"full_name":           "the merge and name-fill writers name the same stored contact column that the record editor and History label name",
	"greeting_register":   "the merge and name-fill writers name the same stored contact column that the record editor and History label name",
	"last_name":           "the merge and name-fill writers name the same stored contact column that the record editor and History label name",
	"salutation":          "the merge and name-fill writers name the same stored contact column that the record editor and History label name",
	"title":               "the merge and name-fill writers name the same stored contact column that the record editor and History label name",
	"company_id":          "the confirmed company link on a lead names the same company that the record form and its history label name",
	"email":               "the contact's address either way — the stranded-capture sweep names the field an edit names",
	"occurred_at":         "when the thing happened, for an activity's column and for a qualifying event alike",
	"social":              "the contact's social profiles, whether a LinkedIn match or a human filled them in",
	"source":              "where the record came from, for an activity's column and for a withdrawal's press",
})

// runtimeAuditImages ratifies a site whose image this walk cannot read: the map
// is built somewhere it cannot follow, so the keys inside it are unknown.
//
// Each entry states what keeps those keys out of the projection, or names the
// contract columns they are. An unratified one is a finding, because a site
// nobody can read is a site that carries whatever it likes past the map.
var runtimeAuditImages = gatekit.Waive(map[string]string{
	"internal/compose/extledger.go:recordExtensionChange": "an extension unit's own change, entity and image both handed over at run time. " +
		"It cannot reach the History tab whatever it holds: extension.Change.Entity must be a table in the INVOKING unit's namespace " +
		"(`ext_<namespace>_<table>`), which is never one of the six core record types fieldHistoryEntityTypes projects",
})

func anyProjected(values []string, projected map[string]bool) bool {
	return slices.ContainsFunc(values, func(value string) bool { return projected[value] })
}

// imageKeys reads the field names this site's audit images name. An image built
// where the walk cannot follow is a finding rather than an absence.
func (s auditDoorSite) imageKeys(t *testing.T, _ callIndex) []string {
	t.Helper()
	var keys []string
	reader := historyImageReader{index: s.index, evidenceFields: historyEvidenceFields(t), reachEntities: historyReachEntities(t)}
	if strings.HasSuffix(s.use.pkg.path, "/platform/settings") {
		reader.settingKeys = historyProjectedSettingsKeys(t)
	}
	for _, arg := range s.imageArgs() {
		read, ok := reader.keys(historyImageArg{expr: arg, pkg: s.use.pkg, decl: s.use.decl}, 0)
		if !ok && !runtimeAuditImages.Waived(t, s.key()) {
			t.Errorf("%s: %s builds an image this gate cannot follow; extend the image reader so its field labels are checked", s.key(), s.door.name)
		}
		keys = append(keys, read...)
	}
	return keys
}

// callIndex is what a package's own functions are called with, and what they
// return, by name.
type callIndex struct {
	calls   map[string][]*ast.CallExpr
	returns map[string][]*ast.CompositeLit
	info    *types.Info
}

// callsWithin indexes a package's calls to its own plain functions, so an
// auditing helper's parameters can be read from the sites that fill them.
//
// The whole package, because a helper's callers sit wherever the writes are,
// which is rarely the file that holds the audit door.
func callsWithin(pkg *typedPackage) callIndex {
	index := callIndex{calls: map[string][]*ast.CallExpr{}, returns: map[string][]*ast.CompositeLit{}, info: pkg.info}
	for _, file := range pkg.files {
		ast.Inspect(file, func(n ast.Node) bool {
			if call, isCall := n.(*ast.CallExpr); isCall {
				if name, isPlain := call.Fun.(*ast.Ident); isPlain {
					index.calls[name.Name] = append(index.calls[name.Name], call)
				}
				return true
			}
			fn, isFunc := n.(*ast.FuncDecl)
			if !isFunc || fn.Body == nil {
				return true
			}
			index.returns[fn.Name.Name] = append(index.returns[fn.Name.Name], returnedComposites(fn.Body)...)
			return true
		})
	}
	return index
}

// returnedComposites collects the map literals a function body returns.
func returnedComposites(body *ast.BlockStmt) []*ast.CompositeLit {
	var literals []*ast.CompositeLit
	ast.Inspect(body, func(n ast.Node) bool {
		ret, isReturn := n.(*ast.ReturnStmt)
		if !isReturn {
			return true
		}
		for _, result := range ret.Results {
			if literal, isLiteral := result.(*ast.CompositeLit); isLiteral {
				literals = append(literals, literal)
			}
		}
		return true
	})
	return literals
}

// imageArgs are the before and after images this door carries. `nil` is a real
// answer for a before-image and carries no keys.
func (s auditDoorSite) imageArgs() []ast.Expr {
	var args []ast.Expr
	for _, at := range []int{s.door.before, s.door.after} {
		if image := s.argAt(at); at >= 0 && !isNilExpr(s.use.pkg.info, image) {
			args = append(args, image)
		}
	}
	return args
}

// isNilExpr answers the untyped nil, which every door admits for an image
// that does not exist.
func isNilExpr(info *types.Info, expr ast.Expr) bool {
	return expr == nil || info.Types[expr].IsNil()
}

// fieldHistoryProjection reads the two closed sets that decide whether an audit
// row reaches the History tab at all, out of the file that owns them.
//
// Read rather than copied: both are unexported, and a gate holding its own
// spelling of them would stay green while the real projection narrowed
// underneath it.
func fieldHistoryProjection(t *testing.T) (actions, entities map[string]bool) {
	t.Helper()
	fset := gatekit.SourceFileSet()
	files := parsePackageDir(t, fset, "internal/modules/privacy")
	consts := gatekit.StringConstantsOf(t, files)
	return projectionSetKeys(t, files, consts, "fieldHistoryProjectedActions"),
		projectionSetKeys(t, files, consts, "fieldHistoryEntityTypes")
}

// projectionSetKeys reads one package-level `map[string]bool{…}` variable's
// keys, resolving a named constant. publicevents_test.go's mapLiteralKeys reads
// the same shape and cannot serve here: it takes one file and refuses anything
// but a string literal, and this set is spelled across a package with two of
// its entries written as constants.
func projectionSetKeys(t *testing.T, files []*ast.File, consts map[string]string, name string) map[string]bool {
	t.Helper()
	keys := map[string]bool{}
	for _, file := range files {
		for _, decl := range file.Decls {
			gen, isGen := decl.(*ast.GenDecl)
			if !isGen {
				continue
			}
			for _, spec := range gen.Specs {
				value, isValue := spec.(*ast.ValueSpec)
				if !isValue || len(value.Names) != 1 || value.Names[0].Name != name || len(value.Values) != 1 {
					continue
				}
				readProjectionKeys(t, value.Values[0], consts, name, keys)
			}
		}
	}
	if len(keys) == 0 {
		t.Fatalf("privacy declares no %s this gate can read — what the History tab projects is now judged by nobody", name)
	}
	return keys
}

// readProjectionKeys collects one map literal's keys, resolving a named constant.
func readProjectionKeys(t *testing.T, expr ast.Expr, consts map[string]string, where string, into map[string]bool) {
	t.Helper()
	literal, isLiteral := expr.(*ast.CompositeLit)
	if !isLiteral {
		t.Fatalf("%s is no longer a map literal this gate can read", where)
	}
	for _, element := range literal.Elts {
		pair, isPair := element.(*ast.KeyValueExpr)
		if !isPair {
			t.Fatalf("%s holds an entry with no key", where)
		}
		key, resolved := gatekit.StringExpr(pair.Key, consts, gatekit.FoldStrict)
		if !resolved {
			t.Fatalf("%s holds a key this gate cannot resolve to a string, so the set it decides is read short", where)
		}
		into[key] = true
	}
}

// tsLabelEntry reads one `["key", "message.key"]` pair out of a TypeScript Map,
// in either quote style, so an entry written the other way cannot vanish from a
// census. Comments are stripped first (gatekit.TSWithoutComments): a line MENTIONING a
// key would otherwise keep a gate green after the real entry was deleted.
var tsLabelEntry = regexp.MustCompile(`\[\s*["']([A-Za-z0-9_.]+)["']\s*,`)

// historyLabelMaps reads both label maps out of the screen module.
func historyLabelMaps(t *testing.T) (contract, synthetic map[string]bool) {
	t.Helper()
	source, err := gatekit.ReadFile(historyLabelSource)
	if err != nil {
		t.Fatalf("reading the history label maps: %v", err)
	}
	contract = tsMapKeys(t, historyLabelSource, string(source), contractLabelMap)
	for key := range historyAuxiliaryLabels(t, string(source)) {
		contract[key] = true
	}
	return contract, tsMapKeys(t, historyLabelSource, string(source), syntheticLabelMap)
}

// tsMapKeys reads the keys of the Map literal opened by marker in path's source.
func tsMapKeys(t *testing.T, path, source, marker string) map[string]bool {
	t.Helper()
	start := indexAfter(source, marker)
	if start < 0 {
		t.Fatalf("%s no longer declares %s — this gate is reading a shape that is gone", path, marker)
	}
	end := indexAfter(source[start:], "]);")
	if end < 0 {
		t.Fatalf("%s's %s literal is unterminated", path, marker)
	}
	keys := map[string]bool{}
	for _, match := range tsLabelEntry.FindAllStringSubmatch(gatekit.TSWithoutComments(source[start:start+end]), -1) {
		keys[match[1]] = true
	}
	if len(keys) == 0 {
		t.Fatalf("%s's %s literal reads as empty, so every key would look unlabelled", path, marker)
	}
	return keys
}
