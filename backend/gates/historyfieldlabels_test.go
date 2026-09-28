// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every field name the History tab can print has a word for it.
//
// The record page renders one row per changed field, and the name it prints is
// a key out of the audit image the writer built. Two kinds of key reach it. A
// CONTRACT column — `amount_minor`, `owner_id` — is covered by
// frontend/src/screens/historyfieldlabels.test.ts, which derives its set from
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
// THE EVENT DOORS, not the imaged ones. AuditEvent and AuditEventWithEvidence
// are the writes with NO before-image — an occurrence recorded on a record —
// and a free-form key is what such a write has instead of a column. Audit and
// its two siblings record a field TRANSITION, which is the contract census's
// subject. The line is not watertight: RecordVatCheck writes the same map
// through Audit on a re-check and AuditEvent on a first one, and there are 65
// imaged sites whose images this walk cannot read at all — margince#5863 holds
// that half, with the measurement.
//
// What the line does NOT leave open is a key quietly migrating out of scope. A
// writer moved from an event door to an imaged one leaves its label here with
// nothing emitting it, and the stale-entry direction below fails on exactly
// that.

import (
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	historyLabelSource = "../frontend/src/screens/historyfieldlabels.ts"
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
	files := gatekit.Scope{Roots: []string{"internal"}, Subject: fileReachesAnAuditDoor}.Files(t)
	constantsByPackage := packageConstants(t, files)
	callsByPackage := map[string]callIndex{}
	for _, parsed := range files {
		dir := filepath.Dir(parsed.Path)
		if _, indexed := callsByPackage[dir]; !indexed {
			callsByPackage[dir] = callsWithin(t, dir)
		}
	}

	emitted := map[string]string{}
	for _, parsed := range files {
		consts := constantsByPackage[filepath.Dir(parsed.Path)]
		for _, site := range auditSitesIn(parsed) {
			if auditDoorsWithBeforeImage[site.door] {
				continue
			}
			action, known := site.action(consts)
			entity, entityKnown := resolveString(site.argAt(auditEntityTypeArg), consts)
			// An unresolvable verb or record type is READ AS PROJECTED rather
			// than skipped. Under-recognition is the one direction a census
			// must not have: a site this walk cannot place would otherwise
			// carry its keys past the map unasked, which is exactly how the
			// original gap survived.
			if (known && !actions[action]) || (entityKnown && !entities[entity]) {
				continue
			}
			for _, key := range site.imageKeys(t, consts, callsByPackage[filepath.Dir(parsed.Path)]) {
				if _, seen := emitted[key]; !seen {
					emitted[key] = site.key
				}
			}
		}
	}
	if len(emitted) < projectedKeyFloor {
		t.Fatalf("resolved only %d field name(s) across every projected audit image, expected at least %d — a walk this short would agree with any map",
			len(emitted), projectedKeyFloor)
	}
	contract, synthetic := historyLabelMaps(t)
	for _, finding := range labelFindings(emitted, contract, synthetic, func(key string) bool {
		return sharedHistoryWords.Waived(t, key)
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
	"email":       "the contact's address either way — the stranded-capture sweep names the field an edit names",
	"occurred_at": "when the thing happened, for an activity's column and for a qualifying event alike",
	"social":      "the contact's social profiles, whether a LinkedIn match or a human filled them in",
	"source":      "where the record came from, for an activity's column and for a withdrawal's press",
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
	"internal/modules/contacts/company_evidence_write.go:writeEvidence": "the image is the patch's own After(), assembled from the request " +
		"struct, and the entity is the SIDECAR being written — company_fact or company_profile_field, neither of which the projection " +
		"admits. The company's own history is written by the update beside this one",
})

// imageKeys reads the field names this site's audit images name. An image built
// where the walk cannot follow is a finding rather than an absence.
func (s auditSite) imageKeys(t *testing.T, consts map[string]string, callers callIndex) []string {
	t.Helper()
	var keys []string
	for _, arg := range s.imageArgs() {
		literals, resolved := s.resolveImage(arg, callers)
		if !resolved {
			if !runtimeAuditImages.Waived(t, s.key) {
				t.Errorf("%s: %s builds its audit image where this gate cannot read it, so the field names inside it are judged by nobody.\n"+
					"\tSpell the map at the call, or ratify it in runtimeAuditImages with what keeps its keys off the History tab.", s.key, s.door)
			}
			continue
		}
		for _, element := range allElements(literals) {
			pair, isPair := element.(*ast.KeyValueExpr)
			if !isPair {
				continue
			}
			if key, resolved := resolveString(pair.Key, consts); resolved {
				keys = append(keys, key)
			} else if !runtimeAuditImages.Waived(t, s.key) {
				t.Errorf("%s: %s names an audit field this gate cannot resolve to a string, so the History tab's word for it is judged by nobody.", s.key, s.door)
			}
		}
	}
	return keys
}

// resolveImage follows an image the call NAMES rather than spells, which is the
// ordinary shape once a door already takes five arguments. Two forms, and both
// are followed because a gate that read only the inline one would ratify the
// rest — and a ratified site carries whatever keys it likes past the map.
//
//   - a LOCAL assigned a map literal a few lines up;
//   - a PARAMETER of a small auditing helper, filled by its own callers. The
//     keys are then every caller's, unioned: each is a payload this site can
//     write.
//
// Anything else — a method's return, a value assembled across a package
// boundary — is reported rather than guessed at.
func (s auditSite) resolveImage(arg ast.Expr, callers callIndex) ([]*ast.CompositeLit, bool) {
	if literal, isLiteral := arg.(*ast.CompositeLit); isLiteral {
		return []*ast.CompositeLit{literal}, true
	}
	if literals, built := callers.returnedLiterals(arg); built {
		return literals, true
	}
	named, isNamed := arg.(*ast.Ident)
	if !isNamed || s.fn == nil {
		return nil, false
	}
	if local, found := s.localLiteral(named.Name); found {
		return []*ast.CompositeLit{local}, true
	}
	at, isParam := paramIndex(s.fn, named.Name)
	if !isParam {
		return nil, false
	}
	return callers.literalsAt(s.fn.Name.Name, at)
}

// localLiteral is the map literal this function assigns to a name. The LAST
// assignment wins, so a map declared empty and then replaced by a literal reads
// as the literal.
func (s auditSite) localLiteral(name string) (*ast.CompositeLit, bool) {
	var found *ast.CompositeLit
	ast.Inspect(s.fn.Body, func(n ast.Node) bool {
		assign, isAssign := n.(*ast.AssignStmt)
		if !isAssign {
			return true
		}
		for i, target := range assign.Lhs {
			ident, isIdent := target.(*ast.Ident)
			if !isIdent || ident.Name != name || i >= len(assign.Rhs) {
				continue
			}
			if literal, isLiteral := assign.Rhs[i].(*ast.CompositeLit); isLiteral {
				found = literal
			}
		}
		return true
	})
	return found, found != nil
}

// paramIndex is where a name sits in a function's parameter list, counting the
// grouped spelling (`before, after map[string]any`) as the two it declares.
func paramIndex(fn *ast.FuncDecl, name string) (int, bool) {
	at := 0
	for _, field := range fn.Type.Params.List {
		for _, ident := range field.Names {
			if ident.Name == name {
				return at, true
			}
			at++
		}
	}
	return 0, false
}

// callIndex is what a package's own functions are called with, and what they
// return, by name.
type callIndex struct {
	calls   map[string][]*ast.CallExpr
	returns map[string][]*ast.CompositeLit
}

// returnedLiterals reads an image a small builder returns — `handleImage(…)`,
// `reachabilityImage(…)`. The builders are one-line functions whose whole body
// is the literal, so following them costs nothing and buys the sites that would
// otherwise be ratified with a reason that amounted to "it is behind a helper".
func (c callIndex) returnedLiterals(arg ast.Expr) ([]*ast.CompositeLit, bool) {
	call, isCall := arg.(*ast.CallExpr)
	if !isCall {
		return nil, false
	}
	var name string
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		name = fn.Name
	case *ast.SelectorExpr:
		name = fn.Sel.Name
	default:
		return nil, false
	}
	literals := c.returns[name]
	return literals, len(literals) > 0
}

