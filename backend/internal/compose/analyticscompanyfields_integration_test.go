// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/compose/installseam"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// tierBook is a company picklist opted into reporting, and deals at companies
// that hold it. One Gold company is capture-private to Rep3, so the admin who
// asks may not open it.
type tierBook struct {
	field crmcontracts.CustomField
	name  string
}

func seedTierBook(t *testing.T, e *integration.Env) tierBook {
	t.Helper()
	catalog := customfields.NewService(e.Pool, integration.SchemaPool(t))
	field, err := catalog.Create(e.As(e.Rep1, nil, integration.CustomFieldAdminPerms), customfields.FieldSpec{
		Object: "company", Label: "Tier", Type: customfields.TypePicklist,
		Options: []string{"Gold", "Silver"}, Source: "manual",
	})
	if err != nil {
		t.Fatalf("create the tier field: %v", err)
	}
	column := *field.ColumnName
	companies := e.Contacts.WithFieldCatalog(catalog)
	company := func(name string, tier any) *ids.CompanyID {
		created, err := companies.CreateCompany(e.Admin(), contacts.CreateCompanyInput{
			DisplayName: name, CustomFields: map[string]any{column: tier},
		})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		id := ids.From[ids.CompanyKind](ids.UUID(created.Id))
		return &id
	}
	gold, silver, hidden, untiered := company("Gold A", "Gold"), company("Silver B", "Silver"),
		company("Hidden Gold", "Gold"), company("No tier", nil)

	pipeline, open, _ := integration.DealFixture(t, e)
	store := deals.NewStore(e.DB(), installseam.Deals())
	for i, at := range []*ids.CompanyID{gold, gold, silver, hidden, untiered, nil} {
		if _, err := store.CreateDeal(e.Admin(), deals.CreateDealInput{
			Name: fmt.Sprintf("Tier deal %d", i), PipelineID: pipeline, StageID: open, CompanyID: at,
		}); err != nil {
			t.Fatalf("seed deal %d: %v", i, err)
		}
	}
	e.MakeCapturePrivate(t, "company", hidden.UUID, e.Rep3)
	optIn(t, e, catalog, field)
	return tierBook{field: field, name: analyticsquery.CompanyCustomFieldPrefix + strings.TrimPrefix(column, "cf_")}
}

func askDeals(ctx context.Context, t *testing.T, e *integration.Env, q analyticsquery.Query) map[string]string {
	t.Helper()
	var answer AnalyticsAnswer
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		answer, err = RunAnalyticsQuery(ctx, tx, q, analyticsquery.Floor(1))
		return err
	}); err != nil {
		t.Fatalf("asking %+v: %v", q, err)
	}
	counts := map[string]string{}
	for _, row := range answer.Rows {
		counts[fmt.Sprint(row[q.GroupBy[0]])] = fmt.Sprint(row["count"])
	}
	return counts
}

// A company field groups deals through each deal's company. The deal at the
// company the reader may not open leaves the answer, so its tier is not
// counted. A deal with no company, or one whose company holds no tier, groups
// as empty.
func TestACompanyFieldGroupsDealsThroughTheirCompany(t *testing.T) {
	e := integration.Setup(t)
	book := seedTierBook(t, e)
	grouped := askDeals(e.Admin(), t, e, analyticsquery.Query{
		Entity: "deals-by-stage", GroupBy: []string{book.name},
		Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll}},
	})
	if want := map[string]string{"Gold": "2", "Silver": "1", "<nil>": "2"}; !maps.Equal(grouped, want) {
		t.Fatalf("deals by company tier %v, want %v: the hidden Gold company's deal must leave the answer", grouped, want)
	}

	filtered := askDeals(e.Admin(), t, e, analyticsquery.Query{
		Entity: "deals-by-stage", GroupBy: []string{book.name},
		Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll}},
		Filters:  []analyticsquery.Filter{{Field: book.name, Op: analyticsquery.OpEq, Value: "Gold"}},
	})
	if want := map[string]string{"Gold": "2"}; !maps.Equal(filtered, want) {
		t.Errorf("deals filtered to Gold companies %v, want %v without the hidden company's", filtered, want)
	}
}

// The schema says which record a field is on and how many deals hold a value,
// counted under the same company scope a grouping applies.
func TestTheSchemaSaysHowManyDealsHoldACompanyField(t *testing.T) {
	e := integration.Setup(t)
	book := seedTierBook(t, e)
	ctx := e.Admin()
	records := e.WsCount(t, `SELECT count(*) FROM deal WHERE archived_at IS NULL`)

	var schema analyticsquery.Schema
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		schema, err = AnalyticsSchemaWithCoverage(ctx, tx)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	described := analyticsCustomFieldsOf(schema.Entities["deals-by-stage"])
	if described == nil || len(*described) != 1 {
		t.Fatalf("the schema describes %v custom fields, want the tier alone", described)
	}
	got := (*described)[0]
	if got.Name != book.name || got.Object != crmcontracts.AnalyticsCustomFieldObjectCompany ||
		got.Records != records || got.WithValue != 3 {
		t.Errorf("the schema describes %+v, want %s on the company, held by 3 of %d deals", got, book.name, records)
	}

	document, err := analyticsVocabularyText(ctx, e.DB())
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf(`%s "Tier": ["Gold","Silver"], the deal's company's field, held by 3 of %d live deals you can read`,
		book.name, records)
	if !strings.Contains(document, want) {
		t.Errorf("the vocabulary a model reads does not say %q:\n%s", want, document)
	}
}
