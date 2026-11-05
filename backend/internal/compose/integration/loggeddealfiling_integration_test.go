// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

type loggedDealWorld struct {
	*Env
	company ids.CompanyID
	contact ids.ContactID
	store   *activities.Store
	deals   []ids.UUID
}

func newLoggedDealWorld(t *testing.T, count int) *loggedDealWorld {
	t.Helper()
	e := Setup(t)
	w := &loggedDealWorld{
		Env: e, company: companyIDOf(e.SeedCompany(t, "Call account", &e.AdminUser)),
		contact: ContactIDOf(e.SeedContact(t, "Call contact", &e.AdminUser)),
		store:   activities.NewStore(e.DB()).WithDealFiling(compose.NewActivityDealFiling(e.DB())),
	}
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", CompanyID: &w.company, ContactID: &w.contact, IsCurrentPrimary: new(true), Source: "manual",
	}); err != nil {
		t.Fatal(err)
	}
	pipeline, open, _ := DealFixture(t, e)
	for i := range count {
		deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
			Name: fmt.Sprintf("Open deal %d", i+1), PipelineID: pipeline, StageID: open,
			CompanyID: &w.company, Source: "manual",
		})
		if err != nil {
			t.Fatal(err)
		}
		w.deals = append(w.deals, ids.UUID(deal.Id))
	}
	return w
}

func (w *loggedDealWorld) call(at time.Time) activities.LogActivityInput {
	return activities.LogActivityInput{
		Kind: "call", Body: new("Discussed the renewal"), CallOutcome: new("reached"), OccurredAt: &at,
		Source: "manual", Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: w.contact.UUID}},
	}
}

func TestAReachedCallFilesOnTheOnlyOpenDealAndMovesItsClock(t *testing.T) {
	w := newLoggedDealWorld(t, 1)
	at := time.Date(2026, 1, 10, 9, 0, 0, 0, time.UTC)
	logged, _, err := w.store.LogActivity(w.Admin(), w.call(at))
	if err != nil {
		t.Fatal(err)
	}
	if got := w.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND deal_id = $2`, logged.Id, w.deals[0]); got != 1 {
		t.Fatal("the contact call was not filed on the company's only open deal")
	}
	if got := w.WsCount(t, `SELECT count(*) FROM deal WHERE id = $1 AND last_activity_at = $2`, w.deals[0], at); got != 1 {
		t.Fatal("the reached call did not move the deal's clock")
	}
	unreached := w.call(at.Add(time.Hour))
	unreached.CallOutcome = new("not_reached")
	if _, _, err := w.store.LogActivity(w.Admin(), unreached); err != nil {
		t.Fatal(err)
	}
	if got := w.WsCount(t, `SELECT count(*) FROM deal WHERE id = $1 AND last_activity_at = $2`, w.deals[0], at); got != 1 {
		t.Fatal("an unreached call moved the deal's clock")
	}
}

func TestLoggedDealFilingDistinguishesNoDealARequiredChoiceAndExplicitNone(t *testing.T) {
	for _, count := range []int{0, 1, 2} {
		t.Run(fmt.Sprintf("open deals %d", count), func(t *testing.T) {
			w := newLoggedDealWorld(t, count)
			if count > 1 {
				selected := ids.From[ids.DealKind](w.deals[0])
				if _, err := w.Contacts.CreateRelationship(w.Admin(), contacts.CreateRelationshipInput{
					Kind: "deal_stakeholder", ContactID: &w.contact, DealID: &selected, Source: "manual",
				}); err != nil {
					t.Fatal(err)
				}
			}
			ref := crmcontracts.ActivityDealFiling{RecordType: "contact", RecordId: crmcontracts.Id(w.contact.UUID)}
			options, err := w.store.DealFilingOptions(w.Admin(), ref)
			if err != nil {
				t.Fatal(err)
			}
			if options.ChoiceRequired != (count > 1) || (options.SuggestedDealId != nil) != (count == 1) {
				t.Fatalf("filing options = %+v for %d open deals", options, count)
			}
			in := w.call(time.Date(2026, 1, 10, 9, 0, 0, 0, time.UTC))
			in.DealFiling = &ref
			if count > 1 {
				if _, _, err := w.store.LogActivity(w.Admin(), in); err == nil {
					t.Fatal("an ambiguous filing did not ask for a deal or none")
				} else {
					fault, ok := errors.AsType[*httperr.DetailedError](err)
					if !ok {
						t.Fatalf("wanted a filing validation error: %v", err)
					}
					if len(fault.Fields) != 1 {
						t.Fatalf("field faults = %+v", fault.Fields)
					}
					field, code := fault.Fields[0].Field, fault.Fields[0].Code
					if field != "deal_filing" || code != "choose_deal" {
						t.Fatalf("fault = %s/%s", field, code)
					}
				}
				ref.Skip = new(true)
			}
			logged, _, err := w.store.LogActivity(w.Admin(), in)
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if count == 1 {
				want = 1
			}
			if got := w.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND entity_type = 'deal'`, logged.Id); got != want {
				t.Fatalf("deal links = %d, want %d", got, want)
			}
		})
	}
}

