// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The account page's timeline reads through the activity list, so it carries
// the custom-field values the list does.
func TestTheCompanyTimelineCarriesActivityCustomFieldValues(t *testing.T) {
	e := integration.Setup(t)
	catalog := customfields.NewService(e.Pool, integration.SchemaPool(t))
	field, err := catalog.Create(e.As(e.Rep1, nil, integration.CustomFieldAdminPerms), customfields.FieldSpec{
		Object: "activity", Label: "Visit note", Type: customfields.TypeText, Source: "manual",
	})
	if err != nil {
		t.Fatalf("create the activity field: %v", err)
	}
	column := *field.ColumnName
	store := activities.NewStore(e.DB()).WithFieldCatalog(catalog)
	company := e.SeedCompany(t, "Halden Werke", nil)
	subject := "Site notes"
	if _, _, err := store.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "note", Subject: &subject, Source: "manual",
		Links:        []activities.ActivityLinkInput{{EntityType: "company", EntityID: company}},
		CustomFields: map[string]any{column: "second shift starts at six"},
	}); err != nil {
		t.Fatalf("log: %v", err)
	}

	page, err := company360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects,
		approvals.NewService(e.DB()), func() time.Time { return clocktest.Now(t) }).WithActivityFields(store).
		Assemble(e.Admin(), ids.From[ids.CompanyKind](company))
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if page.Activities == nil || len(page.Activities.Data) != 1 {
		t.Fatalf("the account timeline holds %v, want the one note", page.Activities)
	}
	if got := page.Activities.Data[0].AdditionalProperties[column]; got != "second shift starts at six" {
		t.Errorf("the account timeline's note carries %v under %s, want the logged value", got, column)
	}
}
