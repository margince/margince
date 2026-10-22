// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// everyRegisteredSpec is the census: the prebuilt catalog and the ad-hoc
// vocabulary of every datasource entity, named so a failure says which.
func everyRegisteredSpec(t *testing.T) map[string]reportSpec {
	t.Helper()
	specs := maps.Clone(prebuiltReports)
	for _, entity := range datasource.EntityTypes() {
		if spec, ok := adHocSpec(entity); ok {
			specs["adhoc:"+string(entity)] = spec
		}
	}
	if _, ok := specs["activities-by-kind"]; !ok {
		t.Fatal("the census lost activities-by-kind; it is reading a smaller catalog than the engine runs")
	}
	if _, ok := specs["adhoc:activity"]; !ok {
		t.Fatal("the census lost the ad-hoc activity vocabulary")
	}
	return specs
}

func TestEveryActivityPopulationCountsOnlyMeetingsAndTasks(t *testing.T) {
	activitySpecs := 0
	for name, spec := range everyRegisteredSpec(t) {
		if spec.table != tableActivity {
			continue
		}
		activitySpecs++
		if !strings.Contains(spec.populationWhere(), activityKindBound) {
			t.Errorf("%s: population %q is not bounded to %v", name, spec.populationWhere(), reportableActivityKinds)
		}
		for filter, expr := range spec.filters {
			if expr == colKind && len(filterDomain(spec, expr).Values) == 0 {
				t.Errorf("%s: filter %s over the activity kind admits any value", name, filter)
			}
		}
	}
	if activitySpecs < 3 {
		t.Fatalf("found %d activity populations, want at least activities-by-kind, meeting-conversion and the ad-hoc one", activitySpecs)
	}
}

// subqueryOverActivity finds an activity read inside another population's
// expression, with the alias it reads it under.
var subqueryOverActivity = regexp.MustCompile(`(?i)\b(?:FROM|JOIN)\s+activity\s+(?:AS\s+)?([a-z_][a-z0-9_]*)`)

// kindBoundFor reports whether sql pins alias.kind to reportable kinds only.
func kindBoundFor(sql, alias string) bool {
	eq := regexp.MustCompile(`\b` + regexp.QuoteMeta(alias) + `\.kind\s*=\s*'([a-z_]+)'`)
	in := regexp.MustCompile(`\b` + regexp.QuoteMeta(alias) + `\.kind\s+IN\s*\(([^)]*)\)`)
	bound := false
	for _, m := range eq.FindAllStringSubmatch(sql, -1) {
		if !slices.Contains(reportableActivityKinds, m[1]) {
			return false
		}
		bound = true
	}
	for _, m := range in.FindAllStringSubmatch(sql, -1) {
		for kind := range strings.SplitSeq(m[1], ",") {
			if !slices.Contains(reportableActivityKinds, strings.Trim(strings.TrimSpace(kind), "'")) {
				return false
			}
		}
		bound = true
	}
	return bound
}

func TestEveryActivityReadInsideAReportExpressionIsBounded(t *testing.T) {
	for name, spec := range everyRegisteredSpec(t) {
		exprs := slices.Concat(spec.joins, slices.Collect(maps.Values(spec.dimensions)),
			slices.Collect(maps.Values(spec.measures)), slices.Collect(maps.Values(spec.filters)))
		for _, expr := range exprs {
			for _, m := range subqueryOverActivity.FindAllStringSubmatch(expr, -1) {
				if !kindBoundFor(expr, m[1]) {
					t.Errorf("%s: %q reads activity without bounding %s.kind to %v", name, expr, m[1], reportableActivityKinds)
				}
			}
		}
	}
}

// TestEveryMetricReaderOverActivityIsBounded reads the source of every reader
// the metric catalog registers. A new metric over activity joins the census
// when it is registered.
func TestEveryMetricReaderOverActivityIsBounded(t *testing.T) {
	bodies := composeFuncLiterals(t)
	for _, metric := range reportingMetrics {
		full := runtime.FuncForPC(reflect.ValueOf(metric.read).Pointer()).Name()
		name := full[strings.LastIndex(full, ".")+1:]
		sql, ok := bodies[name]
		if !ok {
			t.Fatalf("metric %s: reader %s not found in the compose sources", metric.definition.Id, name)
		}
		for _, m := range subqueryOverActivity.FindAllStringSubmatch(sql, -1) {
			if !kindBoundFor(sql, m[1]) {
				t.Errorf("metric %s: %s reads activity without bounding %s.kind to %v", metric.definition.Id, name, m[1], reportableActivityKinds)
			}
		}
	}
}

