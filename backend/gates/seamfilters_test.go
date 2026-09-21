// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// A filter the contract declares and the store can bind is OFFERED to an agent.
//
// list_records publishes the intersection of two vocabularies: what the
// contract's own list operation declares, and what the store behind the seam
// can narrow by. The dangerous direction is already guarded — an unbindable
// filter would run the list unnarrowed while looking narrowed, and
// TestOnlyAFilterBothTheContractAndAStoreCarryIsPublished refuses it.
//
// NOTHING GUARDED THE OTHER ONE. A filter both halves carry, which the seam
// simply never names, is capability nobody can reach: an agent asked "which of
// our contacts are tagged vip" enumerates every contact and discards rows
// against its listing budget, and the answer it returns is a subset nobody can
// size wearing the shape of a complete one.
//
// Three names sat in that gap — `tag` on a contact, `domain` on a company,
// `min_score` on a lead. The last had been there far longer than the others,
// which is the argument for a gate rather than for three edits: they were found
// by somebody looking, and a fourth will not be.
//
// WHY THE DECLARED SET IS ENOUGH to stand for "both halves". A filter the
// contract declares for a list operation is one that operation's handler binds
// to its store — TestEveryDeclaredNarrowingParameterIsReadByItsHandler is the
// obligation, and a parameter nothing reads is removed from the contract rather
// than left declared. So declared implies bindable-by-the-store, and what this
// gate has left to ask is whether the SEAM's vocabulary names it too.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// seamVocabularySources are the two halves, each read where it is written
// rather than restated here.
const (
	// recordShapesFile is generated from crm.yaml by gen-recordfields and
	// drift-gated, so reading it is reading the contract.
	recordShapesFile = "internal/modules/agents/recordshapes_gen.go"
	// datasourceDir holds the EntityType constants a ListFilters switch names.
	datasourceDir = "internal/shared/ports/datasource"
)

// filterSetModules are the packages that own a record type's list vocabulary.
// Each declares a ListFilters method whose switch maps an entity constant to
// one FilterSet, and the sets themselves live beside it.
//
// A directory list rather than a set of file names: which file a module keeps
// its sets in is the module's business, and naming files here would make this
// gate break on a rename that changed nothing about the vocabulary.
var filterSetModules = []string{
	"internal/modules/contacts",
	"internal/modules/deals",
	"internal/modules/projects",
}

// unofferedSeamFilters ratifies a filter both halves carry that the seam
// deliberately does NOT offer.
//
// Keyed `record_type.filter`. There will be one: a filter that is expensive, or
// whose answer would be a disclosure an agent should not be handed by
// enumeration. The escape exists so that case can be stated rather than left as
// a silence indistinguishable from the gap this gate closes.
var unofferedSeamFilters = gatekit.Waive(map[string]string{})

// deferredSeamFilters are the gaps this gate found on the day it landed: a
// filter both halves carry that the seam does not name, and that SHOULD be
// offered rather than refused.
//
// A separate set from the one above because they are different facts. Above is
// "an agent may not have this"; here is "an agent should have this and does
// not yet", and collapsing them would let a decision look like a backlog.
//
// WHY THEY ARE NOT SIMPLY ADDED HERE. Each one enters the runner's system
// prompt: list_records' input schema is rendered into it, so a filter name is
// model-visible text. Measured on this tree, offering all eleven moves the
// published catalog from 24,331 to 24,389 tokens — comfortably inside the
// headroom, and beside the point. What it also does is move the prompt, which
// takes one certification site's best record out of `current` and owes the
// paid lane that re-earns it. #828's ruling narrows that ticket to the gate for
// exactly this reason — "a gate is not a model-visible change and owes no paid
// lane" — so the gate lands and names what it found, and the offering rides the
// certification pass that can pay for it.
//
// Every one of these is an ordinary narrowing dial the store already binds on
// its HTTP surface, and none discloses a row an agent could not already reach
// by enumerating: narrowing returns a subset of what listing returns.
var deferredSeamFilters = gatekit.Waive(map[string]string{
	"contact.company_id":    "the employer narrowing; #828 offers it with the certification pass",
	"contact.owner_team_id": "the team arm of the ownership dial; #828",
	"contact.unassigned":    "the unowned-queue arm of the ownership dial; #828",
	"company.industry":      "free-text industry on the record; #828",
	"company.size_band":     "the contract's size enum; #828",
	"company.owner_team_id": "the team arm of the ownership dial; #828",
	"company.unassigned":    "the unowned-queue arm of the ownership dial; #828",
	"lead.source":           "where the lead came from; #828",
	"lead.sla_state":        "the first-response state (formulas §18); #828",
	"lead.owner_team_id":    "the team arm of the ownership dial; #828",
	"lead.unassigned":       "the unowned-queue arm of the ownership dial; #828",
})

