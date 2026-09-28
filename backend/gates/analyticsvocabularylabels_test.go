// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The question builder names exactly the analytics vocabulary the server serves.
//
// The vocabulary is derived from the report catalog, so a field added to a spec
// reaches the builder the day it ships. Without its catalog key it renders as a
// wire name beside translated neighbours; a key for a field that is gone is one
// the builder offers and the engine refuses. The same holds for the populations,
// and the measures shown in a row's own currency. The aggregates and comparisons
// the browser types from the contract's enums, so those are held against the
// contract, and the compiler holds the browser's exhaustive label maps to them.
//
// Each server side is derived from its Go owner, the vocabulary from what a seat
// holding every grant is handed, and each is held in both directions. The
// TypeScript is read with regexps, the trade frontendminorunits_test.go makes.

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	analyticsCatalog     = "../frontend/src/i18n/en.ts"
	analyticsVocabMirror = "../frontend/src/screens/analytics.questions.vocab.ts"
)

// analyticsFieldLabel reads one field's catalog key. Anchored at a line's start,
// so a comment quoting a key is not read as the key.
var analyticsFieldLabel = regexp.MustCompile(`(?m)^\s*"analytics\.field\.([^"]+)"\s*:`)

// tsLiteralKey reads an object literal's keys, bare or quoted, and not the
// quoted values beside them, which are followed by a comma rather than a colon.
var tsLiteralKey = regexp.MustCompile(`(?:^|[{,\n])\s*["']?([a-z][a-z0-9_-]*)["']?\s*:`)

var tsLiteralString = regexp.MustCompile(`["']([^"'\n]+)["']`)

// analyticsServer holds the server side of each mirror, derived once.
type analyticsServer struct {
	fields, entities, native, numberDims, booleanDims []string
}

// analyticsPlant is one drift planted in the real sources, and the mirror that
// must see it.
type analyticsPlant struct {
	shape, catalog, vocab, drifts string
}

// analyticsMirror is one set as the server has it and as the browser spells it.
type analyticsMirror struct {
	what            string
	server, browser []string
	consequence     string
}

func TestTheQuestionBuilderNamesExactlyTheAnalyticsVocabulary(t *testing.T) {
	t.Parallel()
	server := deriveAnalyticsServer(t)
	for _, m := range analyticsMirrors(t, server, readSource(t, analyticsCatalog), readSource(t, analyticsVocabMirror)) {
		if missing, extra := mirrorDrift(m.server, m.browser); len(missing)+len(extra) > 0 {
			t.Errorf("%s: the browser lacks %v and carries %v the server does not have — %s",
				m.what, missing, extra, m.consequence)
		}
	}
}

// Each shape of drift, planted in the real sources, must be seen. A mirror this
// gate cannot read would otherwise agree with everything.
func TestTheQuestionBuilderGateSeesEveryDriftShape(t *testing.T) {
	t.Parallel()
	server := deriveAnalyticsServer(t)
	catalog, vocab := readSource(t, analyticsCatalog), readSource(t, analyticsVocabMirror)
	entities := literalSpan(t, vocab, "ENTITY_LABEL_KEY", '{', '}')
	firstField := analyticsFieldLabel.FindStringIndex(catalog)
	firstEntity := tsLiteralKey.FindStringSubmatchIndex(vocab[entities[0]:entities[1]])
	if firstField == nil || firstEntity == nil {
		t.Fatal("a mirror has no entry to remove, so the removal plants below prove nothing")
	}
	opens := strings.Index(catalog, "{") + 1
	retiredField := catalog[:opens] + "\n  \"analytics.field.retired_field\": \"x\"," + catalog[opens:]
	unlabelledEntity := cut(vocab, entities[0]+firstEntity[2], entities[0]+firstEntity[3])
	retiredEntity := vocab[:entities[0]+1] + `"retired-population": "x",` + vocab[entities[0]+1:]

	plants := []analyticsPlant{
		{"a label removed", cut(catalog, firstField[0], firstField[1]), vocab, "field"},
		{"a label for a retired field", retiredField, vocab, "field"},
		{"a population without its label", catalog, unlabelledEntity, "population"},
		{"a label for a retired population", catalog, retiredEntity, "population"},
	}
	plants = append(plants, listPlants(t, catalog, vocab, "NATIVE_CURRENCY_MEASURES", "native")...)
	plants = append(plants, listPlants(t, catalog, vocab, "NUMBER_DIMENSIONS", "number")...)
	plants = append(plants, listPlants(t, catalog, vocab, "BOOLEAN_DIMENSIONS", "boolean")...)
	for _, plant := range plants {
		seen := false
		for _, m := range analyticsMirrors(t, server, plant.catalog, plant.vocab) {
			missing, extra := mirrorDrift(m.server, m.browser)
			seen = seen || (strings.HasPrefix(m.what, plant.drifts) && len(missing)+len(extra) > 0)
		}
		if !seen {
			t.Errorf("planted %s and the %s mirror still agrees", plant.shape, plant.drifts)
		}
	}
}

