// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Extension reports run through reportEngine with the same scopes, masks and
// group floor as core reports. Declarations name fields, never arbitrary SQL.

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/margince/margince/backend/internal/compose/reportjoin"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
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
		return fmt.Errorf("population %q takes no breakdown by a custom field or link type; only deal populations do", b.Population)
	}
	if b.By != dimensionLinkType && !strings.HasPrefix(b.By, datasource.CustomFieldPrefix) && !strings.HasPrefix(b.By, analyticsquery.CompanyCustomFieldPrefix) {
		return fmt.Errorf("breakdown %q splits a core population by %q, which is neither a custom field nor %s", b.Name, b.By, dimensionLinkType)
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
				unit: e.Name, population: reportjoin.ResolvedPopulation(e.Name, c.Population),
			})
		}
		for _, b := range e.ReportBreakdowns {
			extReports = append(extReports, registeredReport{
				name: extension.ReportName(e.Name, b.Name), label: b.Label, kind: extReportBreakdown,
				unit: e.Name, population: reportjoin.ResolvedPopulation(e.Name, b.Population), by: b.By,
			})
		}
	}
	slices.SortFunc(extReports, func(a, b registeredReport) int { return strings.Compare(a.name, b.name) })
}

func registeredReports() []registeredReport {
	extReportsMu.RLock()
	defer extReportsMu.RUnlock()
	return slices.Clone(extReports)
}

// extSchemaPrefix opens the table of every spec over a unit's own rows.
const extSchemaPrefix = `"ext".`

// extensionKindSpec is the population of a record kind a unit declared. The
// kind's rights object gates the read and names the field masks that reach it.
//
// A breakdown here skips the administrator's reportable opt-in. That opt-in
// guards fields an administrator adds to a core record. A unit's fields are
// its own declared schema, and the kind's read right and field masks gate
// every value a report can group by.
func extensionKindSpec(d recordkinds.Declared, by string) reportSpec {
	spec := reportSpec{
		entity:        datasource.EntityType(d.Object),
		table:         extSchemaPrefix + pgx.Identifier{d.Table}.Sanitize(),
		extensionKind: d.Name,
		baseWhere:     whereArchivedNull,
		basePlain:     "live " + d.DisplayName + " records",
		population:    measureEveryReadableRow,
		dimensions:    map[string]string{},
		measures:      map[string]string{},
		filters:       map[string]string{},
		defaultAggs:   []reportAggregate{{Fn: aggFnCount, As: extReportRecords}},
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
	spec = withCustomDimensions(spec, reportableColumns{deal: reportjoin.ReportableColumns(deal), company: reportjoin.ReportableColumns(company)})
	for name, expr := range spec.dimensions {
		if _, custom := spec.customFields[expr]; custom {
			spec.dimensions[name] = reportjoin.SortedCombination(expr)
			delete(spec.filters, name)
		}
	}
	return spec
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
	if strings.Contains(r.by, ".") {
		return r.joinedSpecFor(ctx, pool, spec)
	}
	var deal, company []fieldcatalog.Column
	err := database.WithTx(ctx, pool, func(tx pgx.Tx) error {
		var err error
		if deal, err = reportjoin.MultiselectColumns(ctx, tx, datasource.EntityDeal); err != nil {
			return err
		}
		company, err = reportjoin.MultiselectColumns(ctx, tx, datasource.EntityCompany)
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
	var answer extensionReportAnswer
	var offered bool
	err := database.WithSnapshot(ctx, e.pool, func(ctx context.Context) error {
		var err error
		answer, offered, err = e.runRegisteredSnapshot(ctx, r, floor)
		return err
	})
	return answer, offered, err
}

func (e *reportEngine) runRegisteredSnapshot(ctx context.Context, r registeredReport, floor analyticsquery.Floor) (extensionReportAnswer, bool, error) {
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

func (r registeredReport) joinedSpecFor(ctx context.Context, pool *pgxpool.Pool, spec reportSpec) (reportSpec, bool, error) {
	join, err := reportjoin.Resolve(ctx, pool, r.unit, r.by)
	if err != nil || join == nil {
		return reportSpec{}, false, err
	}
	spec.joins = append(slices.Clone(spec.joins), join.SQL)
	spec.dimensions = maps.Clone(spec.dimensions)
	spec.dimensions[r.by] = join.Dimension
	spec.grants = maps.Clone(spec.grants)
	if spec.grants == nil {
		spec.grants = map[string]string{}
	}
	spec.grants[r.by] = join.Kind
	spec.extensionJoin = join
	spec.defaultBy = nil
	return spec, true, nil
}