func TestLoggedDealFilingPreservesTheChosenDealAmongSeveral(t *testing.T) {
	w := newLoggedDealWorld(t, 2)
	in := w.call(time.Date(2026, 1, 10, 9, 0, 0, 0, time.UTC))
	in.DealFiling = &crmcontracts.ActivityDealFiling{RecordType: "company", RecordId: crmcontracts.Id(w.company.UUID)}
	in.Links = append(in.Links, activities.ActivityLinkInput{EntityType: "deal", EntityID: w.deals[1]})
	logged, _, err := w.store.LogActivity(w.Admin(), in)
	if err != nil {
		t.Fatal(err)
	}
	if got := w.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND entity_type = 'deal'`, logged.Id); got != 1 {
		t.Fatalf("deal links = %d, want the one chosen deal", got)
	}
	if got := w.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND deal_id = $2`, logged.Id, w.deals[1]); got != 1 {
		t.Fatal("the selected deal was replaced")
	}
}

func TestTheActivityDatasourceUsesTheSameSingleDealRule(t *testing.T) {
	w := newLoggedDealWorld(t, 1)
	at := time.Date(2026, 1, 10, 9, 0, 0, 0, time.UTC)
	logged, err := compose.NewProviderFor(w.DB()).Create(w.Admin(), datasource.CreateInput{
		EntityType: datasource.EntityActivity, Source: "manual",
		Fields: AnyMap{
			"kind": "call", "body": "Discussed the renewal", "call_outcome": "reached", "occurred_at": at,
			"links": []AnyMap{{"entity_type": "contact", "entity_id": w.contact.String()}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := w.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND deal_id = $2`, logged.ID, w.deals[0]); got != 1 {
		t.Fatal("the activity datasource did not use the shared single-deal rule")
	}
}

func TestLoggedDealFilingThroughTheHTTPFormAndWriteRoutes(t *testing.T) {
	e := setupRelationships(t)
	if status := e.Call(t, "POST", "/v1/relationships", AnyMap{
		"kind": "employment", "contact_id": e.contactID, "company_id": e.companyID,
		"is_current_primary": true, "source": "manual",
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("create employment: %d", status)
	}
	stages := apptest.DiscoverSeededPipeline(t, e.AppEnv)
	deal := createAndID(t, e.AppEnv, "/v1/deals", AnyMap{
		"name": "Only open account deal", "pipeline_id": stages.PipelineID,
		"stage_id": stages.Open, "company_id": e.companyID, "source": "manual",
	})
	for _, source := range []struct{ kind, id string }{{"contact", e.contactID}, {"company", e.companyID}} {
		var options crmcontracts.ActivityDealFilingOptions
		if status := e.Call(t, "GET", "/v1/activities/deal-filing?record_type="+source.kind+"&record_id="+source.id, nil, nil, &options); status != http.StatusOK {
			t.Fatalf("read %s filing options: %d", source.kind, status)
		}
		if options.ChoiceRequired || options.SuggestedDealId == nil || options.SuggestedDealId.String() != deal {
			t.Fatalf("HTTP filing options = %+v, want the one open deal", options)
		}
	}
	at := time.Date(2026, 1, 10, 9, 0, 0, 0, time.UTC)
	var logged crmcontracts.Activity
	if status := e.Call(t, "POST", "/v1/activities", AnyMap{
		"kind": "call", "body": "Discussed the renewal", "call_outcome": "reached", "occurred_at": at,
		"source": "manual", "links": []AnyMap{{"entity_type": "contact", "entity_id": e.contactID}},
	}, nil, &logged); status != http.StatusCreated {
		t.Fatalf("log contact call: %d", status)
	}
	var last *time.Time
	if err := e.Owner.QueryRow(t.Context(), `SELECT last_activity_at FROM deal WHERE id = $1`, deal).Scan(&last); err != nil {
		t.Fatal(err)
	}
	if last == nil || !last.Equal(at) {
		t.Fatalf("HTTP call left deal clock at %v, want %v", last, at)
	}
	heldAt := at.Add(time.Hour)
	if status := e.Call(t, "POST", "/v1/activities", AnyMap{
		"kind": "meeting", "body": "Reviewed the renewal", "meeting_status": "held", "occurred_at": heldAt,
		"source": "manual", "links": []AnyMap{{"entity_type": "contact", "entity_id": e.contactID}},
		"deal_filing": AnyMap{"record_type": "company", "record_id": e.companyID},
	}, nil, &logged); status != http.StatusCreated {
		t.Fatalf("log company meeting: %d", status)
	}
	if err := e.Owner.QueryRow(t.Context(), `SELECT last_activity_at FROM deal WHERE id = $1`, deal).Scan(&last); err != nil {
		t.Fatal(err)
	}
	if last == nil || !last.Equal(heldAt) {
		t.Fatalf("held meeting left deal clock at %v, want %v", last, heldAt)
	}
}

func TestDealFilingHonorsDealReadPermission(t *testing.T) {
	for _, count := range []int{0, 1, 2} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			w := newLoggedDealWorld(t, count)
			perms := principal.Permissions{RowScope: principal.RowScopeTeam, RoleKeys: []string{"rep"}, Objects: map[string]principal.ObjectGrant{
				"activity": {Create: true, Read: true}, "contact": {Read: true}, "company": {Read: true}, "deal": {Read: true}, "relationship": {Read: true},
			}}
			ref := crmcontracts.ActivityDealFiling{RecordType: "contact", RecordId: crmcontracts.Id(w.contact.UUID)}
			ctx := w.As(w.Rep1, []ids.UUID{w.Team1}, perms)
			out, err := w.store.DealFilingOptions(ctx, ref)
			if err != nil {
				t.Fatal(err)
			}
			if out.ChoiceRequired != (count > 1) || (out.SuggestedDealId != nil) != (count == 1) {
				t.Fatalf("a team-scoped rep can read all deals: %+v", out)
			}
			delete(perms.Objects, "deal")
			ctx = w.As(w.Rep1, []ids.UUID{w.Team1}, perms)
			out, err = w.store.DealFilingOptions(ctx, ref)
			if err != nil {
				t.Fatal(err)
			}
			if out.ChoiceRequired || out.SuggestedDealId != nil {
				t.Fatalf("deal details without read permission: %+v", out)
			}
		})
	}
}

func TestDealFilingRefusesAnUnrelatedCompanyContext(t *testing.T) {
	w := newLoggedDealWorld(t, 1)
	other := w.SeedCompany(t, "Unrelated account", &w.AdminUser)
	in := w.call(time.Date(2026, 1, 10, 9, 0, 0, 0, time.UTC))
	in.DealFiling = &crmcontracts.ActivityDealFiling{RecordType: "company", RecordId: crmcontracts.Id(other)}
	_, _, err := w.store.LogActivity(w.Admin(), in)
	fault, ok := errors.AsType[*httperr.DetailedError](err)
	if !ok {
		t.Fatalf("wanted an unrelated context fault: %v", err)
	}
	if len(fault.Fields) != 1 {
		t.Fatalf("field faults = %+v", fault.Fields)
	}
	field, code := fault.Fields[0].Field, fault.Fields[0].Code
	if field != "deal_filing" || code != "unrelated_record" {
		t.Fatalf("fault = %s/%s", field, code)
	}
}

func TestRecordingAnExistingMeetingFilesTheSameActivityOnItsDeal(t *testing.T) {
	for _, count := range []int{1, 2} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			w := newLoggedDealWorld(t, count)
			at := time.Date(2026, 1, 10, 9, 0, 0, 0, time.UTC)
			in := w.call(at)
			in.Kind, in.CallOutcome, in.MeetingStatus = "meeting", nil, new("booked")
			booked, _, err := w.store.LogActivity(w.Admin(), in)
			if err != nil {
				t.Fatal(err)
			}
			if got := w.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND entity_type = 'deal'`, booked.Id); got != 0 {
				t.Fatal("a booking was filed before its outcome")
			}
			ref := &crmcontracts.ActivityDealFiling{RecordType: "contact", RecordId: crmcontracts.Id(w.contact.UUID)}
			if count == 2 {
				ref.DealId = new(crmcontracts.Id(w.deals[1]))
			}
			out, err := w.store.UpdateActivity(w.Admin(), ids.From[ids.ActivityKind](ids.UUID(booked.Id)), activities.UpdateActivityInput{
				MeetingStatus: new("held"), IfVersion: (*int64)(booked.Version), DealFiling: ref,
			})
			if err != nil {
				t.Fatal(err)
			}
			if out.Id != booked.Id {
				t.Fatal("the meeting identity changed")
			}
			if got := w.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND deal_id = $2`, out.Id, w.deals[count-1]); got != 1 {
				t.Fatal("the recorded meeting did not gain its deal link")
			}
			if got := w.WsCount(t, `SELECT count(*) FROM deal WHERE id = $1 AND last_activity_at = $2`, w.deals[count-1], at); got != 1 {
				t.Fatal("the held meeting did not move its deal clock")
			}
		})
	}
}

func TestConcurrentCallsCanFileOnTheSameDealWithoutALockUpgrade(t *testing.T) {
	w := newLoggedDealWorld(t, 1)
	start, results := make(chan struct{}), make(chan error, 2)
	for i := range 2 {
		go func() {
			<-start
			in := w.call(time.Date(2026, 1, 10, 9+i, 0, 0, 0, time.UTC))
			results <- w.DB().Tx(w.Admin(), func(tx pgx.Tx) error {
				_, _, err := w.store.LogActivityTx(w.Admin(), tx, in)
				return err
			})
		}()
	}
	close(start)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
}

func TestAnOlderClientLeavesAnAmbiguousCallUnfiled(t *testing.T) {
	w := newLoggedDealWorld(t, 2)
	logged, _, err := w.store.LogActivity(w.Admin(), w.call(time.Date(2026, 1, 10, 9, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatal(err)
	}
	if got := w.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND entity_type = 'deal'`, logged.Id); got != 0 {
		t.Fatal("an older client's call was guessed onto a deal")
	}
}

func TestACompanyCallMayIncludeAContactFromAnotherCompany(t *testing.T) {
	w := newLoggedDealWorld(t, 1)
	partner := companyIDOf(w.SeedCompany(t, "Partner account", &w.AdminUser))
	guest := ContactIDOf(w.SeedContact(t, "Partner attendee", &w.AdminUser))
	if _, err := w.Contacts.CreateRelationship(w.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", CompanyID: &partner, ContactID: &guest, IsCurrentPrimary: new(true), Source: "manual",
	}); err != nil {
		t.Fatal(err)
	}
	in := w.call(time.Date(2026, 1, 10, 9, 0, 0, 0, time.UTC))
	in.Links = append(in.Links, activities.ActivityLinkInput{EntityType: "contact", EntityID: guest.UUID})
	in.DealFiling = &crmcontracts.ActivityDealFiling{RecordType: "company", RecordId: crmcontracts.Id(w.company.UUID)}
	logged, _, err := w.store.LogActivity(w.Admin(), in)
	if err != nil {
		t.Fatal(err)
	}
	if got := w.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND deal_id = $2`, logged.Id, w.deals[0]); got != 1 {
		t.Fatal("the company's deal was lost because another attendee worked elsewhere")
	}
	in.DealFiling = nil
	unfiled, _, err := w.store.LogActivity(w.Admin(), in)
	if err != nil {
		t.Fatal(err)
	}
	if got := w.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND entity_type = 'deal'`, unfiled.Id); got != 0 {
		t.Fatal("an ambiguous company context was guessed")
	}
}

