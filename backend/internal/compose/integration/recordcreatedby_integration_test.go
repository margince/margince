// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// captured_by on a record an agent wrote names only the passport. Every read of
// a contact, company or deal names the colleague the agent worked for and the
// tool, from the record's create audit row.

import (
	"context"
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var recordCreatorPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		"contact":               {Create: true, Read: true},
		"company":               {Create: true, Read: true},
		"deal":                  {Create: true, Read: true},
		"pipeline":              {Read: true},
		"installation_settings": {Read: true},
	},
	RowScope: principal.RowScopeTeam,
}

const creatorColleague = "Anna Weber"

func TestEveryReadOfAnAgentCreatedRecordNamesTheColleagueAndTheTool(t *testing.T) {
	e := Setup(t)
	e.WsExec(t, `UPDATE app_user SET display_name = $2 WHERE id = $1`, e.Rep1, creatorColleague)
	passport := seedGrantedPassport(t, e, e.Rep1, "Claude")
	agent := e.AgentOnPassport(t, e.Rep1, []ids.UUID{e.Team1}, recordCreatorPerms, ids.From[ids.PassportKind](passport))
	contact, company, deal := createEachRecord(agent, t, e)

	reader := e.As(e.Rep2, []ids.UUID{e.Team1}, recordCreatorPerms)
	for read, who := range everyCreatorRead(reader, t, e, contact, company, deal) {
		if who == nil {
			t.Errorf("%s: created_by is absent for a record whose create row exists", read)
			continue
		}
		if who.ActorType != crmcontracts.PrincipalAttributionActorTypeAgent {
			t.Errorf("%s: created_by.actor_type = %q, want agent", read, who.ActorType)
		}
		if who.OnBehalfOfName == nil || *who.OnBehalfOfName != creatorColleague {
			t.Errorf("%s: created_by.on_behalf_of_name = %v, want %q", read, who.OnBehalfOfName, creatorColleague)
		}
		if who.AgentClient == nil || *who.AgentClient != "Claude" {
			t.Errorf("%s: created_by.agent_client = %v, want the passport's client Claude", read, who.AgentClient)
		}
	}
}

func TestEveryReadOfAHumanCreatedRecordNamesTheHuman(t *testing.T) {
	e := Setup(t)
	e.WsExec(t, `UPDATE app_user SET display_name = $2 WHERE id = $1`, e.Rep1, creatorColleague)
	human := e.As(e.Rep1, []ids.UUID{e.Team1}, recordCreatorPerms)
	contact, company, deal := createEachRecord(human, t, e)

	reader := e.As(e.Rep2, []ids.UUID{e.Team1}, recordCreatorPerms)
	for read, who := range everyCreatorRead(reader, t, e, contact, company, deal) {
		if who == nil {
			t.Errorf("%s: created_by is absent for a record whose create row exists", read)
			continue
		}
		if who.ActorType != crmcontracts.PrincipalAttributionActorTypeHuman || who.ActorId != "human:"+e.Rep1.String() {
			t.Errorf("%s: created_by = %s %s, want the human Rep1", read, who.ActorType, who.ActorId)
		}
		if who.ActorName == nil || *who.ActorName != creatorColleague {
			t.Errorf("%s: created_by.actor_name = %v, want %q", read, who.ActorName, creatorColleague)
		}
		if who.OnBehalfOfName != nil || who.AgentClient != nil {
			t.Errorf("%s: a human's own create names an agent: %+v", read, *who)
		}
	}
}

