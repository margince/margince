// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"slices"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const profileFieldContract = "../../../api/crm.yaml"

// profileFieldsTheAPIAccepts is the field vocabulary of
// PATCH /companies/{id}/profile-fields/{field}. It is read from the contract, so
// a field added there is held to the History without a second list.
func profileFieldsTheAPIAccepts(t *testing.T) []string {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromFile(profileFieldContract)
	if err != nil {
		t.Fatalf("load contract: %v", err)
	}
	param := doc.Components.Parameters["ProfileFieldKey"]
	if param == nil || param.Value == nil || param.Value.Schema == nil {
		t.Fatal("the ProfileFieldKey parameter is missing from the contract")
	}
	var fields []string
	for _, raw := range param.Value.Schema.Value.Enum {
		name, ok := raw.(string)
		if !ok {
			t.Fatalf("ProfileFieldKey has a non-string member %#v", raw)
		}
		fields = append(fields, name)
	}
	if len(fields) < 15 {
		t.Fatalf("read only %d profile fields from the contract; the census is too short to hold anything", len(fields))
	}
	return fields
}

// Every field the API accepts shows in both histories, old value and new.
func TestEveryProfileFieldAUserCanEditAppearsInTheCompanyHistory(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "History Check GmbH", &e.Rep3)
	companyID := ids.From[ids.CompanyKind](company)
	fields := profileFieldsTheAPIAccepts(t)

	for _, field := range fields {
		first, second := "first "+field, "second "+field
		for _, value := range []string{first, second} {
			if _, err := e.Contacts.UpdateCompanyProfileField(e.Admin(), companyID, field,
				contacts.ProfileFieldWriteInput{Value: &value}); err != nil {
				t.Fatalf("stating %s: %v", field, err)
			}
		}

		changes, err := privacy.ListFieldHistory(e.Admin(), e.DB(), privacy.FieldHistoryFilter{
			EntityType: "company", EntityID: company, Field: &field,
		})
		if err != nil {
			t.Fatalf("reading the changes: %v", err)
		}
		var olds, news []string
		for _, entry := range changes.Entries {
			if entry.OldValue != nil {
				olds = append(olds, *entry.OldValue)
			}
			if entry.NewValue != nil {
				news = append(news, *entry.NewValue)
			}
		}
		if !slices.Contains(news, second) || !slices.Contains(olds, first) {
			t.Errorf("%s: Changes shows old values %v and new values %v, want %q replaced by %q",
				field, olds, news, first, second)
		}
	}

	full, err := privacy.ListRecordHistory(e.Admin(), e.DB(), privacy.RecordHistoryFilter{
		EntityType: "company", EntityID: company, Limit: new(200),
	})
	if err != nil {
		t.Fatalf("reading the full history: %v", err)
	}
	for _, field := range fields {
		shown := false
		for _, entry := range full.Entries {
			if after, ok := entry.After[field]; ok && after == "second "+field {
				shown = true
				if entry.Action != "update" || entry.Before[field] != "first "+field {
					t.Errorf("%s: the line reads %q from %v, want an update from %q", field, entry.Action, entry.Before[field], "first "+field)
				}
			}
		}
		if !shown {
			t.Errorf("%s: the full history has no line for the correction", field)
		}
	}
}

func TestACompanyFactChangeAndRemovalAppearInItsHistory(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Fact History GmbH", &e.Rep3)
	companyID := ids.From[ids.CompanyKind](company)

	fact, err := e.Contacts.CreateCompanyFact(e.Admin(), companyID,
		contacts.FactCreateInput{Category: "company", Field: "employee_range", Value: "11-50"})
	if err != nil {
		t.Fatalf("stating the fact: %v", err)
	}
	key := "employee_range:" + fact.ValueKey
	corrected := "51-200"
	if _, err := e.Contacts.UpdateCompanyFact(e.Admin(), companyID, key,
		contacts.FactWriteInput{Value: &corrected}); err != nil {
		t.Fatalf("correcting the fact: %v", err)
	}
	if err := e.Contacts.DeleteCompanyFact(e.Admin(), companyID, key, contacts.FactWriteInput{}); err != nil {
		t.Fatalf("removing the fact: %v", err)
	}

	field := "employee_range"
	changes, err := privacy.ListFieldHistory(e.Admin(), e.DB(), privacy.FieldHistoryFilter{
		EntityType: "company", EntityID: company, Field: &field,
	})
	if err != nil {
		t.Fatalf("reading the changes: %v", err)
	}
	if len(changes.Entries) != 2 || changes.Entries[0].OldValue == nil || *changes.Entries[0].OldValue != "11-50" ||
		changes.Entries[0].NewValue == nil || *changes.Entries[0].NewValue != corrected {
		t.Errorf("Changes shows %+v, want the fact stated and then corrected from 11-50 to 51-200", changes.Entries)
	}

	full, err := privacy.ListRecordHistory(e.Admin(), e.DB(), privacy.RecordHistoryFilter{
		EntityType: "company", EntityID: company, Limit: new(50),
	})
	if err != nil {
		t.Fatalf("reading the full history: %v", err)
	}
	newest := full.Entries[0]
	if newest.Before[field] != corrected || newest.After != nil {
		t.Errorf("the newest line is %+v, want the removal of the fact that read %q", newest, corrected)
	}
}
