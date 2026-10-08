// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// An account's last contact counts every exchange that is contact, whatever
// its direction: a meeting and a call carry none and are the likeliest last
// contact on an account nobody mails. A note is not contact, and neither is a
// meeting called off.

import (
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// employedAccount is a company with one employed contact and no activity yet.
func employedAccount(t *testing.T, e *integration.Env, name string) quietAccount {
	t.Helper()
	acct := quietAccount{
		company: ids.From[ids.CompanyKind](e.SeedCompany(t, name, nil)),
		contact: e.SeedContact(t, "Casey Contact", nil),
		now:     time.Now().UTC(),
	}
	contactID, primary := ids.From[ids.ContactKind](acct.contact), true
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &contactID, CompanyID: &acct.company, IsCurrentPrimary: &primary,
	}); err != nil {
		t.Fatalf("employing the contact: %v", err)
	}
	return acct
}

// logWith files one activity against the account's contact, which is how a
// meeting reaches its company.
func logWith(t *testing.T, e *integration.Env, acct quietAccount, kind, status string, at time.Time) openapi_types.UUID {
	t.Helper()
	subject := kind + " with the account"
	in := activities.LogActivityInput{
		Kind: kind, Subject: &subject, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: acct.contact}},
	}
	if status != "" {
		in.MeetingStatus = &status
	}
	created, _, err := e.Activities.LogActivity(e.Admin(), in)
	if err != nil {
		t.Fatalf("logging the %s: %v", kind, err)
	}
	return created.Id
}

func TestLastContactCountsMeetingsAndCallsButNotNotes(t *testing.T) {
	e := integration.Setup(t)
	acct := employedAccount(t, e, "Met Only GmbH")
	logWith(t, e, acct, "meeting", "held", acct.now.AddDate(0, 0, -10))
	call := logWith(t, e, acct, "call", "", acct.now.AddDate(0, 0, -5))
	logWith(t, e, acct, "meeting", "canceled", acct.now.AddDate(0, 0, -3))
	logWith(t, e, acct, "note", "", acct.now.AddDate(0, 0, -1))

	page := pageOf(t, e, acct)
	got := page.LastContact
	if got == nil {
		t.Fatalf("an account with a held meeting and a call has no last contact")
	}
	if got.ActivityId != call || got.Kind != crmcontracts.Company360LastContactKindCall {
		t.Fatalf("last contact = %s %v, want the call %v: a canceled meeting and a note are not contact",
			got.Kind, got.ActivityId, call)
	}
	if page.LastInboundAt != nil || page.LastOutboundAt != nil {
		t.Fatalf("no mail was logged, yet a direction is set: in=%v out=%v", page.LastInboundAt, page.LastOutboundAt)
	}
}

func TestANoteAloneIsNoLastContact(t *testing.T) {
	e := integration.Setup(t)
	acct := employedAccount(t, e, "Noted GmbH")
	logWith(t, e, acct, "note", "", acct.now.AddDate(0, 0, -1))

	if got := pageOf(t, e, acct).LastContact; got != nil {
		t.Fatalf("last contact = %+v for an account that only has a note", got)
	}
}
