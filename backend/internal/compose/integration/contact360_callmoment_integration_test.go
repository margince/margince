// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// A task filed from a logged call leads the contact's Overview with the call,
// and the same task filed from a note does not. The source is the writer's
// SourceActivityID, so the test runs the page's own reply_channel read.
func TestAPromiseFiledFromACallLeadsTheMomentWithTheCall(t *testing.T) {
	t.Parallel()
	e := Setup(t)
	cases := []struct {
		name, sourceKind string
		phone            bool
		wantKind         crmcontracts.ContactMomentActionKind
	}{
		{"a call, number on file", "call", true, crmcontracts.ContactMomentActionKindCall},
		{"a call, no number", "call", false, crmcontracts.ContactMomentActionKindDraftReply},
		{"a note, number on file", "note", true, crmcontracts.ContactMomentActionKindDraftReply},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := contacts.CreateContactInput{FullName: "Dana " + tc.name, Source: provenance.RecordSourceManual}
			if tc.phone {
				input.Phones = []contacts.ContactPhoneInput{{Phone: "+49301234567", PhoneType: "mobile", IsPrimary: true}}
			}
			contact, err := e.Contacts.CreateContact(e.Admin(), input)
			if err != nil {
				t.Fatalf("seeding the contact: %v", err)
			}
			link := []activities.ActivityLinkInput{{EntityType: objContact, EntityID: ids.UUID(contact.Id)}}
			source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
				Kind: tc.sourceKind, Subject: new("Weekly sync"), Source: provenance.RecordSourceManual, Links: link,
			})
			if err != nil {
				t.Fatalf("logging the %s: %v", tc.sourceKind, err)
			}
			taskType := crmcontracts.TaskTypeReachOut
			if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
				Kind: "task", Subject: new("Call Dana back on Monday"), TaskType: &taskType,
				SourceActivityID: new(ids.UUID(source.Id)), Source: provenance.RecordSourceManual, Links: link,
			}); err != nil {
				t.Fatalf("filing the task: %v", err)
			}

			page, err := taskOpenService(e, source.OccurredAt.AddDate(0, 0, 1)).Assemble(e.Admin(), ids.From[ids.ContactKind](ids.UUID(contact.Id)))
			if err != nil {
				t.Fatalf("assembling the page: %v", err)
			}
			if page.Moment == nil || page.Moment.Rule != crmcontracts.ContactMomentRuleOpenPromise {
				t.Fatalf("moment = %+v, want the open promise", page.Moment)
			}
			if got := page.Moment.RecommendedAction.Kind; got != tc.wantKind {
				t.Errorf("lead action = %q, want %q", got, tc.wantKind)
			}
		})
	}
}
