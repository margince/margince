// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The growth fit judges a company against the offering chosen for it. These
// tests drive the real store, the real company-context preparation and the real
// cache; only the model is a recording stand-in.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/companydossier"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// offeringLane stands in for the model. It sends each request through the real
// company-context preparation first, so the prompt it records is the one a
// provider would receive.
type offeringLane struct {
	provider *companyContextProvider
	prompts  []string
	reply    string
}

func (l *offeringLane) Complete(ctx context.Context, req model.Request) (model.Response, error) {
	prepared, err := l.provider.Prepare(ctx, ai.TaskGrowthFit, req)
	if err != nil {
		return model.Response{}, err
	}
	var prompt strings.Builder
	for _, m := range prepared.Messages {
		prompt.WriteString(m.Content)
	}
	l.prompts = append(l.prompts, prompt.String())
	return model.Response{Text: l.reply}, nil
}

type offeringEnv struct {
	*integration.Env
	ctx     context.Context
	lane    *offeringLane
	service *companydossier.GrowthFitService
}

var offeringAdmin = principal.Permissions{
	RoleKeys: []string{"admin"},
	Objects: map[string]principal.ObjectGrant{
		"company":      {Create: true, Read: true, Update: true, Delete: true},
		"legal_entity": {Create: true, Read: true, Update: true, Delete: true},
	},
	RowScope: principal.RowScopeAll,
}

func setupOffering(t *testing.T) *offeringEnv {
	t.Helper()
	e := integration.Setup(t)
	ctx := e.As(e.AdminUser, nil, offeringAdmin)
	store := e.Contacts
	if _, err := store.SaveCompany(ctx, contacts.SaveCompanyInput{
		DisplayName: "Heatwave AG",
		Fields: map[string]*string{
			"offer_summary": new("Industrial heat pumps"),
			"icp":           new("Plant operators with process heat"),
		},
	}); err != nil {
		t.Fatalf("describe the main offering: %v", err)
	}
	lane := &offeringLane{provider: newCompanyContextProvider(store)}
	service := companydossier.NewGrowthFitService(e.Pool, store, offeringConfirmed(store), lane, "routing-1", func() time.Time { return clocktest.Now(t) })
	return &offeringEnv{Env: e, ctx: ctx, lane: lane, service: service}
}

// secondOffering creates a further legal entity. A nil icp leaves it
// unconfirmed.
func (o *offeringEnv) secondOffering(t *testing.T, icp *string) crmcontracts.LegalEntity {
	t.Helper()
	entity, err := o.Contacts.CreateLegalEntity(o.ctx, contacts.LegalEntityInput{
		DisplayName: new("Wien"), LegalName: new("Wien GmbH"), Country: new("at"),
		Offering: contacts.LegalEntityOffering{OfferSummary: new("Solar carports"), ICP: icp},
	})
	if err != nil {
		t.Fatalf("create the second offering: %v", err)
	}
	return contacts.LegalEntityWire(entity)
}

// customer is complete enough that the model is asked, and returns the id of
// one profile field the reply can cite.
func (o *offeringEnv) customer(t *testing.T, name string) (ids.CompanyID, string) {
	t.Helper()
	created, err := o.Contacts.CreateCompany(o.ctx, contacts.CreateCompanyInput{
		DisplayName: name, Industry: new("automotive"), Source: "manual",
	})
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	id := ids.From[ids.CompanyKind](ids.UUID(created.Id))
	for field, value := range map[string]string{
		"offer_summary": "Car parts", "icp": "Dealers", "buying_center": "Head of Purchasing",
		"buying_intents": "Cutting energy cost",
	} {
		if _, err := o.Contacts.UpdateCompanyProfileField(o.ctx, id, field,
			contacts.ProfileFieldWriteInput{Value: new(value)}); err != nil {
			t.Fatalf("write %s of %s: %v", field, name, err)
		}
	}
	fields, err := o.Contacts.ListCompanyProfileFields(o.ctx, id)
	if err != nil || len(fields) == 0 {
		t.Fatalf("read the profile fields of %s: %v", name, err)
	}
	return id, fields[0].Id.String()
}

func (o *offeringEnv) judge(t *testing.T, id ids.CompanyID, citedField string) crmcontracts.CompanyGrowthFit {
	t.Helper()
	o.lane.reply = `{"band":"strong","positive_factors":[{"text":"They run hot processes.","nature":"fact",
		"evidence":[{"entity_type":"profile_field","entity_id":"` + citedField + `"}]}]}`
	fit, err := o.service.Get(o.ctx, id, false)
	if err != nil {
		t.Fatalf("growth fit: %v", err)
	}
	return fit
}

func (o *offeringEnv) choose(t *testing.T, id ids.CompanyID, entity ids.LegalEntityID) {
	t.Helper()
	if _, err := o.Contacts.UpdateCompany(o.ctx, id, contacts.UpdateCompanyInput{AssessAgainstEntityID: &entity}); err != nil {
		t.Fatalf("choose the offering: %v", err)
	}
}

func entityID(e crmcontracts.LegalEntity) ids.LegalEntityID {
	return ids.From[ids.LegalEntityKind](ids.UUID(e.Id))
}

