// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The counts and breakdowns extensions add to Reports and Home.
//
// A declaration is data. It becomes a reportSpec and runs through
// reportEngine.runSpec, so the engine's row scope and field masks apply. The
// group floor is analyticsquery's own Apply. A unit supplies only a column name
// its kind declared or a custom field this installation defines. Both are
// quoted as identifiers, so no SQL comes from a declaration.

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
	"github.com/margince/margince/backend/pkg/extension"
)

const (
	extReportCount     = "count"
	extReportBreakdown = "breakdown"
	// extReportRecords is the output column of the count every registered
	// report asks for; the floor judges each group by it.
	extReportRecords = "records"
)

// registeredReport is one declared count or breakdown, resolved to the name a
// caller sees and the population it reads.
type registeredReport struct {
	name, label, kind string
	unit              extension.Name
	// population is a record kind's full name, or the key of a core population.
	population string
	by         string
}

var (
	extReportsMu sync.RWMutex
	extReports   []registeredReport
)

// preflightReports refuses a count or breakdown core could not run, and a name
// one unit uses twice. A core population must be one the report catalog serves,
// and a breakdown of one must name a custom field of its deal.
func preflightReports(exts []extension.Extension) error {
	for _, e := range exts {
		seen := map[string]bool{}
		check := func(name string, err error) error {
			if err != nil {
				return fmt.Errorf("compose: extension %q: %w", e.Name, err)
			}
			if seen[name] {
				return fmt.Errorf("compose: extension %q declares report %q twice", e.Name, name)
			}
			seen[name] = true
			return nil
		}
		for _, c := range e.ReportCounts {
			err := c.Validate(e.RecordKinds)
			if err == nil {
				err = corePopulationServed(c.Population)
			}
			if err := check(c.Name, err); err != nil {
				return err
			}
		}
		for _, b := range e.ReportBreakdowns {
			err := b.Validate(e.RecordKinds)
			if err == nil {
				err = coreBreakdownServed(b)
			}
			if err := check(b.Name, err); err != nil {
				return err
			}
		}
	}
	return nil
}

// corePopulationServed refuses a hyphenated population the report catalog does
// not serve. A kind's short name has no hyphen, so it passes here.
func corePopulationServed(population string) error {
	if !strings.Contains(population, "-") {
		return nil
	}
	if _, ok := prebuiltReports[population]; !ok {
		return fmt.Errorf("population %q is not a core report population", population)
	}
	return nil
}

func coreBreakdownServed(b extension.ReportBreakdown) error {
	if !strings.Contains(b.Population, "-") {
		return nil
	}
	if err := corePopulationServed(b.Population); err != nil {
		return err
	}
	if spec := prebuiltReports[b.Population]; spec.entity != datasource.EntityDeal || spec.table != tableDeal {
		return fmt.Errorf("population %q takes no custom-field breakdown; only deal populations do", b.Population)
	}
	if !strings.HasPrefix(b.By, datasource.CustomFieldPrefix) && !strings.HasPrefix(b.By, analyticsquery.CompanyCustomFieldPrefix) {
		return fmt.Errorf("breakdown %q splits a core population by %q, which is not a custom field", b.Name, b.By)
	}
	return nil
}

// registerReports fills the registry the Reports and Home endpoint reads.
func registerReports(exts []extension.Extension) {
	extReportsMu.Lock()
	defer extReportsMu.Unlock()
	extReports = extReports[:0]
	for _, e := range exts {
		for _, c := range e.ReportCounts {
			extReports = append(extReports, registeredReport{
				name: extension.ReportName(e.Name, c.Name), label: c.Label, kind: extReportCount,
				unit: e.Name, population: resolvedPopulation(e.Name, c.Population),
			})
		}
		for _, b := range e.ReportBreakdowns {
			extReports = append(extReports, registeredReport{
				name: extension.ReportName(e.Name, b.Name), label: b.Label, kind: extReportBreakdown,
				unit: e.Name, population: resolvedPopulation(e.Name, b.Population), by: b.By,
			})
		}
	}
	slices.SortFunc(extReports, func(a, b registeredReport) int { return strings.Compare(a.name, b.name) })
}

func resolvedPopulation(unit extension.Name, population string) string {
	if strings.Contains(population, "-") {
		return population
	}
	return extension.RecordKindName(unit, population)
}

func registeredReports() []registeredReport {
	extReportsMu.RLock()
	defer extReportsMu.RUnlock()
	return slices.Clone(extReports)
}

// extSchemaPrefix opens the table of every spec over a unit's own rows.
const extSchemaPrefix = `"ext".`

// overExtensionTable says the spec reads a unit's own table. Those rows have no
// owner, so no row-scope clause exists to render. The kind's read right, taken
// at admission, is their whole scope.
func (s reportSpec) overExtensionTable() bool {
	return strings.HasPrefix(s.table, extSchemaPrefix)
}

// extensionKindSpec is the population of a record kind a unit declared. The
// kind's rights object gates the read and names the field masks that reach it.
func extensionKindSpec(d recordkinds.Declared, by string) reportSpec {
	spec := reportSpec{
		entity:      datasource.EntityType(d.Object),
		table:       extSchemaPrefix + pgx.Identifier{d.Table}.Sanitize(),
		baseWhere:   whereArchivedNull,
		basePlain:   "live " + d.DisplayName + " records",
		population:  measureEveryReadableRow,
		dimensions:  map[string]string{},
		measures:    map[string]string{},
		filters:     map[string]string{},
		defaultAggs: []reportAggregate{{Fn: aggFnCount, As: extReportRecords}},
	}
	if by != "" {
		// Cast to text: a unit's column can hold any type, and the wire key is text.
		spec.dimensions[by] = "t." + pgx.Identifier{by}.Sanitize() + "::text"
	}
	return spec
}

