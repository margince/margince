// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestSharingCompanyContactsGrantsOnlyTheReadableCurrentContactsTheCallerMayShare(t *testing.T) {
	e := Setup(t)
	company := companyIDOf(e.SeedCompany(t, "Current employer", &e.Rep1))
	owned := []ids.UUID{}
	for _, name := range []string{"Buyer", "Engineer", "Finance"} {
		contact := e.SeedContact(t, name, &e.Rep1)
		owned = append(owned, contact)
		shareCompanyEmployee(t, e, company, contact, false)
		e.MakeCapturePrivate(t, "contact", contact, e.Rep1)
	}
	visible := e.SeedContact(t, "Readable colleague contact", &e.Rep3)
	shareCompanyEmployee(t, e, company, visible, false)
	hidden := e.SeedContact(t, "Hidden colleague contact", &e.Rep3)
	shareCompanyEmployee(t, e, company, hidden, false)
	e.MakeCapturePrivate(t, "contact", hidden, e.Rep3)
	former := e.SeedContact(t, "Former employee", &e.Rep1)
	shareCompanyEmployee(t, e, company, former, true)
	archived := e.SeedContact(t, "Archived employee", &e.Rep1)
	shareCompanyEmployee(t, e, company, archived, false)
	if _, err := e.Contacts.ArchiveContact(e.Admin(), ContactIDOf(archived), nil); err != nil {
		t.Fatal(err)
	}
	perms := principal.Permissions{
		RoleKeys: []string{"rep"}, RowScope: principal.RowScopeOwn,
		Objects: map[string]principal.ObjectGrant{
			"contact": {Read: true, Update: true}, "company": {Read: true},
			"relationship": {Read: true}, "activity": {Read: true, Create: true},
		},
	}
	owner := e.As(e.Rep1, []ids.UUID{e.Team1}, perms)
	recipient := e.As(e.Rep2, []ids.UUID{e.Team1}, perms)
	for _, contact := range owned {
		if _, err := e.Contacts.GetContact(recipient, ContactIDOf(contact), storekit.LiveOnly); !errors.Is(err, apperrors.ErrNotFound) {
			t.Fatalf("team membership alone disclosed a private contact: %v", err)
		}
	}
	out, err := compose.ShareCompanyContacts(owner, e.DB(), identity.NewServiceFor(e.DB()), company, crmcontracts.ShareCompanyContactsRequest{
		SubjectType: "team", SubjectId: crmcontracts.Id(e.Team1), Access: "write",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Shared != 3 || out.Skipped != 1 {
		t.Fatalf("counts = %+v, want three shared and one readable contact skipped", out)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM record_grant WHERE record_type = 'contact' AND subject_id = $1`, e.Team1); got != 3 {
		t.Fatalf("grants = %d, want 3", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE action = 'record_share' AND actor_id = $1`, "human:"+e.Rep1.String()); got != 3 {
		t.Fatalf("share audit rows = %d, want 3", got)
	}
	for _, contact := range owned {
		if _, _, err := e.Activities.LogActivity(recipient, activities.LogActivityInput{
			Kind: "note", Body: new("Account follow-up"),
			Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
		}); err != nil {
			t.Fatalf("team member cannot log on a shared contact: %v", err)
		}
	}
	if _, err := e.Contacts.GetContact(recipient, ContactIDOf(hidden), storekit.LiveOnly); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("the bulk action disclosed an excluded private contact: %v", err)
	}
}

func shareCompanyEmployee(t *testing.T, e *Env, company ids.CompanyID, contact ids.UUID, ended bool) {
	t.Helper()
	in := contacts.CreateRelationshipInput{
		Kind: "employment", CompanyID: &company, ContactID: new(ContactIDOf(contact)), Source: "manual",
	}
	if ended {
		in.StartedAt = new(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))
		in.EndedAt = new(time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC))
	}
	if _, err := e.Contacts.CreateRelationship(e.Admin(), in); err != nil {
		t.Fatal(err)
	}
}

func TestSharingCompanyContactsValidatesRecipientsWithoutEligibleContacts(t *testing.T) {
	for _, populated := range []bool{false, true} {
		for _, missing := range []bool{false, true} {
			t.Run(fmt.Sprintf("contacts=%t/missing_recipient=%t", populated, missing), func(t *testing.T) {
				e := Setup(t)
				company := companyIDOf(e.SeedCompany(t, "Empty or unshareable", &e.Rep1))
				if populated {
					contact := e.SeedContact(t, "Another owner's contact", &e.Rep3)
					shareCompanyEmployee(t, e, company, contact, false)
				}
				recipient := e.Rep2
				want := apperrors.ErrSeatTierInsufficient
				if missing {
					recipient = ids.NewV7()
					want = apperrors.ErrNotFound
				} else {
					e.WsExec(t, `UPDATE app_user SET seat_type = 'read' WHERE id = $1`, recipient)
				}
				caller := e.As(e.Rep1, nil, principal.Permissions{
					RoleKeys: []string{"rep"}, RowScope: principal.RowScopeOwn,
					Objects: map[string]principal.ObjectGrant{
						"contact": {Read: true, Update: true}, "company": {Read: true}, "relationship": {Read: true},
					},
				})
				result, err := compose.ShareCompanyContacts(caller, e.DB(), identity.NewServiceFor(e.DB()), company, crmcontracts.ShareCompanyContactsRequest{
					SubjectType: "user", SubjectId: crmcontracts.Id(recipient), Access: "write",
				})
				if !errors.Is(err, want) || result.Shared != 0 || result.Skipped != 0 {
					t.Fatalf("result = %+v, err = %v, want %v and no successful counts", result, err, want)
				}
				if got := e.WsCount(t, `SELECT count(*) FROM record_grant`); got != 0 {
					t.Fatalf("invalid bulk request wrote %d grants", got)
				}
			})
		}
	}
}