// listPlants adds an entry the server lacks to one string-list mirror, and
// removes its first entry, each in a copy of the source.
func listPlants(t *testing.T, catalog, vocab, name, drifts string) []analyticsPlant {
	t.Helper()
	span := literalSpan(t, vocab, name, '[', ']')
	first := tsLiteralString.FindStringIndex(vocab[span[0]:span[1]])
	if first == nil {
		t.Fatalf("%s has no entry to remove, so its removal plant proves nothing", name)
	}
	return []analyticsPlant{
		{name + " naming a retired field", catalog, vocab[:span[0]+1] + `"retired_field", ` + vocab[span[0]+1:], drifts},
		{name + " missing an entry", catalog, cut(vocab, span[0]+first[0], span[0]+first[1]), drifts},
	}
}

func TestTheContractNamesTheEnginesAggregatesAndComparisons(t *testing.T) {
	t.Parallel()
	for _, m := range []analyticsMirror{
		{
			"AnalyticsMeasure.fn", analyticsquery.AggregateNames(), crmYAMLEnum(t, "AnalyticsMeasure", "fn"),
			"the builder offers an aggregate the engine refuses, or cannot offer one it has",
		},
		{
			"AnalyticsFilter.op", analyticsquery.FilterOpNames(), crmYAMLEnum(t, "AnalyticsFilter", "op"),
			"the builder offers a comparison the engine refuses, or cannot offer one it has",
		},
	} {
		if missing, extra := mirrorDrift(m.server, m.browser); len(missing)+len(extra) > 0 {
			t.Errorf("%s: the contract lacks %v and carries %v the engine does not have — %s",
				m.what, missing, extra, m.consequence)
		}
	}
}

func deriveAnalyticsServer(t *testing.T) analyticsServer {
	t.Helper()
	// The system principal clears every grant, so this is the whole vocabulary
	// rather than one seat's narrowing of it.
	ctx := principal.WithActor(principal.WithWorkspaceID(context.Background(), ids.NewV7()),
		principal.Principal{Type: principal.PrincipalSystem, ID: "system:gate"})
	schema := compose.AnalyticsSchemaFor(ctx)
	fields := map[string]bool{}
	shapes := map[analyticsquery.ColumnShape]map[string]bool{
		analyticsquery.ShapeNumber: {}, analyticsquery.ShapeBoolean: {},
	}
	dimensionShape := map[string]analyticsquery.ColumnShape{}
	for _, entity := range schema.Entities {
		for name, field := range entity.Fields {
			fields[name] = true
			if field.Kind != analyticsquery.KindDimension {
				continue
			}
			// The browser keys a dimension's shape by NAME, so one name must hold
			// one shape in every population that offers it.
			if seen, twice := dimensionShape[name]; twice && seen != field.Shape {
				t.Fatalf("dimension %s holds %q in one population and %q in another — "+
					"the browser cannot tell which", name, seen, field.Shape)
			}
			dimensionShape[name] = field.Shape
			if named, held := shapes[field.Shape]; held {
				named[name] = true
			}
		}
	}
	server := analyticsServer{
		fields: slices.Sorted(maps.Keys(fields)), entities: schema.EntityNames(),
		native:      nativeMoneyMeasures(t),
		numberDims:  slices.Sorted(maps.Keys(shapes[analyticsquery.ShapeNumber])),
		booleanDims: slices.Sorted(maps.Keys(shapes[analyticsquery.ShapeBoolean])),
	}
	if len(server.fields) == 0 || len(server.entities) == 0 || len(server.native) == 0 ||
		len(server.numberDims) == 0 || len(server.booleanDims) == 0 {
		t.Fatalf("a server set derived empty (%+v) — this gate would hold the browser to nothing", server)
	}
	return server
}

