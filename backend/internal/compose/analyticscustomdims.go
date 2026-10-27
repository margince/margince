// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"hash"
	"maps"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// customDimension is one reportable custom field as a report may group and
// filter by it.
type customDimension struct {
	label   string
	typ     string
	options []string
	// company says the field is the deal's company's, so a count over it owes
	// the company's row scope as well as the deal's.
	company bool
}

// describe names the field in the vocabulary: its label, and the value shape
// and choices a builder offers for it.
func (d customDimension) describe(field analyticsquery.Field) analyticsquery.Field {
	field.Label = d.label
	field.Options = d.options
	if d.typ == fieldcatalog.TypeBoolean {
		field.Shape = analyticsquery.ShapeBoolean
	}
	return field
}

// reportableColumns are the custom fields an administrator opted into
// reporting: the deal's own, and the company's, which a deal reaches through
// its company_id.
type reportableColumns struct {
	deal, company []fieldcatalog.ReportableColumn
}

func reportableColumnsIn(ctx context.Context, tx pgx.Tx) (reportableColumns, error) {
	var cols reportableColumns
	var err error
	if cols.deal, err = customfields.ReportableColumnsIn(ctx, tx, string(datasource.EntityDeal)); err != nil {
		return cols, err
	}
	cols.company, err = customfields.ReportableColumnsIn(ctx, tx, string(datasource.EntityCompany))
	return cols, err
}

// analyticsCatalog is the populations a question may be asked of in this
// installation. Every deal population of the prebuilt catalog gains the deal
// and company fields an administrator opted into reporting.
func analyticsCatalog(ctx context.Context, tx pgx.Tx) (map[string]reportSpec, error) {
	cols, err := reportableColumnsIn(ctx, tx)
	if err != nil {
		return nil, err
	}
	if len(cols.deal) == 0 && len(cols.company) == 0 {
		return prebuiltReports, nil
	}
	out := make(map[string]reportSpec, len(prebuiltReports))
	for key, spec := range prebuiltReports {
		if spec.entity == datasource.EntityDeal && spec.table == tableDeal {
			spec = withCustomDimensions(spec, cols)
		}
		out[key] = spec
	}
	return out, nil
}

// dimensionLinkType is the name a report groups deals by their typed links under.
const dimensionLinkType = "link_type"

// init gives every deal population the link types a deal holds as a dimension
// and a filter, because a report's dimension is always one too.
// The expression reads only links to other live deals, so the deal read right
// the report already requires covers both ends.
func init() {
	for _, spec := range prebuiltReports {
		if spec.entity == datasource.EntityDeal && spec.table == tableDeal {
			expr := storekit.DealLinkTypesExpr(colRowID)
			spec.dimensions[dimensionLinkType] = expr
			spec.filters[dimensionLinkType] = expr
		}
	}
}

// withCustomDimensions copies a deal spec and adds each column as a dimension
// and a filter. The maps are copied: every request shares the prebuilt catalog.
//
// A deal field needs no scope beyond the deal's own. A company field has its
// company's row scope, so it carries scopeVia to the company id, as the size
// band does. A deal at a company the reader may not open then leaves the answer.
func withCustomDimensions(spec reportSpec, cols reportableColumns) reportSpec {
	spec.dimensions = maps.Clone(spec.dimensions)
	spec.filters = maps.Clone(spec.filters)
	spec.customFields = make(map[string]customDimension, len(cols.deal)+len(cols.company))
	add := func(name, expr string, col fieldcatalog.ReportableColumn, company bool) {
		spec.dimensions[name] = expr
		spec.filters[name] = expr
		spec.customFields[expr] = customDimension{label: col.Label, typ: col.Type, options: col.Options, company: company}
	}
	for _, col := range cols.deal {
		add(col.Name, "t."+pgx.Identifier{col.Name}.Sanitize(), col, false)
	}
	if len(cols.company) == 0 {
		return spec
	}
	spec.referenceScopes = maps.Clone(spec.referenceScopes)
	if spec.referenceScopes == nil {
		spec.referenceScopes = map[string]string{}
	}
	spec.referenceScopes[colCompanyID] = tableCompany
	spec.scopeVia = maps.Clone(spec.scopeVia)
	if spec.scopeVia == nil {
		spec.scopeVia = map[string]string{}
	}
	spec.grants = maps.Clone(spec.grants)
	if spec.grants == nil {
		spec.grants = map[string]string{}
	}
	for _, col := range cols.company {
		name := analyticsquery.CompanyCustomFieldPrefix + strings.TrimPrefix(col.Name, datasource.CustomFieldPrefix)
		add(name, companyFieldExpr(col.Name), col, true)
		spec.scopeVia[name] = colCompanyID
		spec.grants[name] = tableCompany
	}
	return spec
}