func TestSharingCompanyContactsRestatesAnExistingColleagueGrantAtTheChosenLevel(t *testing.T) {
	e := Setup(t)
	company := companyIDOf(e.SeedCompany(t, "Shared account", &e.Rep1))
	contact := e.SeedContact(t, "Shared employee", &e.Rep1)
	shareCompanyEmployee(t, e, company, contact, false)
	e.MakeCapturePrivate(t, "contact", contact, e.Rep1)
	caller := e.As(e.Rep1, nil, principal.Permissions{
		RoleKeys: []string{"rep"}, RowScope: principal.RowScopeOwn,
		Objects: map[string]principal.ObjectGrant{
			"contact": {Read: true, Update: true}, "company": {Read: true}, "relationship": {Read: true},
		},
	})
	grants := identity.NewServiceFor(e.DB())
	if _, err := grants.CreateRecordGrant(caller, identity.CreateGrantInput{
		RecordType: "contact", RecordID: contact, SubjectType: "user", SubjectID: e.Rep2, Access: "write",
	}); err != nil {
		t.Fatal(err)
	}
	expires := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	result, err := compose.ShareCompanyContacts(caller, e.DB(), grants, company, crmcontracts.ShareCompanyContactsRequest{
		SubjectType: "user", SubjectId: crmcontracts.Id(e.Rep2), Access: "read", ExpiresAt: &expires,
	})
	if err != nil || result.Shared != 1 || result.Skipped != 0 {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM record_grant
		WHERE record_id = $1 AND subject_id = $2 AND access = 'read' AND expires_at = $3 AND version = 2`, contact, e.Rep2, expires); got != 1 {
		t.Fatal("the existing grant did not take the chosen access and expiry")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE action = 'record_share' AND entity_id = $1`, contact); got != 2 {
		t.Fatalf("share and restatement audit rows = %d, want 2", got)
	}
}

func TestSharingCompanyContactsThroughTheHTTPRoute(t *testing.T) {
	e := setupRelationships(t)
	contacts := []string{e.contactID}
	for _, name := range []string{"Account engineer", "Account finance"} {
		contacts = append(contacts, createAndID(t, e.AppEnv, "/v1/contacts", AnyMap{"source": "manual", "full_name": name}))
	}
	for _, contact := range contacts {
		if status := e.Call(t, "POST", "/v1/relationships", AnyMap{
			"kind": "employment", "contact_id": contact, "company_id": e.companyID,
			"is_current_primary": true, "source": "manual",
		}, nil, nil); status != http.StatusCreated {
			t.Fatalf("create employment: %d", status)
		}
	}
	var result crmcontracts.ShareCompanyContactsResult
	if status := e.Call(t, "POST", "/v1/companies/00000000-0000-0000-0000-000000000000/share-contacts", AnyMap{
		"subject_type": "user", "subject_id": meUserID(t, e), "access": "write",
	}, nil, nil); status != http.StatusNotFound {
		t.Fatalf("share on a missing company: status %d, want 404", status)
	}
	if status := e.Call(t, "POST", "/v1/companies/"+e.companyID+"/share-contacts", AnyMap{
		"subject_type": "user", "subject_id": meUserID(t, e), "access": "write",
	}, nil, &result); status != http.StatusOK || result.Shared != 3 || result.Skipped != 0 {
		t.Fatalf("share company contacts: status %d, result %+v", status, result)
	}
}

func TestSharingCompanyContactsThroughTheOwnersAgent(t *testing.T) {
	e := Setup(t)
	company := companyIDOf(e.SeedCompany(t, "Owner's account", &e.Rep1))
	contact := e.SeedContact(t, "Owner's employee", &e.Rep1)
	shareCompanyEmployee(t, e, company, contact, false)
	e.MakeCapturePrivate(t, "contact", contact, e.Rep1)
	agent := e.AgentOnPassport(t, e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		RoleKeys: []string{"rep"}, RowScope: principal.RowScopeOwn,
		Objects: map[string]principal.ObjectGrant{
			"contact": {Read: true, Update: true}, "company": {Read: true}, "relationship": {Read: true},
		},
	}, ids.From[ids.PassportKind](e.AgentPassport))
	result, err := compose.ShareCompanyContacts(agent, e.DB(), identity.NewServiceFor(e.DB()), company, crmcontracts.ShareCompanyContactsRequest{
		SubjectType: "team", SubjectId: crmcontracts.Id(e.Team1), Access: "write",
	})
	if err != nil || result.Shared != 1 || result.Skipped != 0 {
		t.Fatalf("agent share: result %+v, err %v", result, err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log
		WHERE action = 'record_share' AND actor_type = 'agent' AND on_behalf_of = $1 AND entity_id = $2`, e.Rep1, contact); got != 1 {
		t.Fatal("the share was not attributed to the agent and its owner")
	}
}