// nativeMoneyMeasures reads every nativeMoney declaration in compose, the one
// place a spec names a measure denominated in its row's own currency.
func nativeMoneyMeasures(t *testing.T) []string {
	t.Helper()
	consts := gatekit.PackageStringConstants(t, "internal/compose")
	sources, err := filepath.Glob("internal/compose/*.go")
	if err != nil {
		t.Fatalf("listing compose: %v", err)
	}
	measures := map[string]bool{}
	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), source, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", source, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, isCall := n.(*ast.CallExpr)
			if !isCall {
				return true
			}
			if fn, named := call.Fun.(*ast.Ident); named && fn.Name == "nativeMoney" {
				for _, arg := range call.Args {
					measures[nativeMoneyArg(t, source, arg, consts)] = true
				}
			}
			return true
		})
	}
	return slices.Sorted(maps.Keys(measures))
}

// nativeMoneyArg resolves one argument to its measure name, failing on a shape
// it cannot read rather than skipping it, or the census would come up short.
func nativeMoneyArg(t *testing.T, source string, arg ast.Expr, consts map[string]string) string {
	t.Helper()
	switch value := arg.(type) {
	case *ast.Ident:
		if name, ok := consts[value.Name]; ok {
			return name
		}
	case *ast.BasicLit:
		if name, err := strconv.Unquote(value.Value); err == nil {
			return name
		}
	}
	t.Fatalf("%s: a nativeMoney argument this gate cannot resolve to a measure name — "+
		"pass a string constant of package compose", source)
	return ""
}

func analyticsMirrors(t *testing.T, server analyticsServer, catalog, vocab string) []analyticsMirror {
	t.Helper()
	entities := literalSpan(t, vocab, "ENTITY_LABEL_KEY", '{', '}')
	list := func(name string) []string {
		span := literalSpan(t, vocab, name, '[', ']')
		return literalMatches(tsLiteralString, vocab[span[0]:span[1]])
	}
	return []analyticsMirror{
		{
			"field labels (analytics.field.*)", server.fields, submatches(analyticsFieldLabel, catalog),
			"a field shows as its wire name, or a retired one is offered and refused",
		},
		{
			"population labels (ENTITY_LABEL_KEY)", server.entities,
			literalMatches(tsLiteralKey, vocab[entities[0]:entities[1]]),
			"a population shows as its wire name, or a retired one is offered and refused",
		},
		{
			"native currency measures (NATIVE_CURRENCY_MEASURES)", server.native,
			list("NATIVE_CURRENCY_MEASURES"),
			"a per-currency amount is shown in the base currency, or a base amount in a row's own",
		},
		{
			"number dimensions (NUMBER_DIMENSIONS)", server.numberDims, list("NUMBER_DIMENSIONS"),
			"a filter sends a number as quoted text and is refused, or text as a number",
		},
		{
			"boolean dimensions (BOOLEAN_DIMENSIONS)", server.booleanDims, list("BOOLEAN_DIMENSIONS"),
			"a filter sends a yes-or-no as quoted text and is refused, or text as a boolean",
		},
	}
}

// literalSpan finds the literal a TypeScript declaration assigns, from its
// opening bracket to the first closing one: every mirror holds only strings.
// A Set's literal is the array it is built from.
func literalSpan(t *testing.T, source, name string, opens, closes byte) [2]int {
	t.Helper()
	declared := regexp.MustCompile(`\b` + name + `\b[^=\n]*=\s*(?:new Set(?:<[^>]*>)?\(\s*)?` +
		regexp.QuoteMeta(string(opens)))
	at := declared.FindStringIndex(source)
	if at == nil {
		t.Fatalf("%s declares no %s literal — this gate is reading a shape that is gone", analyticsVocabMirror, name)
	}
	end := strings.IndexByte(source[at[1]:], closes)
	if end < 0 {
		t.Fatalf("%s's %s literal is unterminated", analyticsVocabMirror, name)
	}
	return [2]int{at[1] - 1, at[1] + end + 1}
}

// literalMatches reads a literal with its comments stripped, so an entry named
// in prose beside it cannot stand in for the entry.
func literalMatches(pattern *regexp.Regexp, literal string) []string {
	return submatches(pattern, tsComment.ReplaceAllString(literal, " "))
}

// submatches is the catalog's reading, unstripped: a value such as "image/*"
// would open a block comment that swallows the keys after it.
func submatches(pattern *regexp.Regexp, source string) []string {
	var out []string
	for _, m := range pattern.FindAllStringSubmatch(source, -1) {
		out = append(out, m[1])
	}
	return out
}

// mirrorDrift is what one side has and the other lacks, each way.
func mirrorDrift(server, browser []string) (missing, extra []string) {
	for _, name := range server {
		if !slices.Contains(browser, name) {
			missing = append(missing, name)
		}
	}
	for _, name := range browser {
		if !slices.Contains(server, name) {
			extra = append(extra, name)
		}
	}
	return missing, extra
}

func cut(s string, from, to int) string { return s[:from] + s[to:] }