// literalsAt reads the map literals a function's callers pass at one position.
// A caller that names rather than spells its argument is reported, because the
// union would otherwise be short by exactly that caller's keys.
func (c callIndex) literalsAt(fn string, at int) ([]*ast.CompositeLit, bool) {
	calls := c.calls[fn]
	if len(calls) == 0 {
		return nil, false
	}
	var literals []*ast.CompositeLit
	for _, call := range calls {
		if at >= len(call.Args) {
			return nil, false
		}
		if isNilExpr(call.Args[at]) {
			continue
		}
		switch arg := call.Args[at].(type) {
		case *ast.CompositeLit:
			literals = append(literals, arg)
		default:
			built, isBuilt := c.returnedLiterals(arg)
			if !isBuilt {
				return nil, false
			}
			literals = append(literals, built...)
		}
	}
	return literals, true
}

// callsWithin indexes a package's calls to its own plain functions, so an
// auditing helper's parameters can be read from the sites that fill them.
//
// The whole DIRECTORY, for the reason packageConstants reads one: a helper's
// callers sit wherever the writes are, which is rarely the file that holds the
// audit door. Indexing only the swept files would find some of them and union a
// subset of the keys — a census short by exactly the callers it could not see.
func callsWithin(t *testing.T, dir string) callIndex {
	t.Helper()
	index := callIndex{calls: map[string][]*ast.CallExpr{}, returns: map[string][]*ast.CompositeLit{}}
	for _, file := range parsePackageDir(t, token.NewFileSet(), dir) {
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
func (s auditSite) imageArgs() []ast.Expr {
	after := auditBeforeArg
	var args []ast.Expr
	if auditDoorsWithBeforeImage[s.door] {
		after++
		if before := s.argAt(auditBeforeArg); !isNilExpr(before) {
			args = append(args, before)
		}
	}
	if image := s.argAt(after); !isNilExpr(image) {
		args = append(args, image)
	}
	return args
}

// allElements flattens the literals one image resolved to.
func allElements(literals []*ast.CompositeLit) []ast.Expr {
	var elements []ast.Expr
	for _, literal := range literals {
		elements = append(elements, literal.Elts...)
	}
	return elements
}

// isNilExpr answers the literal `nil`, which every door admits for an image
// that does not exist.
func isNilExpr(expr ast.Expr) bool {
	ident, isIdent := expr.(*ast.Ident)
	return expr == nil || (isIdent && ident.Name == "nil")
}

// fieldHistoryProjection reads the two closed sets that decide whether an audit
// row reaches the History tab at all, out of the file that owns them.
//
// Read rather than copied: both are unexported, and a gate holding its own
// spelling of them would stay green while the real projection narrowed
// underneath it.
func fieldHistoryProjection(t *testing.T) (actions, entities map[string]bool) {
	t.Helper()
	fset := token.NewFileSet()
	files := parsePackageDir(t, fset, "internal/modules/privacy")
	consts := stringConsts(t, fset, files)
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
		key, resolved := resolveString(pair.Key, consts)
		if !resolved {
			t.Fatalf("%s holds a key this gate cannot resolve to a string, so the set it decides is read short", where)
		}
		into[key] = true
	}
}

// tsLabelEntry reads one `["key", "message.key"]` pair out of a TypeScript Map,
// in either quote style, so an entry written the other way cannot vanish from a
// census. Comments are stripped first (tsComment): a line MENTIONING a key would
// otherwise keep a gate green after the real entry was deleted.
var tsLabelEntry = regexp.MustCompile(`\[\s*["']([a-z0-9_]+)["']\s*,`)

// historyLabelMaps reads both label maps out of the screen module.
func historyLabelMaps(t *testing.T) (contract, synthetic map[string]bool) {
	t.Helper()
	source, err := os.ReadFile(historyLabelSource)
	if err != nil {
		t.Fatalf("reading the history label maps: %v", err)
	}
	return tsMapKeys(t, historyLabelSource, string(source), contractLabelMap),
		tsMapKeys(t, historyLabelSource, string(source), syntheticLabelMap)
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
	for _, match := range tsLabelEntry.FindAllStringSubmatch(tsComment.ReplaceAllString(source[start:start+end], " "), -1) {
		keys[match[1]] = true
	}
	if len(keys) == 0 {
		t.Fatalf("%s's %s literal reads as empty, so every key would look unlabelled", path, marker)
	}
	return keys
}