// companyFieldExpr reads one company column for the deal's company. The
// subselect on the company's primary key matches at most one row, so the grain
// stays one row per deal. A deal with no company reads NULL.
func companyFieldExpr(column string) string {
	return "(SELECT c." + pgx.Identifier{column}.Sanitize() + " FROM " + tableCompany +
		" c WHERE c.id = " + colCompanyID + ")"
}

// AnalyticsSchemaWithCoverage is AnalyticsSchemaIn with each custom field's
// coverage counted. The schema endpoint and the vocabulary document show it; a
// question run reads AnalyticsSchemaIn and does not pay for the count.
func AnalyticsSchemaWithCoverage(ctx context.Context, tx pgx.Tx) (analyticsquery.Schema, error) {
	schema, specs, err := AnalyticsSchemaIn(ctx, tx)
	if err != nil {
		return schema, err
	}
	fields := map[string]customDimension{}
	for _, spec := range specs {
		maps.Copy(fields, spec.customFields)
	}
	counted, err := customFieldCoverage(ctx, tx, fields)
	if err != nil {
		return schema, err
	}
	for _, entity := range schema.Entities {
		for name, field := range entity.Fields {
			if coverage, ok := counted[field.Expr]; ok && field.Label != "" {
				field.Coverage = &coverage
				entity.Fields[name] = field
			}
		}
	}
	return schema, nil
}

// customFieldCoverage counts the live deals this caller can read, and how many
// hold a value in each field, keyed by the field's expression. A company
// field's value counts only where the caller may open the company, the scope a
// grouping by it applies.
func customFieldCoverage(
	ctx context.Context, tx pgx.Tx, fields map[string]customDimension,
) (map[string]analyticsquery.Coverage, error) {
	if len(fields) == 0 {
		return map[string]analyticsquery.Coverage{}, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	scope, err := auth.ScopeClauseFor(ctx, tableDeal, "t", arg)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = sqlUnnarrowed
	}
	exprs := slices.Sorted(maps.Keys(fields))
	visible, err := companyVisibleClause(ctx, fields, arg)
	if err != nil {
		return nil, err
	}
	counts := make([]string, 0, len(exprs))
	for _, expr := range exprs {
		count := "count(" + expr + ")"
		if fields[expr].company {
			count += " FILTER (WHERE " + visible + ")"
		}
		counts = append(counts, count)
	}
	records := 0
	held := make([]int, len(exprs))
	targets := []any{&records}
	for i := range held {
		targets = append(targets, &held[i])
	}
	sql := "SELECT count(*), " + strings.Join(counts, ", ") +
		" FROM " + tableDeal + " t WHERE t.archived_at IS NULL AND " + scope
	if err := tx.QueryRow(ctx, sql, args...).Scan(targets...); err != nil {
		return nil, fmt.Errorf("compose: counting the custom fields' coverage: %w", err)
	}
	out := make(map[string]analyticsquery.Coverage, len(exprs))
	for i, expr := range exprs {
		out[expr] = analyticsquery.Coverage{Records: records, WithValue: held[i]}
	}
	return out, nil
}