// TestNoDoorReadsTheBasePredicateAroundTheBound fails when code outside
// populationWhere reads baseWhere, so no door can skip the activity bound.
func TestNoDoorReadsTheBasePredicateAroundTheBound(t *testing.T) {
	for _, file := range parsedComposeSources(t) {
		for _, decl := range file.syntax.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if isFunc && fn.Name.Name == "populationWhere" {
				continue
			}
			ast.Inspect(decl, func(node ast.Node) bool {
				if sel, ok := node.(*ast.SelectorExpr); ok && sel.Sel.Name == "baseWhere" {
					t.Errorf("%s: reads .baseWhere directly; read spec.populationWhere() so the activity bound applies",
						file.fset.Position(sel.Pos()))
				}
				return true
			})
		}
	}
}

func TestAFilterNamingAnUncountedKindIsRefusedOnEveryDoor(t *testing.T) {
	spec := prebuiltReports["activities-by-kind"]
	var refusal *analyticsquery.RefusalError
	for _, kind := range []string{"email", "message", "call", "note"} {
		err := filterDomain(spec, colKind).Admits(fieldKind, kind)
		if !errors.As(err, &refusal) || refusal.Kind != analyticsquery.RefusalUnsupported {
			t.Errorf("report filter kind=%s: err = %v, want an unsupported refusal", kind, err)
		}
		if _, err := resolveValuePredicate(spec, fieldKind, kind); !errors.As(err, &refusal) {
			t.Errorf("drill-through kind=%s: err = %v, want a refusal", kind, err)
		}
	}
	for _, kind := range reportableActivityKinds {
		if err := filterDomain(spec, colKind).Admits(fieldKind, kind); err != nil {
			t.Errorf("kind=%s: %v, want it admitted", kind, err)
		}
	}
	if err := filterDomain(prebuiltReports["deals-by-stage"], colKind).Admits(fieldKind, "email"); err != nil {
		t.Errorf("a kind column on another table took the activity domain: %v", err)
	}
}

func TestTheQueryVocabularyCarriesTheKindDomain(t *testing.T) {
	entity, ok := AnalyticsSchemaFor(seatWith(everySeat()...)).Entities["activities-by-kind"]
	if !ok {
		t.Fatal("activities-by-kind is missing from a full seat's vocabulary")
	}
	if !strings.Contains(entity.BaseWhere, activityKindBound) {
		t.Errorf("BaseWhere = %q, want the activity bound", entity.BaseWhere)
	}
	if got := entity.Fields[fieldKind].Domain.Values; !slices.Equal(got, reportableActivityKinds) {
		t.Errorf("kind domain = %v, want %v", got, reportableActivityKinds)
	}
}

type parsedComposeFile struct {
	fset   *token.FileSet
	syntax *ast.File
}

func parsedComposeSources(t *testing.T) []parsedComposeFile {
	t.Helper()
	var out []parsedComposeFile
	for _, path := range composeSources(t) {
		fset := token.NewFileSet()
		syntax, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		out = append(out, parsedComposeFile{fset: fset, syntax: syntax})
	}
	if len(out) < 100 {
		t.Fatalf("read %d compose sources; the census is not reading the package", len(out))
	}
	return out
}

// composeFuncLiterals maps each top-level function to the text of every string
// literal in its body, which is where a reader spells its SQL.
func composeFuncLiterals(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, file := range parsedComposeSources(t) {
		for _, decl := range file.syntax.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Recv != nil {
				continue
			}
			var text strings.Builder
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				if lit, ok := node.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if value, err := strconv.Unquote(lit.Value); err == nil {
						text.WriteString(value)
						text.WriteString(" ")
					}
				}
				return true
			})
			out[fn.Name.Name] = text.String()
		}
	}
	return out
}