func TestConcurrentMeetingOutcomesFileBothMeetingsOnTheSameDeal(t *testing.T) {
	w := newLoggedDealWorld(t, 1)
	colleague := ContactIDOf(w.SeedContact(t, "Second attendee", &w.AdminUser))
	if _, err := w.Contacts.CreateRelationship(w.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &colleague, CompanyID: &w.company, IsCurrentPrimary: new(true), Source: "manual",
	}); err != nil {
		t.Fatal(err)
	}
	var meetings []crmcontracts.Activity
	for i, contact := range []ids.ContactID{w.contact, colleague} {
		in := w.call(time.Date(2026, 1, 10, 9+i, 0, 0, 0, time.UTC))
		in.Kind, in.CallOutcome, in.MeetingStatus = "meeting", nil, new("booked")
		in.Links = []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact.UUID}}
		booked, _, err := w.store.LogActivity(w.Admin(), in)
		if err != nil {
			t.Fatal(err)
		}
		meetings = append(meetings, booked)
	}
	start, results := make(chan struct{}), make(chan error, 2)
	for _, meeting := range meetings {
		go func() {
			<-start
			_, err := w.store.UpdateActivity(w.Admin(), ids.From[ids.ActivityKind](ids.UUID(meeting.Id)), activities.UpdateActivityInput{
				MeetingStatus: new("held"), IfVersion: (*int64)(meeting.Version),
			})
			results <- err
		}()
	}
	close(start)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	for _, meeting := range meetings {
		if got := w.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND deal_id = $2`, meeting.Id, w.deals[0]); got != 1 {
			t.Fatal("a concurrent meeting outcome lost its deal filing")
		}
	}
}

func TestCancelingABookedMeetingDoesNotFileItAsARecordedMeeting(t *testing.T) {
	w := newLoggedDealWorld(t, 1)
	in := w.call(time.Date(2026, 1, 10, 9, 0, 0, 0, time.UTC))
	in.Kind, in.CallOutcome, in.MeetingStatus = "meeting", nil, new("booked")
	booked, _, err := w.store.LogActivity(w.Admin(), in)
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.store.UpdateActivity(w.Admin(), ids.From[ids.ActivityKind](ids.UUID(booked.Id)), activities.UpdateActivityInput{
		MeetingStatus: new("canceled"), IfVersion: (*int64)(booked.Version),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := w.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND entity_type = 'deal'`, booked.Id); got != 0 {
		t.Fatal("canceling a booking filed it as a recorded meeting")
	}
}