func TestEveryDeclaredFilterTheStoreBindsIsOfferedToTheSeam(t *testing.T) {
	t.Parallel()
	declared := declaredRecordFilters(t)
	offered := seamFilterVocabulary(t)

	for _, recordType := range sortedKeysOf(declared) {
		names, listed := offered[recordType]
		if !listed {
			t.Errorf("the contract declares filters for %q and no module's ListFilters names that record type — "+
				"the seam publishes none of them, which is the whole gap this gate is about", recordType)
			continue
		}
		for _, filter := range declared[recordType] {
			if slices.Contains(names, filter) {
				continue
			}
			subject := recordType + "." + filter
			if unofferedSeamFilters.Waived(t, subject) || deferredSeamFilters.Waived(t, subject) {
				continue
			}
			t.Errorf("%s declares the %q filter and the seam does not offer it — an agent asked to narrow by it "+
				"enumerates instead, and returns a subset nobody can size.\n"+
				"  Add it to that record type's FilterSet, or ratify it in unofferedSeamFilters with the reason "+
				"an agent may not have it.\n"+
				"  The seam offers: %v", recordType, filter, names)
		}
	}
	unofferedSeamFilters.AssertAllMatched(t)
	// A deferred entry that stopped being a gap is a stale claim about the
	// seam: it would read as "still owed" beside a filter the seam now offers.
	deferredSeamFilters.AssertAllMatched(t)
	t.Logf("seam filters: %d record type(s) judged, %d deferred, %d ratified",
		len(declared), len(deferredSeamFilters.Subjects()), len(unofferedSeamFilters.Subjects()))
}

// declaredRecordFilters reads the contract half: listRecordFilters, generated
// from crm.yaml, as record type to filter names.
func declaredRecordFilters(t *testing.T) map[string][]string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), recordShapesFile, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", recordShapesFile, err)
	}
	out := map[string][]string{}
	for _, decl := range file.Decls {
		general, isGeneral := decl.(*ast.GenDecl)
		if !isGeneral || general.Tok != token.VAR {
			continue
		}
		for _, spec := range general.Specs {
			value, isValue := spec.(*ast.ValueSpec)
			if !isValue || len(value.Names) != 1 || value.Names[0].Name != "listRecordFilters" {
				continue
			}
			readRecordFilterMap(t, value, out)
		}
	}
	if len(out) == 0 {
		t.Fatalf("%s holds no listRecordFilters map this gate can read — the contract half is generated, so "+
			"either the generator's shape moved or this reading has gone blind, and a blind reading passes",
			recordShapesFile)
	}
	return out
}

// readRecordFilterMap reads the map literal's record types and the Name field
// of each filter under them.
func readRecordFilterMap(t *testing.T, value *ast.ValueSpec, into map[string][]string) {
	t.Helper()
	literal, isLiteral := value.Values[0].(*ast.CompositeLit)
	if !isLiteral {
		t.Fatalf("listRecordFilters is not a composite literal; this gate reads it as one")
	}
	for _, element := range literal.Elts {
		pair, isPair := element.(*ast.KeyValueExpr)
		if !isPair {
			continue
		}
		recordType := gatekit.TextOf(pair.Key)
		filters, isFilters := pair.Value.(*ast.CompositeLit)
		if recordType == "" || !isFilters {
			continue
		}
		for _, entry := range filters.Elts {
			if name := filterEntryName(entry); name != "" {
				into[recordType] = append(into[recordType], name)
			}
		}
	}
}

