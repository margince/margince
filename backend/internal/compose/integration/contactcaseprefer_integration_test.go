// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration_test

// A case merge keeps every value of every member once, and the reviewer's
// preference picks the primary address.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/introductions"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// caseMergeStore is the contacts store with the carriers a case merge runs.
func caseMergeStore(t *testing.T, e *integration.Env) *contacts.Store {
	t.Helper()
	consentStore := consent.NewStore(e.DB())
	return contacts.NewStore(e.DB()).
		WithStopCarrier(consentStore).
		WithSatelliteCarriers(consentStore, introductions.NewStore(e.DB(), func() time.Time { return clocktest.Now(t) })).
		WithGrantCarrier(identity.NewServiceFor(e.DB())).
		WithParticipantRosterReader(activities.EnsureParticipantRosterVisibleTx).
		WithSettings(configtest.StoredOver(t, e.Pool, contacts.ConfigSettings()))
}

func TestACaseMergeKeepsASharedNumberOnceAndThePreferredAddress(t *testing.T) {
	e := integration.Setup(t)
	store := caseMergeStore(t, e)
	admin := e.Admin()
	const number = "+493055509001"
	owner := ids.From[ids.UserKind](e.Rep1)
	var members []ids.UUID
	for _, email := range []string{"alice@example.com", "alice.w@example.com", "a.wong@example.com"} {
		created, err := store.CreateContact(admin, contacts.CreateContactInput{
			FullName: "Alice Wong", Source: "manual", OwnerID: &owner,
			Emails: []contacts.ContactEmailInput{{Email: email, EmailType: "work", IsPrimary: true}},
			Phones: []contacts.ContactPhoneInput{{Phone: number, PhoneType: "mobile", IsPrimary: true}},
		})
		if err != nil {
			t.Fatalf("creating a duplicate: %v", err)
		}
		members = append(members, ids.UUID(created.Id))
	}
	read, err := store.GetContactCase(admin, members[0])
	if err != nil {
		t.Fatalf("reading the case: %v", err)
	}
	decision := contacts.ContactCaseDecision{Revision: read.Revision, MemberIDs: members, SurvivorID: members[0], OwnerID: e.Rep1,
		Prefer: contacts.MergePreference{contacts.MergeFieldEmail: members[2]}}

	preview, err := store.PreviewContactCaseMerge(admin, members[0], decision)
	if err != nil {
		t.Fatalf("previewing: %v", err)
	}
	out, err := store.MergeContactCase(admin, members[0], decision)
	if err != nil {
		t.Fatalf("merging the case: %v", err)
	}
	for name, survivor := range map[string]*crmcontracts.Contact{"preview": preview.Survivor, "merge": out.Survivor} {
		if survivor == nil || survivor.Phones == nil || survivor.Emails == nil {
			t.Fatalf("the %s returned no survivor numbers and addresses", name)
		}
		if got := liveNumbers(*survivor.Phones); len(got) != 1 || got[0] != number {
			t.Errorf("%s: the survivor's numbers = %v, want %s once", name, got, number)
		}
		if len(*survivor.Emails) != 3 {
			t.Errorf("%s: the survivor keeps %d addresses, want all three", name, len(*survivor.Emails))
		}
		if survivor.PrimaryEmail == nil || string(*survivor.PrimaryEmail) != "a.wong@example.com" {
			t.Errorf("%s: the primary address is not the preferred member's", name)
		}
	}
}

func liveNumbers(phones []crmcontracts.ContactPhone) []string {
	var out []string
	for _, phone := range phones {
		if phone.ArchivedAt == nil {
			out = append(out, phone.Phone)
		}
	}
	return out
}