// companyVisibleClause is the company scope a grouping by a company field
// applies. It binds its arguments only when some field is a company's, since a
// statement handed arguments it never reads is refused.
func companyVisibleClause(ctx context.Context, fields map[string]customDimension, arg func(any) int) (string, error) {
	if !slices.ContainsFunc(slices.Collect(maps.Values(fields)), func(d customDimension) bool { return d.company }) {
		return sqlUnnarrowed, nil
	}
	companies := reportSpec{referenceScopes: map[string]string{colCompanyID: tableCompany}}
	clauses, err := referenceScopeClauses(ctx, companies, referencedColumns{colCompanyID: true}, arg)
	if err != nil || len(clauses) == 0 {
		return sqlUnnarrowed, err
	}
	return clauses[0], nil
}

// analyticsCustomFieldsOf lists the population's custom dimensions for the
// schema endpoint, in name order. Nil when it has none, so the key is left out.
func analyticsCustomFieldsOf(entity analyticsquery.Entity) *[]crmcontracts.AnalyticsCustomField {
	var out []crmcontracts.AnalyticsCustomField
	for _, name := range entity.FieldNames(analyticsquery.KindDimension) {
		field := entity.Fields[name]
		if field.Label == "" {
			continue
		}
		typ := crmcontracts.AnalyticsCustomFieldTypePicklist
		if field.Shape == analyticsquery.ShapeBoolean {
			typ = crmcontracts.AnalyticsCustomFieldTypeBoolean
		}
		options := field.Options
		if options == nil {
			options = []string{}
		}
		object := crmcontracts.AnalyticsCustomFieldObjectDeal
		if strings.HasPrefix(name, analyticsquery.CompanyCustomFieldPrefix) {
			object = crmcontracts.AnalyticsCustomFieldObjectCompany
		}
		var coverage analyticsquery.Coverage
		if field.Coverage != nil {
			coverage = *field.Coverage
		}
		out = append(out, crmcontracts.AnalyticsCustomField{
			Name: name, Label: field.Label, Type: typ, Options: options, Object: object,
			Records: coverage.Records, WithValue: coverage.WithValue,
		})
	}
	if out == nil {
		return nil
	}
	return &out
}

// describeCustomFields names each custom dimension for a model, which has no
// other way to learn what an installation's cf_ column means. An administrator
// typed the labels and choices, so each is JSON-quoted. A comma or a line break
// inside one then cannot read as a second choice or a new line.
func describeCustomFields(out *strings.Builder, entity analyticsquery.Entity) {
	header := false
	for _, name := range entity.FieldNames(analyticsquery.KindDimension) {
		field := entity.Fields[name]
		if field.Label == "" {
			continue
		}
		if !header {
			out.WriteString("  custom fields, each with the label and choices an administrator gave it:\n")
			header = true
		}
		choices := field.Options
		if field.Shape == analyticsquery.ShapeBoolean {
			choices = []string{jsonTrue, jsonFalse}
		}
		fmt.Fprintf(out, "    %s %s: %s", name, jsonText(field.Label), jsonText(choices))
		if strings.HasPrefix(name, analyticsquery.CompanyCustomFieldPrefix) {
			out.WriteString(", the deal's company's field")
		}
		if field.Coverage != nil {
			fmt.Fprintf(out, ", held by %d of %d live deals you can read", field.Coverage.WithValue, field.Coverage.Records)
		}
		out.WriteString("\n")
	}
}

// writeCustomMeaning adds a custom dimension's label and choices to the schema
// digest, so a renamed field or changed choice moves the version. A built-in
// field writes nothing, which keeps every existing version as it was.
func writeCustomMeaning(sum hash.Hash, field analyticsquery.Field) {
	if field.Label == "" {
		return
	}
	sum.Write([]byte(jsonText(field.Label) + jsonText(field.Options)))
}

// jsonText is v as JSON, with < > & left as typed, since a model reads it and
// no browser does. A string or a slice of strings always encodes.
func jsonText[T string | []string](v T) string {
	var out strings.Builder
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return ""
	}
	return strings.TrimSuffix(out.String(), "\n")
}