// filterEntryName reads one `{Name: "…", Type: "…"}` entry's declared name.
func filterEntryName(entry ast.Expr) string {
	fields, isFields := entry.(*ast.CompositeLit)
	if !isFields {
		return ""
	}
	for _, field := range fields.Elts {
		pair, isPair := field.(*ast.KeyValueExpr)
		if !isPair {
			continue
		}
		if key, isIdent := pair.Key.(*ast.Ident); isIdent && key.Name == "Name" {
			return gatekit.TextOf(pair.Value)
		}
	}
	return ""
}

// seamFilterVocabulary reads the store half: each module's ListFilters switch,
// resolved through the entity constants it names and the FilterSet it returns.
func seamFilterVocabulary(t *testing.T) map[string][]string {
	t.Helper()
	entities := gatekit.PackageStringConstants(t, datasourceDir)
	out := map[string][]string{}
	for _, dir := range filterSetModules {
		constants := gatekit.PackageStringConstants(t, dir)
		sets := filterSetKeys(t, dir, constants)
		for entity, setName := range listFilterCases(t, dir) {
			recordType, known := entities[entity]
			if !known {
				t.Errorf("%s's ListFilters names %s, which is no EntityType constant — the mapping this gate "+
					"reads is broken rather than empty", dir, entity)
				continue
			}
			keys, found := sets[setName]
			if !found {
				t.Errorf("%s's ListFilters returns %s for %s and this gate found no such FilterSet — "+
					"a vocabulary it cannot read reads as an empty one", dir, setName, recordType)
				continue
			}
			out[recordType] = append(out[recordType], keys...)
		}
	}
	if len(out) == 0 {
		t.Fatal("no module's ListFilters could be read — every record type would then look unoffered, which is " +
			"a failure of this gate rather than of the tree")
	}
	return out
}

// listFilterCases maps an EntityType constant name to the FilterSet a module's
// ListFilters returns for it.
//
// TWO SHAPES, because the tree writes both and a reader that knew one would
// report the other's record types as offering nothing — which reads exactly
// like the gap this gate exists to find. A module owning several types writes a
// switch; one owning a single type writes an early return and then answers its
// one set. A ListFilters this cannot read at all is an ERROR rather than an
// empty answer, for the same reason.
func listFilterCases(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, file := range parsedSources(t, dir) {
		for _, decl := range file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || fn.Name.Name != "ListFilters" || fn.Body == nil {
				continue
			}
			before := len(out)
			readListFilterSwitch(fn.Body, out)
			if len(out) == before {
				readSingleTypeListFilters(t, dir, fn, out)
			}
		}
	}
	return out
}

// readSingleTypeListFilters pairs the one entity a single-type ListFilters
// names with the one FilterSet it returns.
func readSingleTypeListFilters(t *testing.T, dir string, fn *ast.FuncDecl, into map[string]string) {
	t.Helper()
	entities, sets := namedEntitiesAndSets(fn.Body)
	if len(entities) == 1 && len(sets) == 1 {
		into[entities[0]] = sets[0]
		return
	}
	t.Errorf("%s's ListFilters names %v and returns %v — this gate reads a switch over entity types or a "+
		"single-type function, and a shape it cannot read reports every record type behind it as offering "+
		"nothing, which is indistinguishable from the gap it is looking for", dir, entities, sets)
}

// namedEntitiesAndSets collects the EntityType constants and FilterSet
// variables one function body mentions, each once and in source order.
func namedEntitiesAndSets(body *ast.BlockStmt) (entities, sets []string) {
	ast.Inspect(body, func(n ast.Node) bool {
		node, isSelector := n.(*ast.SelectorExpr)
		if !isSelector {
			return true
		}
		if name := node.Sel.Name; strings.HasPrefix(name, "Entity") && !slices.Contains(entities, name) {
			entities = append(entities, name)
		}
		if node.Sel.Name == "Names" {
			if set, isIdent := node.X.(*ast.Ident); isIdent && !slices.Contains(sets, set.Name) {
				sets = append(sets, set.Name)
			}
		}
		return true
	})
	return entities, sets
}