// multiselectDimensions copies a deal population and adds the multi-select
// custom fields as dimensions. The shared custom-field writer supplies the
// company scope, the grants and the reference scopes. A multi-select column
// holds an array, so the dimension reads the sorted combination of values.
// One record then falls in one group and the groups add up to the whole.
// The field is not a filter, because an array has no single value to equal.
func multiselectDimensions(spec reportSpec, deal, company []fieldcatalog.Column) reportSpec {
	toReportable := func(cols []fieldcatalog.Column) []fieldcatalog.ReportableColumn {
		out := make([]fieldcatalog.ReportableColumn, 0, len(cols))
		for _, c := range cols {
			out = append(out, fieldcatalog.ReportableColumn{Column: c, Label: c.Name})
		}
		return out
	}
	spec = withCustomDimensions(spec, reportableColumns{deal: toReportable(deal), company: toReportable(company)})
	for name, expr := range spec.dimensions {
		if _, custom := spec.customFields[expr]; custom {
			spec.dimensions[name] = sortedCombination(expr)
			delete(spec.filters, name)
		}
	}
	return spec
}

// sortedCombination renders an array column as its values in one fixed order,
// so {a,b} and {b,a} are one group.
func sortedCombination(expr string) string {
	return "(SELECT string_agg(v, ',' ORDER BY v) FROM unnest(" + expr + ") v)"
}

// multiselectColumns reads the active multi-select custom fields of one object.
func multiselectColumns(ctx context.Context, tx pgx.Tx, object datasource.EntityType) ([]fieldcatalog.Column, error) {
	cols, err := customfields.ActiveColumnsIn(ctx, tx, string(object))
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(cols, func(c fieldcatalog.Column) bool { return c.Type != fieldcatalog.TypeMultiselect }), nil
}

// specFor builds the spec one registered report runs over, and says false for a
// breakdown whose custom field this installation does not define.
func (r registeredReport) specFor(ctx context.Context, pool *pgxpool.Pool) (reportSpec, bool, error) {
	if declared, ok := recordkinds.Lookup(r.population); ok {
		return extensionKindSpec(declared, r.by), true, nil
	}
	spec, ok := prebuiltReports[r.population]
	if !ok {
		return reportSpec{}, false, nil
	}
	if r.kind == extReportCount {
		spec.defaultBy = nil
		return spec, true, nil
	}
	var deal, company []fieldcatalog.Column
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		var err error
		if deal, err = multiselectColumns(ctx, tx, datasource.EntityDeal); err != nil {
			return err
		}
		company, err = multiselectColumns(ctx, tx, datasource.EntityCompany)
		return err
	})
	if err != nil {
		return reportSpec{}, false, err
	}
	spec = multiselectDimensions(spec, deal, company)
	spec.defaultBy = nil
	_, offered := spec.dimensions[r.by]
	return spec, offered, nil
}

// extensionReportAnswer is one registered report's answer after the floor.
type extensionReportAnswer struct {
	Report registeredReport
	Groups []extensionReportGroup
	// Withheld says the floor kept a group back, and with it the total.
	Withheld bool
	// Total is the whole count, nil while Withheld.
	Total                *int
	ExcludedByPermission *int
}

type extensionReportGroup struct {
	Key      any
	Count    int
	Withheld bool
}

// run answers one registered report through the report engine, then the floor.
// ok is false when the report does not apply here: a custom field this
// installation does not define.
func (e *reportEngine) runRegistered(ctx context.Context, r registeredReport, floor analyticsquery.Floor) (extensionReportAnswer, bool, error) {
	spec, ok, err := r.specFor(ctx, e.pool)
	if err != nil || !ok {
		return extensionReportAnswer{}, false, err
	}
	req := reportRequest{Aggregates: []reportAggregate{{Fn: aggFnCount, As: extReportRecords}}}
	if r.by != "" {
		req.GroupBy = []string{r.by}
	}
	outcome, err := e.runSpec(ctx, r.name, spec, req)
	if err != nil {
		return extensionReportAnswer{}, false, err
	}
	rows := make([]analyticsquery.Row, len(outcome.Rows))
	for i, row := range outcome.Rows {
		rows[i] = analyticsquery.Row{Count: rowCount(row[extReportRecords])}
	}
	judged, withheld := floor.Apply(rows)
	answer := extensionReportAnswer{Report: r, Withheld: withheld, ExcludedByPermission: outcome.ExcludedByPermission}
	sum := 0
	for i, row := range outcome.Rows {
		group := extensionReportGroup{Withheld: judged[i].Withheld}
		if !group.Withheld {
			group.Count = judged[i].Count
			if r.by != "" {
				group.Key = row[r.by]
			}
			sum += group.Count
		}
		answer.Groups = append(answer.Groups, group)
	}
	if analyticsquery.TotalIsSafe(withheld) {
		answer.Total = &sum
	}
	return answer, true, nil
}
