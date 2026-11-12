// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func readRecordWith(t *testing.T, page []datasource.Record, sidecars RecordSidecarsFor, recordType string, id ids.UUID) (readRecordAnswer, error) {
	t.Helper()
	r := NewRegistry(nil, nil, WithRecordSidecars(sidecars))
	RegisterCoreTools(r, &pageProbeProvider{page: page}, nil, nil, nil, nil, nil)
	raw, err := r.tools["read_record"].Handle(humanCtx(ids.NewV7()),
		json.RawMessage(`{"record_type":"`+recordType+`","id":"`+id.String()+`"}`))
	if err != nil {
		return readRecordAnswer{}, err
	}
	var out readRecordAnswer
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out, nil
}

type readRecordAnswer struct {
	ProfileFields []recordProfileField `json:"profile_fields"`
	Facts         []recordFact         `json:"facts"`
}

func TestReadRecordAnswersTheProfileFieldsAndFactsACompanyPageShows(t *testing.T) {
	company := ids.NewV7()
	confirmedAt := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	page := []datasource.Record{{
		Ref:    datasource.EntityRef{Type: datasource.EntityCompany, ID: company},
		Fields: json.RawMessage(`{"display_name":"Acme"}`),
	}}
	sidecars := func(_ context.Context, recordType datasource.EntityType, id ids.UUID) (RecordSidecars, error) {
		if recordType != datasource.EntityCompany || id != company {
			t.Fatalf("read the sidecars of %s %s, want the company read", recordType, id)
		}
		return RecordSidecars{
			CompanyProfileFields: []crmcontracts.CompanyProfileField{
				{Field: crmcontracts.CompanyProfileFieldFieldRegisterVat, Value: "DE123456789", Source: crmcontracts.CompanyProfileFieldSourceHuman},
				{Field: crmcontracts.CompanyProfileFieldFieldLegalForm, Value: "GmbH", Source: crmcontracts.CompanyProfileFieldSourceSiteRead},
			},
			CompanyFacts: []crmcontracts.CompanyFact{{
				Category: crmcontracts.CompanyFactCategoryCompany, Field: crmcontracts.CompanyFactFieldPhone,
				Value: "+49 30 1234", ValueKey: "+49301234", Source: crmcontracts.CompanyFactSourceSiteRead, VerifiedAt: &confirmedAt,
			}},
		}, nil
	}
	got, err := readRecordWith(t, page, sidecars, "company", company)
	if err != nil {
		t.Fatal(err)
	}
	wantFields := []recordProfileField{
		{Field: "register_vat", Value: "DE123456789", Source: "human", Confirmed: true},
		{Field: "legal_form", Value: "GmbH", Source: "site_read", Confirmed: false},
	}
	if len(got.ProfileFields) != len(wantFields) || got.ProfileFields[0] != wantFields[0] || got.ProfileFields[1] != wantFields[1] {
		t.Errorf("profile_fields = %+v, want %+v", got.ProfileFields, wantFields)
	}
	wantFact := recordFact{FactKey: "phone:+49301234", Category: "company", Field: "phone", Value: "+49 30 1234", Source: "site_read", Confirmed: true}
	if len(got.Facts) != 1 || got.Facts[0] != wantFact {
		t.Errorf("facts = %+v, want [%+v]", got.Facts, wantFact)
	}
}

func TestReadRecordAddsOnlyTheContactProfileFieldsNoContactFieldCarries(t *testing.T) {
	contact := ids.NewV7()
	suppressed := crmcontracts.ContactProfileFieldVerdictSuppressed
	page := []datasource.Record{{
		Ref:    datasource.EntityRef{Type: datasource.EntityContact, ID: contact},
		Fields: json.RawMessage(`{"full_name":"Ada Lovelace","title":"CTO"}`),
	}}
	sidecars := func(context.Context, datasource.EntityType, ids.UUID) (RecordSidecars, error) {
		return RecordSidecars{ContactProfileFields: []crmcontracts.ContactProfileField{
			{Field: crmcontracts.ContactProfileFieldFieldTitle, Value: "Chief Technology Officer", Source: "capture_enrich", CapturedBy: "agent:enrich"},
			{Field: crmcontracts.ContactProfileFieldFieldRole, Value: "Decision maker", Source: "capture_enrich", CapturedBy: "human:019ff000-0000-7000-8000-000000000001"},
			{Field: crmcontracts.ContactProfileFieldFieldWebsite, Value: "ada.example", Source: "site_read", CapturedBy: "agent:enrich", Verdict: &suppressed},
		}}, nil
	}
	got, err := readRecordWith(t, page, sidecars, "contact", contact)
	if err != nil {
		t.Fatal(err)
	}
	want := recordProfileField{Field: "role", Value: "Decision maker", Source: "capture_enrich", Confirmed: true}
	if len(got.ProfileFields) != 1 || got.ProfileFields[0] != want {
		t.Errorf("profile_fields = %+v, want only [%+v]: title is a contact field and website was suppressed", got.ProfileFields, want)
	}
}

func TestReadRecordAnswersTheSidecarReadsRefusal(t *testing.T) {
	company := ids.NewV7()
	page := []datasource.Record{{
		Ref: datasource.EntityRef{Type: datasource.EntityCompany, ID: company}, Fields: json.RawMessage(`{}`),
	}}
	refuse := func(context.Context, datasource.EntityType, ids.UUID) (RecordSidecars, error) {
		return RecordSidecars{}, apperrors.ErrNotFound
	}
	if _, err := readRecordWith(t, page, refuse, "company", company); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("read_record answered %v, want the sidecar read's not-found", err)
	}
}
