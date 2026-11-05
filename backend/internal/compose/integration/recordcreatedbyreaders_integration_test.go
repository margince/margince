// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Which agents act for a colleague is that colleague's personal data. Only a
// human session reads a record's creator, and never the passport behind it.

import (
	"reflect"
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAnAgentReadingRecordsGetsNoCreator(t *testing.T) {
	e := Setup(t)
	human := e.As(e.Rep1, []ids.UUID{e.Team1}, recordCreatorPerms)
	contact, company, deal := createEachRecord(human, t, e)
	body := "Asked for the renewal terms."
	note, _, err := e.Activities.LogActivity(human, activities.LogActivityInput{
		Kind: "note", Body: &body, Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("logging the note: %v", err)
	}

	agent := e.AgentFor(t, e.Rep2, []ids.UUID{e.Team1}, recordCreatorPerms)
	reads := everyCreatorRead(agent, t, e, contact, company, deal)
	got, err := e.Activities.GetActivity(agent, ids.From[ids.ActivityKind](ids.UUID(note.Id)), storekit.LiveOnly)
	if err != nil {
		t.Fatalf("GET /activities/{id}: %v", err)
	}
	reads["GET /activities/{id}"] = got.CreatedBy
	entity := "contact"
	listed, _, err := e.Activities.ListActivities(agent, activities.ListActivitiesInput{EntityType: &entity, EntityID: &contact})
	if err != nil || len(listed) != 1 {
		t.Fatalf("GET /activities → %d rows, %v; want the one note", len(listed), err)
	}
	reads["GET /activities"] = listed[0].CreatedBy
	for read, who := range reads {
		if who != nil {
			t.Errorf("%s: an agent reads the creator %+v", read, *who)
		}
	}
}

// A write's echo is a read, and names the creator the GET names.
func TestAWriteEchoNamesTheCreatorAsTheGetDoes(t *testing.T) {
	e := Setup(t)
	human := e.As(e.Rep1, []ids.UUID{e.Team1}, recordCreatorPerms)
	contact, company, deal := createEachRecord(human, t, e)
	renamed := "Renamed"
	echoes := map[string]*crmcontracts.PrincipalAttribution{}
	updatedContact, err := e.Contacts.UpdateContact(human, ids.From[ids.ContactKind](contact), contacts.UpdateContactInput{FullName: &renamed})
	if err != nil {
		t.Fatalf("PATCH /contacts/{id}: %v", err)
	}
	echoes["contact"] = updatedContact.CreatedBy
	updatedCompany, err := e.Contacts.UpdateCompany(human, ids.From[ids.CompanyKind](company), contacts.UpdateCompanyInput{DisplayName: &renamed})
	if err != nil {
		t.Fatalf("PATCH /companies/{id}: %v", err)
	}
	echoes["company"] = updatedCompany.CreatedBy
	updatedDeal, err := e.Deals.UpdateDeal(human, ids.From[ids.DealKind](deal), deals.UpdateDealInput{Name: &renamed})
	if err != nil {
		t.Fatalf("PATCH /deals/{id}: %v", err)
	}
	echoes["deal"] = updatedDeal.CreatedBy

	gotContact, err := e.Contacts.GetContact(human, ids.From[ids.ContactKind](contact), storekit.LiveOnly)
	if err != nil {
		t.Fatalf("GET /contacts/{id}: %v", err)
	}
	gotCompany, err := e.Contacts.GetCompany(human, ids.From[ids.CompanyKind](company), storekit.LiveOnly)
	if err != nil {
		t.Fatalf("GET /companies/{id}: %v", err)
	}
	gotDeal, err := e.Deals.GetDeal(human, ids.From[ids.DealKind](deal), storekit.LiveOnly)
	if err != nil {
		t.Fatalf("GET /deals/{id}: %v", err)
	}
	gets := map[string]*crmcontracts.PrincipalAttribution{
		"contact": gotContact.CreatedBy, "company": gotCompany.CreatedBy, "deal": gotDeal.CreatedBy,
	}
	for kind, echo := range echoes {
		if echo == nil || !reflect.DeepEqual(echo, gets[kind]) {
			t.Errorf("%s: the PATCH echo names %+v, the GET names %+v", kind, echo, gets[kind])
		}
	}
}

// The name is the creator's row as it is now, never a copy the audit log kept.
func TestACreatorsNameIsTheirRowAsItIsNow(t *testing.T) {
	e := Setup(t)
	e.WsExec(t, `UPDATE app_user SET display_name = $2 WHERE id = $1`, e.Rep1, creatorColleague)
	human := e.As(e.Rep1, []ids.UUID{e.Team1}, recordCreatorPerms)
	_, company, _ := createEachRecord(human, t, e)
	const erased = "Erased member"
	e.WsExec(t, `UPDATE app_user SET display_name = $2 WHERE id = $1`, e.Rep1, erased)

	got, err := e.Contacts.GetCompany(human, ids.From[ids.CompanyKind](company), storekit.LiveOnly)
	if err != nil {
		t.Fatalf("GET /companies/{id}: %v", err)
	}
	if got.CreatedBy == nil || got.CreatedBy.ActorName == nil || *got.CreatedBy.ActorName != erased {
		t.Errorf("created_by = %+v, want the name %q the row holds now, not %q", got.CreatedBy, erased, creatorColleague)
	}
}

// A field mask withholds money and references, never who wrote the row. The
// row's captured_by and owner stay on the wire, so its creator does too.
func TestAFieldMaskedDealStillNamesItsCreator(t *testing.T) {
	e := Setup(t)
	human := e.As(e.Rep1, []ids.UUID{e.Team1}, recordCreatorPerms)
	_, _, deal := createEachRecord(human, t, e)
	masked := recordCreatorPerms
	masked.FieldMasks = []principal.FieldMask{{Object: "deal", Field: "amount_minor", Condition: principal.MaskAlways}}

	got, err := e.Deals.GetDeal(e.As(e.Rep2, []ids.UUID{e.Team1}, masked), ids.From[ids.DealKind](deal), storekit.LiveOnly)
	if err != nil {
		t.Fatalf("GET /deals/{id}: %v", err)
	}
	if got.MaskedFields == nil || !slices.Contains(*got.MaskedFields, "amount_minor") {
		t.Fatalf("masked_fields = %v; the mask did not apply, so the case tests nothing", got.MaskedFields)
	}
	if got.CreatedBy == nil || got.CreatedBy.ActorId != "human:"+e.Rep1.String() {
		t.Errorf("created_by on a masked deal = %+v, want Rep1", got.CreatedBy)
	}
}
