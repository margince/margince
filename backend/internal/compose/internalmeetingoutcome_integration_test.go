// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestInternalMeetingOutcomeEligibilityAgreesOnListsAndDetails(t *testing.T) {
	e := integration.Setup(t)
	trustOwnDomain(t, e)
	anchor, err := e.Contacts.SaveCompany(e.Admin(), contacts.SaveCompanyInput{DisplayName: "Our company"})
	if err != nil {
		t.Fatal(err)
	}
	ownDomain := addressedContact(t, e, "Internal colleague", "colleague@"+colleagueTestDomain)
	memberEmail := e.WsScalar(t, `SELECT email FROM app_user WHERE id=$1`, e.Rep1)
	memberContact := addressedContact(t, e, "Booking colleague", memberEmail)
	employee := addressedContact(t, e, "Own company employee", "employee@another.test")
	_, err = e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{Kind: "employment", ContactID: ptrContact(employee), CompanyID: &anchor.CompanyID, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	customer := addressedContact(t, e, "Buyer", "buyer@customer.test")
	now := clocktest.Now(t)
	meetings := map[ids.UUID]bool{}
	for _, tc := range []struct {
		name     string
		contacts []ids.UUID
		expected bool
	}{
		{"own domain", []ids.UUID{ownDomain}, false},
		{"member contact", []ids.UUID{memberContact}, false},
		{"own employment", []ids.UUID{employee}, false},
		{"mixed", []ids.UUID{ownDomain, customer}, true},
	} {
		in := activities.LogActivityInput{Kind: "meeting", Subject: &tc.name, OccurredAt: new(now.Add(-time.Hour)), Source: "manual"}
		for _, contact := range tc.contacts {
			in.Links = append(in.Links, activities.ActivityLinkInput{EntityType: "contact", EntityID: contact})
		}
		row, _, err := e.Activities.LogActivity(e.Admin(), in)
		if err != nil {
			t.Fatal(err)
		}
		meetings[ids.UUID(row.Id)] = tc.expected
	}
	lane := attentionMeetingsAwaitingOutcome{store: e.Activities.WithClock(func() time.Time { return now })}
	rows, err := lane.Since(e.Admin(), attention.MeetingWindow{From: now.Add(-24 * time.Hour), Until: now}, 20, attention.TasksVisible, ids.Nil)
	if err != nil {
		t.Fatal(err)
	}
	shown := map[ids.UUID]bool{}
	for _, row := range rows {
		shown[row.ID] = true
	}
	for id, want := range meetings {
		row, err := e.Activities.WithClock(func() time.Time { return now }).GetActivity(e.Admin(), ids.From[ids.ActivityKind](id), storekit.LiveOnly)
		if err != nil {
			t.Fatal(err)
		}
		if row.AwaitingOutcome == nil || *row.AwaitingOutcome != want || shown[id] != want {
			t.Fatalf("meeting %s: detail=%v listed=%t want=%t", id, row.AwaitingOutcome, shown[id], want)
		}
	}
}