func TestACompanyIsJudgedAgainstTheOfferingChosenForIt(t *testing.T) {
	o := setupOffering(t)
	wien := o.secondOffering(t, new("Car dealers with large lots"))
	company, cited := o.customer(t, "Autohaus Berger")

	main := o.judge(t, company, cited)
	if len(o.lane.prompts) != 1 || !strings.Contains(o.lane.prompts[0], "Industrial heat pumps") {
		t.Fatalf("the unchosen company was not judged against the main offering: %q", o.lane.prompts)
	}
	if main.AssessedAgainst == nil || !main.AssessedAgainst.IsDefault || main.AssessedAgainst.DisplayName != "Heatwave AG" {
		t.Fatalf("assessed_against = %+v, want the main offering", main.AssessedAgainst)
	}

	o.choose(t, company, entityID(wien))
	second := o.judge(t, company, cited)
	if len(o.lane.prompts) != 2 {
		t.Fatalf("choosing another offering made %d model calls in total, want 2", len(o.lane.prompts))
	}
	prompt := o.lane.prompts[1]
	if !strings.Contains(prompt, "Solar carports") || strings.Contains(prompt, "Industrial heat pumps") {
		t.Fatalf("the prompt for the chosen offering = %q, want the entity's text and not the main one's", prompt)
	}
	if second.AssessedAgainst == nil || second.AssessedAgainst.IsDefault || second.AssessedAgainst.DisplayName != "Wien" {
		t.Fatalf("assessed_against = %+v, want Wien", second.AssessedAgainst)
	}

	o.judge(t, company, cited)
	if len(o.lane.prompts) != 2 {
		t.Fatalf("an unchanged choice asked the model again (%d calls)", len(o.lane.prompts))
	}
}

func TestChoosingTheDefaultEntityReturnsToTheMainOffering(t *testing.T) {
	o := setupOffering(t)
	wien := o.secondOffering(t, new("Car dealers with large lots"))
	company, cited := o.customer(t, "Autohaus Berger")
	o.choose(t, company, entityID(wien))
	o.judge(t, company, cited)

	var defaultID ids.LegalEntityID
	if err := o.Pool.QueryRow(context.Background(), `SELECT id FROM legal_entity WHERE is_default`).Scan(&defaultID); err != nil {
		t.Fatalf("read the default entity: %v", err)
	}
	o.choose(t, company, defaultID)
	back := o.judge(t, company, cited)

	if len(o.lane.prompts) != 2 || !strings.Contains(o.lane.prompts[1], "Industrial heat pumps") {
		t.Fatalf("returning to the main offering did not re-judge against it: %q", o.lane.prompts)
	}
	if back.AssessedAgainst == nil || !back.AssessedAgainst.IsDefault {
		t.Fatalf("assessed_against = %+v, want the main offering", back.AssessedAgainst)
	}
	var stored *ids.UUID
	if err := o.Pool.QueryRow(context.Background(), `SELECT assess_against_entity_id FROM company WHERE id = $1`, company).Scan(&stored); err != nil || stored != nil {
		t.Fatalf("stored choice = %v (%v), want NULL for the main offering", stored, err)
	}
}

func TestEditingAnOfferingReJudgesOnlyTheCompaniesChosenAgainstIt(t *testing.T) {
	o := setupOffering(t)
	wien := o.secondOffering(t, new("Car dealers with large lots"))
	pointing, citedPointing := o.customer(t, "Autohaus Berger")
	other, citedOther := o.customer(t, "Autohaus Keller")
	o.choose(t, pointing, entityID(wien))
	o.judge(t, pointing, citedPointing)
	o.judge(t, other, citedOther)
	calls := len(o.lane.prompts)

	if _, err := o.Contacts.UpdateLegalEntity(o.ctx, entityID(wien), contacts.LegalEntityInput{
		Offering: contacts.LegalEntityOffering{OfferSummary: new("Solar carports and wall boxes")},
	}, nil); err != nil {
		t.Fatalf("edit the offering: %v", err)
	}

	o.judge(t, other, citedOther)
	if len(o.lane.prompts) != calls {
		t.Fatalf("editing Wien's offering re-judged a company that is not chosen against it")
	}
	o.judge(t, pointing, citedPointing)
	if len(o.lane.prompts) != calls+1 || !strings.Contains(o.lane.prompts[calls], "wall boxes") {
		t.Fatalf("editing Wien's offering did not re-judge the company chosen against it: %q", o.lane.prompts[calls:])
	}
}

func TestAnOfferingWithoutAnIdealCustomerCapsTheBand(t *testing.T) {
	o := setupOffering(t)
	wien := o.secondOffering(t, nil)
	company, cited := o.customer(t, "Autohaus Berger")
	o.choose(t, company, entityID(wien))

	fit := o.judge(t, company, cited)
	if fit.Band == crmcontracts.GrowthFitBandStrong || fit.BandCappedReason == nil {
		t.Fatalf("band = %s, reason = %v; an unconfirmed offering must cap the band", fit.Band, fit.BandCappedReason)
	}

	if _, err := o.Contacts.UpdateLegalEntity(o.ctx, entityID(wien), contacts.LegalEntityInput{
		Offering: contacts.LegalEntityOffering{ICP: new("Car dealers with large lots")},
	}, nil); err != nil {
		t.Fatalf("describe the ideal customer: %v", err)
	}
	if fit = o.judge(t, company, cited); fit.BandCappedReason != nil {
		t.Fatalf("a confirmed offering is still capped: %q", *fit.BandCappedReason)
	}
}
