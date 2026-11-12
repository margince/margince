// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// read_record answers what a company or contact page shows beside the record's
// own row. It reads through the page's own readers, so it shows what the reader may see.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// sidecarRead is the part of a read_record answer these tests judge.
type sidecarRead struct {
	ProfileFields []struct {
		Field, Value, Source string
		Confirmed            bool
	} `json:"profile_fields"`
	Facts []struct {
		Field string `json:"field"`
		Value string `json:"value"`
	} `json:"facts"`
}

func readRecordAs(ctx context.Context, t *testing.T, registry *agents.Registry, recordType string, id ids.UUID) (sidecarRead, error) {
	t.Helper()
	out, err := registry.Invoke(ctx, "read_record", json.RawMessage(`{"record_type":"`+recordType+`","id":"`+id.String()+`"}`))
	if err != nil {
		return sidecarRead{}, err
	}
	var read struct {
		Data sidecarRead `json:"data"`
	}
	if err := json.Unmarshal(out, &read); err != nil {
		t.Fatal(err)
	}
	return read.Data, nil
}

func TestReadRecordAnswersTheVATIDAndTheFactsOfACompanyPage(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Imprint Check GmbH", &e.Rep3)
	companyID := ids.From[ids.CompanyKind](company)
	vat := "DE123456789"
	if _, err := e.Contacts.UpdateCompanyProfileField(e.Admin(), companyID, "register_vat",
		contacts.ProfileFieldWriteInput{Value: &vat}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Contacts.CreateCompanyFact(e.Admin(), companyID,
		contacts.FactCreateInput{Category: "company", Field: "founded_year", Value: "1998"}); err != nil {
		t.Fatal(err)
	}
	registry := compose.NewRegistry(e.Pool, compose.SendPath{})
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, ContractRepPerms)

	read, err := readRecordAs(reader, t, registry, "company", company)
	if err != nil {
		t.Fatal(err)
	}
	vatFound := false
	for _, pf := range read.ProfileFields {
		vatFound = vatFound || (pf.Field == "register_vat" && pf.Value == vat && pf.Source == "human" && pf.Confirmed)
	}
	if !vatFound {
		t.Errorf("read_record answered profile_fields %+v, want register_vat %s stated by a human", read.ProfileFields, vat)
	}
	if len(read.Facts) != 1 || read.Facts[0].Field != "founded_year" || read.Facts[0].Value != "1998" {
		t.Errorf("read_record answered facts %+v, want the founding year 1998", read.Facts)
	}

	e.WsExec(t, `UPDATE company SET visibility = 'owner' WHERE id = $1`, company)
	if _, err := readRecordAs(reader, t, registry, "company", company); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a rep who cannot see the company got %v from read_record, want not found", err)
	}
}

func TestReadRecordAnswersAContactsProfileFieldsAsItsPageShowsThem(t *testing.T) {
	e := Setup(t)
	contact := e.SeedContact(t, "Ada Lovelace", &e.Rep1)
	owner := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	if _, err := e.Contacts.SaveResearchClaims(owner, ids.From[ids.ContactKind](contact), []contacts.ResearchClaimInput{
		{Field: "role", Value: "Decision maker", Quote: "Ada decides on tooling", SourceURL: "https://example.test/team"},
		{Field: "website", Value: "https://ada.example.test", Quote: "ada.example.test", SourceURL: "https://example.test/team"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := ai.NewFeedbackStore(e.DB()).Record(owner, ai.RecordInput{
		SubjectType: "contact", SubjectID: contact, ClaimKind: ai.ClaimProfileField,
		ClaimPath: ai.ProfileFieldClaimPath("website"), Verdict: ai.VerdictSuppressed,
	}); err != nil {
		t.Fatal(err)
	}
	registry := compose.NewRegistry(e.Pool, compose.SendPath{})
	colleague := e.As(e.Rep2, []ids.UUID{e.Team1}, RepPerms)

	read, err := readRecordAs(colleague, t, registry, "contact", contact)
	if err != nil {
		t.Fatal(err)
	}
	if len(read.ProfileFields) != 1 || read.ProfileFields[0].Field != "role" || read.ProfileFields[0].Value != "Decision maker" {
		t.Errorf("read_record answered profile_fields %+v, want the role alone: the website claim was suppressed", read.ProfileFields)
	}

	e.WsExec(t, `UPDATE contact SET visibility = 'owner' WHERE id = $1`, contact)
	if _, err := readRecordAs(colleague, t, registry, "contact", contact); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a colleague who cannot see the contact got %v from read_record, want not found", err)
	}
}
