// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestMailboxHistoryKeepsAQualifiedOpportunityAsHistoricalRecovery(t *testing.T) {
	e := setupScout(t)
	e.now = baselineAsOf
	company := e.SeedCompany(t, "Maple Studio", nil)
	buyer, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: "Pat Buyer", Emails: []contacts.ContactEmailInput{{Email: "pat@counterparty.example", EmailType: "work", IsPrimary: true}}, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	contactID, companyID := ids.From[ids.ContactKind](ids.UUID(buyer.Id)), ids.From[ids.CompanyKind](company)
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{Kind: "employment", ContactID: &contactID, CompanyID: &companyID, Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	captureMailVia(t, e.Env, e.Rep1, "gmail", threadMail(seatAddress(t, e.Env, e.Rep1), "quiet-opportunity@buyer.test", "Mon, 15 Jun 2026 10:00:00 +0000", ""))
	setBaseline(t, e.Env, e.Rep1, "gmail", "2026-10-01", "Europe/Berlin")
	source := mailSource(t, e.Env, "quiet-opportunity@buyer.test")
	if _, err := e.Activities.SetAudience(e.rep(e.Rep1, e.Team1), ids.From[ids.ActivityKind](source), activities.SetAudienceInput{Audience: "workspace"}); err != nil {
		t.Fatal(err)
	}
	e.signal(t, "new_opportunity", company, source, e.now, ids.Nil, ids.UUID(buyer.Id))
	if got := dispatchFor(t, e.Env, source, e.Rep1); got != CommitmentOutcome(activities.CommitmentHistorical) {
		t.Fatalf("automatic obligation = %s, want historical exclusion", got)
	}
	if pass := e.pass(t); pass.Raised != 0 {
		t.Fatalf("old evidence raised %d current proposals", pass.Raised)
	}
	scanner := assurance.NewScanner(assurance.NewStore(InstallationDB(e.Pool)), AssuranceSubjects, AssuranceCoverage, assurance.DefaultConfig()).WithSignalSubjects(AssuranceSignalSubjects)
	if _, err := scanner.Scan(e.system(), e.now, nil); err != nil {
		t.Fatal(err)
	}
	found := readSignalFindings(e.signalReader(), t, e)
	if len(found) != 1 {
		t.Fatalf("historical recovery findings = %d, want 1", len(found))
	}
	var observed struct {
		Historical    bool      `json:"historical"`
		OccurredAt    time.Time `json:"occurred_at"`
		BuyerVerified bool      `json:"buyer_verified"`
	}
	if err := json.Unmarshal(found[0].Observed, &observed); err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	if !observed.Historical || !observed.BuyerVerified || !observed.OccurredAt.Equal(want) {
		t.Fatalf("recovery = %+v, want historical verified request at %s", observed, want)
	}
}
