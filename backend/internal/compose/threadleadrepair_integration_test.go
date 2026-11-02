// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestLeadSLAWaitsForUnavailableResponseEvidence(t *testing.T) {
	e := integration.Setup(t)
	now := pinClock(t, e)
	e.Contacts.WithSettings(configtest.StoredOver(t, e.Pool, contacts.ConfigSettings(),
		"MARGINCE_CONTACTS_FIRST_RESPONSE_ENABLED=true", "MARGINCE_CONTACTS_FIRST_RESPONSE_TARGET_MINUTES=60"))
	started := now.Add(-2 * time.Hour)
	makeLead := func(email string) ids.LeadID {
		owner := ids.From[ids.UserKind](e.Rep1)
		row, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{Email: &email, FullName: new("Enquirer"), Source: "inbound", OwnerID: &owner})
		if err != nil {
			t.Fatal(err)
		}
		id := ids.From[ids.LeadKind](ids.UUID(row.Id))
		e.WsExec(t, `UPDATE lead SET created_at=$1,routed_at=$1 WHERE id=$2`, started, id)
		if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
			Kind: "email", Direction: new("inbound"), Subject: new("Enquiry"), OccurredAt: &started,
			Links: []activities.ActivityLinkInput{{EntityType: "lead", EntityID: id.UUID}},
		}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	answered, unanswered := makeLead("answered@sla.example"), makeLead("unanswered@sla.example")
	replied := started.Add(50 * time.Minute)
	const key = "sla-answer@reply.example"
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Direction: new("outbound"), Subject: new("Reply"), ThreadKey: key, OccurredAt: &replied,
		Links: []activities.ActivityLinkInput{{EntityType: "lead", EntityID: answered.UUID}},
	}); err != nil {
		t.Fatal(err)
	}
	run := beginUnavailableThread(t, e, key)
	breaches, err := e.Contacts.ScanLeadSLA(e.Admin(), now)
	if err != nil || len(breaches) != 1 || breaches[0].LeadID != unanswered {
		t.Fatalf("breaches during repair=%+v: %v", breaches, err)
	}
	if e.WsCount(t, `SELECT count(*) FROM lead WHERE id=$1 AND sla_breached_at IS NOT NULL`, answered) != 0 {
		t.Fatal("an unavailable on-time response became a persisted SLA breach")
	}
	finishUnavailableThread(t, e, run)
	if _, err := e.Contacts.ScanLeadSLA(e.Admin(), now); err != nil {
		t.Fatal(err)
	}
	if e.WsCount(t, `SELECT count(*) FROM lead WHERE id=$1 AND first_response_at=$2 AND sla_breached_at IS NULL`, answered, replied) != 1 {
		t.Fatal("published on-time response did not settle the SLA without a breach")
	}
}

func TestPendingLatestVendorMailDoesNotHandQuestionToOlderRecipient(t *testing.T) {
	e := integration.Setup(t)
	now := pinClock(t, e)
	older, latest := "vendor-routing-older", "vendor-routing-latest"
	captureStaffMail(t, e, staffMail{sourceID: older, from: hostingStaff, mailbox: e.Rep1, to: []string{seatEmail(t, e.Rep1)}, occurred: now.Add(-2 * time.Hour)})
	runVendorTriage(t, e, hostingDomain)
	captureStaffMail(t, e, staffMail{sourceID: latest, from: hostingStaff, mailbox: e.Rep1, to: []string{seatEmail(t, e.Rep2)}, cc: []string{seatEmail(t, e.Rep1)}, occurred: now.Add(-time.Hour)})
	rep1, rep2 := e.As(e.Rep1, nil, integration.AdminPerms), e.As(e.Rep2, nil, integration.AdminPerms)
	if asksAbout(rep1, t, e, hostingDomain) || !asksAbout(rep2, t, e, hostingDomain) {
		t.Fatal("latest sole recipient did not control the question before repair")
	}
	key := threadKeyOfMessage(t, e, latest)
	if key == threadKeyOfMessage(t, e, older) {
		t.Fatal("routing premise requires independent conversations")
	}
	run := beginUnavailableThread(t, e, key)
	if asksAbout(rep1, t, e, hostingDomain) || asksAbout(rep2, t, e, hostingDomain) {
		t.Fatal("unavailable newest mail handed authority to an older readable conversation")
	}
	finishUnavailableThread(t, e, run)
	if asksAbout(rep1, t, e, hostingDomain) || !asksAbout(rep2, t, e, hostingDomain) {
		t.Fatal("publication did not restore the latest recipient's question")
	}
}