// readListFilterSwitch reads `case datasource.EntityX: return xListFilters.Names()`.
func readListFilterSwitch(body *ast.BlockStmt, into map[string]string) {
	ast.Inspect(body, func(n ast.Node) bool {
		clause, isClause := n.(*ast.CaseClause)
		if !isClause {
			return true
		}
		set := returnedFilterSet(clause.Body)
		if set == "" {
			return true
		}
		for _, match := range clause.List {
			if entity := selectorName(match); entity != "" {
				into[entity] = set
			}
		}
		return true
	})
}

// returnedFilterSet is the variable behind a `return xListFilters.Names()`.
func returnedFilterSet(body []ast.Stmt) string {
	for _, stmt := range body {
		ret, isReturn := stmt.(*ast.ReturnStmt)
		if !isReturn || len(ret.Results) != 1 {
			continue
		}
		call, isCall := ret.Results[0].(*ast.CallExpr)
		if !isCall {
			continue
		}
		selector, isSelector := call.Fun.(*ast.SelectorExpr)
		if !isSelector || selector.Sel.Name != "Names" {
			continue
		}
		if set, isIdent := selector.X.(*ast.Ident); isIdent {
			return set.Name
		}
	}
	return ""
}

// selectorName is the identifier after the dot: datasource.EntityContact →
// EntityContact.
func selectorName(expr ast.Expr) string {
	selector, isSelector := expr.(*ast.SelectorExpr)
	if !isSelector {
		return ""
	}
	return selector.Sel.Name
}

// filterSetKeys reads each FilterSet literal's keys, resolved through the
// package's own string constants. The keys are named constants rather than
// literals, so the wire name a caller types lives beside the rest of its
// module's vocabulary instead of being retyped per table.
func filterSetKeys(t *testing.T, dir string, constants map[string]string) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	for _, file := range parsedSources(t, dir) {
		for _, decl := range file.Decls {
			general, isGeneral := decl.(*ast.GenDecl)
			if !isGeneral || general.Tok != token.VAR {
				continue
			}
			for _, spec := range general.Specs {
				value, isValue := spec.(*ast.ValueSpec)
				if !isValue || len(value.Names) != 1 || len(value.Values) != 1 {
					continue
				}
				if keys := filterSetLiteralKeys(t, dir, value, constants); keys != nil {
					out[value.Names[0].Name] = keys
				}
			}
		}
	}
	return out
}

// filterSetLiteralKeys answers the wire names a FilterSet literal is keyed by,
// or nil when the declaration is not one.
func filterSetLiteralKeys(t *testing.T, dir string, value *ast.ValueSpec, constants map[string]string) []string {
	t.Helper()
	literal, isLiteral := value.Values[0].(*ast.CompositeLit)
	if !isLiteral || !isFilterSetType(literal.Type) {
		return nil
	}
	keys := []string{}
	for _, element := range literal.Elts {
		pair, isPair := element.(*ast.KeyValueExpr)
		if !isPair {
			continue
		}
		name, isIdent := pair.Key.(*ast.Ident)
		if !isIdent {
			t.Errorf("%s's %s is keyed by something this gate cannot resolve to a wire name",
				dir, value.Names[0].Name)
			continue
		}
		wire, known := constants[name.Name]
		if !known {
			t.Errorf("%s's %s names the filter constant %s, whose value this gate could not read — an unread "+
				"key reads as a filter the seam does not offer", dir, value.Names[0].Name, name.Name)
			continue
		}
		keys = append(keys, wire)
	}
	return keys
}

// isFilterSetType reports whether a composite literal's type is
// storekit.FilterSet[...].
func isFilterSetType(expr ast.Expr) bool {
	index, isIndex := expr.(*ast.IndexExpr)
	if !isIndex {
		return false
	}
	return selectorName(index.X) == "FilterSet"
}

func parsedSources(t *testing.T, dir string) []*ast.File {
	t.Helper()
	sources, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("listing %s: %v", dir, err)
	}
	var out []*ast.File
	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), source, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", source, err)
		}
		out = append(out, file)
	}
	if len(out) == 0 {
		t.Fatalf("%s holds no source this gate could read", dir)
	}
	return out
}

func sortedKeysOf[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
