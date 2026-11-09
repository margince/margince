// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func TestNewCopiedRecipientsAndMeetingGuestsReachTheExistingDealFiler(t *testing.T) {
	for _, medium := range []string{"mail", "meeting"} {
		t.Run(medium, func(t *testing.T) {
			e := integration.Setup(t)
			e.GrantRole(t, e.AdminUser, "admin")
			now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
			company, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: "Buyer account", Source: "manual", Domains: []contacts.CompanyDomainInput{{Domain: "buyer.test", IsPrimary: true}}})
			if err != nil {
				t.Fatal(err)
			}
			pipeline, stage, _ := integration.DealFixture(t, e)
			companyID := ids.From[ids.CompanyKind](ids.UUID(company.Id))
			deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{Name: "Buyer rollout", PipelineID: pipeline, StageID: stage, CompanyID: &companyID, Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			sink := newCaptureSink(e.Pool, CaptureConfig{Now: func() time.Time { return now }})
			activity := captureDealParty(t, e, sink, medium)
			var bob ids.UUID
			if err := e.Pool.QueryRow(t.Context(), `SELECT contact_id FROM contact_email WHERE lower(email)='bob@buyer.test' AND archived_at IS NULL`).Scan(&bob); err != nil {
				t.Fatal(err)
			}
			integration.ApplyRiverSchema(t)
			fixture := dealRefileFixture{o: &owedEnv{e: e, owner: e.AdminUser, now: now}}
			args := fixture.enqueue(t, "contact.created", bob)
			fixture.run(t, args)
			if count := e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id=$1 AND deal_id=$2`, activity.ID, deal.Id); count != 1 {
				t.Fatalf("%s has %d links to its admitted guest's deal", medium, count)
			}
			before := e.WsCount(t, `SELECT count(*) FROM audit_log`)
			fixture.run(t, args)
			if e.WsCount(t, `SELECT count(*) FROM audit_log`) != before {
				t.Fatal("replay repeated the deal filing")
			}
		})
	}
}

func captureDealParty(t *testing.T, e *integration.Env, sink *capture.Sink, medium string) datasource.EntityRef {
	t.Helper()
	if medium == "mail" {
		return captureDealMail(t, e, sink)
	}
	return captureDealMeeting(t, e, sink)
}

func captureDealMail(t *testing.T, e *integration.Env, sink *capture.Sink) datasource.EntityRef {
	t.Helper()
	var activity datasource.EntityRef
	original := partiesMailRecord(t, e, partyMailProof{outbound: true, sentByOwner: true}, "Bob Buyer <bob@buyer.test>", "", "Could we discuss the product requirements?")
	raw := []byte(strings.ReplaceAll(string(original.Raw), "Alice Buyer <alice@buyer.test>", "Alice Guide <alice@elsewhere.test>"))
	parsed, err := mailmap.Parse(raw, seatAddress(t, e, e.AdminUser))
	if err != nil {
		t.Fatal(err)
	}
	activity, err = sink.Upsert(threadConnectorCtx(e, e.AdminUser), parsed.AttestSentByOwner(true).ToRecord("gmail", raw))
	if err != nil {
		t.Fatal(err)
	}
	return activity
}

func captureDealMeeting(t *testing.T, e *integration.Env, sink *capture.Sink) datasource.EntityRef {
	t.Helper()
	var activity datasource.EntityRef
	seedCalendarConnection(t, e)
	event := map[string]any{"id": ids.NewV7().String(), "status": "confirmed", "summary": "Product requirements", "start": map[string]string{"dateTime": "2026-10-05T10:00:00Z"}, "end": map[string]string{"dateTime": "2026-10-05T11:00:00Z"}, "organizer": map[string]any{"email": backfillOwner, "self": true}, "attendees": []map[string]any{{"email": backfillOwner, "self": true, "organizer": true, "responseStatus": "accepted"}, {"email": "alice@elsewhere.test", "responseStatus": "accepted"}, {"email": "bob@buyer.test", "responseStatus": "accepted"}}}
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	record, err := gcal.RecordOf(raw, backfillOwner)
	if err != nil {
		t.Fatal(err)
	}
	activity, err = sink.Upsert(calendarOwnerCtx(e, e.AdminUser), record)
	if err != nil {
		t.Fatal(err)
	}
	return activity
}
