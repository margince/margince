// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

import (
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAPromiseFiledFromACallLeadsWithTheCall(t *testing.T) {
	number := []crmcontracts.ContactPhone{{Phone: "+493012345678", IsPrimary: true}}
	call := crmcontracts.ActivityReplyChannelCall
	email := crmcontracts.ActivityReplyChannelEmail
	archived := at(1)
	cases := []struct {
		name     string
		channel  *crmcontracts.ActivityReplyChannel
		phones   *[]crmcontracts.ContactPhone
		archived *time.Time
		due      *time.Time
		wantCall bool
	}{
		{"an open promise from a call, number on file", &call, &number, nil, nil, true},
		{"a late promise from a call, number on file", &call, &number, nil, new(now.Add(-48 * time.Hour)), true},
		{"a promise from a call, no number", &call, nil, nil, nil, false},
		{"a promise from a call, blank number", &call, &[]crmcontracts.ContactPhone{{Phone: ""}}, nil, nil, false},
		{"a promise from a call, archived contact", &call, &number, &archived, nil, false},
		{"a promise from an email", &email, &number, nil, nil, false},
		{"a promise typed by hand", nil, &number, nil, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			task := crmcontracts.Activity{
				Id: openapi_types.UUID(ids.NewV7()), Kind: crmcontracts.ActivityKindTask,
				OccurredAt: at(3), Subject: new("Call Dana back"), DueAt: tc.due, ReplyChannel: tc.channel,
			}
			page := &crmcontracts.Contact360{
				Contact: crmcontracts.Contact{Phones: tc.phones, ArchivedAt: tc.archived},
				NextSteps: &struct {
					Data []crmcontracts.Activity `json:"data"`
					Page crmcontracts.PageInfo   `json:"page"`
				}{Data: []crmcontracts.Activity{task}},
			}

			got := momentCopyFor("en").deriveMoment(readerCtx(), now, page)

			lead := got.RecommendedAction
			if !tc.wantCall {
				if lead.Kind != crmcontracts.ContactMomentActionKindDraftReply || got.SecondaryActions != nil {
					t.Fatalf("lead = %q with second %v, want the plain write", lead.Kind, got.SecondaryActions)
				}
				return
			}
			if lead.Kind != crmcontracts.ContactMomentActionKindCall || lead.State != crmcontracts.ContactMomentActionStateAvailable {
				t.Fatalf("lead = %q (%q), want an available call", lead.Kind, lead.State)
			}
			if lead.Label != "Call them" {
				t.Errorf("label = %q", lead.Label)
			}
			if got.SecondaryActions == nil || len(*got.SecondaryActions) != 1 ||
				(*got.SecondaryActions)[0].Kind != crmcontracts.ContactMomentActionKindDraftReply {
				t.Fatalf("second = %v, want writing kept reachable", got.SecondaryActions)
			}
		})
	}
}

func TestTheCallLabelIsSpokenInEveryLanguage(t *testing.T) {
	for language, want := range map[string]string{"en": "Call them", "de": "Anrufen", "vi": "Gọi cho họ"} {
		if got := momentCopyFor(language).Say(momentWords.Call); got != want {
			t.Errorf("%s = %q, want %q", language, got, want)
		}
	}
}