// A reader the record is closed to learns nothing of it, its creator included.
func TestAReaderWithoutTheRecordGetsNoCreator(t *testing.T) {
	e := Setup(t)
	agent := e.AgentFor(t, e.Rep3, []ids.UUID{e.Team2}, recordCreatorPerms)
	_, company, deal := createEachRecord(agent, t, e)
	e.MakeCapturePrivate(t, "company", company, e.Rep3)

	outsider := e.As(e.Rep1, []ids.UUID{e.Team1}, recordCreatorPerms)
	if _, err := e.Contacts.GetCompany(outsider, ids.From[ids.CompanyKind](company), storekit.LiveOnly); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("GET a capture-private company → %v, want ErrNotFound", err)
	}
	listed, _, err := e.Contacts.ListCompanies(outsider, contacts.ListCompaniesInput{IDs: []ids.UUID{company}})
	if err != nil || len(listed) != 0 {
		t.Errorf("list of a capture-private company → %d rows, %v; want none", len(listed), err)
	}
	noDeals := e.As(e.Rep1, []ids.UUID{e.Team1}, withoutGrant(recordCreatorPerms, "deal"))
	if _, err := e.Deals.GetDeal(noDeals, ids.From[ids.DealKind](deal), storekit.LiveOnly); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("GET a deal without deal:read → %v, want ErrPermissionDenied", err)
	}
	if page, _, err := e.Deals.ListDeals(noDeals, deals.ListDealsInput{IDs: []ids.UUID{deal}}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("list deals without deal:read → %d rows, %v; want ErrPermissionDenied", len(page), err)
	}
}

// createEachRecord is one contact, company and deal, each created by ctx.
func createEachRecord(ctx context.Context, t *testing.T, e *Env) (contact, company, deal ids.UUID) {
	t.Helper()
	pipeline, open, _ := DealFixture(t, e)
	createdContact, err := e.Contacts.CreateContact(ctx, contacts.CreateContactInput{FullName: "Met At A Fair", Source: "manual"})
	if err != nil {
		t.Fatalf("creating the contact: %v", err)
	}
	createdCompany, err := e.Contacts.CreateCompany(ctx, contacts.CreateCompanyInput{DisplayName: "Fair Exhibitor GmbH"})
	if err != nil {
		t.Fatalf("creating the company: %v", err)
	}
	createdDeal, err := e.Deals.CreateDeal(ctx, deals.CreateDealInput{Name: "Fair Follow-up", PipelineID: pipeline, StageID: open})
	if err != nil {
		t.Fatalf("creating the deal: %v", err)
	}
	return ids.UUID(createdContact.Id), ids.UUID(createdCompany.Id), ids.UUID(createdDeal.Id)
}

// everyCreatorRead is created_by as each single read and each list answers it.
func everyCreatorRead(ctx context.Context, t *testing.T, e *Env, contact, company, deal ids.UUID) map[string]*crmcontracts.PrincipalAttribution {
	t.Helper()
	out := map[string]*crmcontracts.PrincipalAttribution{}
	gotContact, err := e.Contacts.GetContact(ctx, ids.From[ids.ContactKind](contact), storekit.LiveOnly)
	if err != nil {
		t.Fatalf("GET /contacts/{id}: %v", err)
	}
	out["GET /contacts/{id}"] = gotContact.CreatedBy
	name := "Met At A Fair"
	listedContacts, _, err := e.Contacts.ListContacts(ctx, contacts.ListContactsInput{Query: &name})
	if err != nil || len(listedContacts) != 1 {
		t.Fatalf("GET /contacts → %d rows, %v; want the one contact", len(listedContacts), err)
	}
	out["GET /contacts"] = listedContacts[0].CreatedBy
	gotCompany, err := e.Contacts.GetCompany(ctx, ids.From[ids.CompanyKind](company), storekit.LiveOnly)
	if err != nil {
		t.Fatalf("GET /companies/{id}: %v", err)
	}
	out["GET /companies/{id}"] = gotCompany.CreatedBy
	listedCompanies, _, err := e.Contacts.ListCompanies(ctx, contacts.ListCompaniesInput{IDs: []ids.UUID{company}})
	if err != nil || len(listedCompanies) != 1 {
		t.Fatalf("GET /companies → %d rows, %v; want the one company", len(listedCompanies), err)
	}
	out["GET /companies"] = listedCompanies[0].CreatedBy
	gotDeal, err := e.Deals.GetDeal(ctx, ids.From[ids.DealKind](deal), storekit.LiveOnly)
	if err != nil {
		t.Fatalf("GET /deals/{id}: %v", err)
	}
	out["GET /deals/{id}"] = gotDeal.CreatedBy
	listedDeals, _, err := e.Deals.ListDeals(ctx, deals.ListDealsInput{IDs: []ids.UUID{deal}})
	if err != nil || len(listedDeals) != 1 {
		t.Fatalf("GET /deals → %d rows, %v; want the one deal", len(listedDeals), err)
	}
	out["GET /deals"] = listedDeals[0].CreatedBy
	return out
}
