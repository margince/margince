// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"maps"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// customDimension is one reportable custom field as a report may group and
// filter by it.
type customDimension struct {
	label   string
	typ     string
	options []string
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

// analyticsCatalog is the populations a question may be asked of in this
// installation: the prebuilt catalog, with every deal population extended by
// the deal fields an administrator opted into reporting. A deal field adds no
// row scope of its own: the deal's own scope already bounds which values a
// reader's question reaches.
func analyticsCatalog(ctx context.Context, tx pgx.Tx) (map[string]reportSpec, error) {
	cols, err := customfields.ReportableColumnsIn(ctx, tx, string(datasource.EntityDeal))
	if err != nil {
		return nil, err
	}
	if len(cols) == 0 {
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

// withCustomDimensions copies a deal spec and adds each column as a dimension
// and a filter. The maps are copied, never written in place: the prebuilt
// catalog is shared by every request, and installations differ in their fields.
func withCustomDimensions(spec reportSpec, cols []fieldcatalog.ReportableColumn) reportSpec {
	spec.dimensions = maps.Clone(spec.dimensions)
	spec.filters = maps.Clone(spec.filters)
	spec.customFields = make(map[string]customDimension, len(cols))
	for _, col := range cols {
		expr := "t." + pgx.Identifier{col.Name}.Sanitize()
		spec.dimensions[col.Name] = expr
		spec.filters[col.Name] = expr
		spec.customFields[expr] = customDimension{label: col.Label, typ: col.Type, options: col.Options}
	}
	return spec
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
		out = append(out, crmcontracts.AnalyticsCustomField{Name: name, Label: field.Label, Type: typ, Options: options})
	}
	if out == nil {
		return nil
	}
	return &out
}

// describeCustomFields names each custom dimension for a model, which has no
// other way to learn what an installation's cf_ column means.
func describeCustomFields(out *strings.Builder, entity analyticsquery.Entity) {
	for _, name := range entity.FieldNames(analyticsquery.KindDimension) {
		field := entity.Fields[name]
		if field.Label == "" {
			continue
		}
		choices := strings.Join(field.Options, ", ")
		if field.Shape == analyticsquery.ShapeBoolean {
			choices = "true, false"
		}
		fmt.Fprintf(out, "  %s %q: %s\n", name, field.Label, choices)
	}
}
