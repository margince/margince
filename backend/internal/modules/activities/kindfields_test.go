// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"errors"
	"fmt"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// An import and the create wire refuse the same fields of the same kind with
// the same error, because both ask CheckKindFields.
func TestTheImportAndTheWireRefuseTheSameKindFields(t *testing.T) {
	seconds := func(n int) *int { return &n }
	held := crmcontracts.CreateActivityRequestMeetingStatus("held")
	transport := "whatsapp"
	cases := map[string]struct {
		kind     string
		duration *int
		status   *crmcontracts.CreateActivityRequestMeetingStatus
		provider *string
	}{
		"no kind":                     {kind: ""},
		"a kind this timeline lacks":  {kind: "postcard"},
		"a duration on a note":        {kind: "note", duration: seconds(600)},
		"a status on a call":          {kind: "call", status: &held},
		"a message without a channel": {kind: "message"},
		"a transport on a note":       {kind: "note", provider: &transport},
		"a clean meeting":             {kind: "meeting", duration: seconds(1800), status: &held},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, wire := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
				Kind: crmcontracts.CreateActivityRequestKind(c.kind), Source: "test",
				DurationSeconds: c.duration, MeetingStatus: c.status, ChannelProvider: c.provider,
			})
			status := meetingStatusOf(c.status)
			imported := CheckKindFields(c.kind, c.duration, status, providerOf(c.provider))
			if fmt.Sprintf("%T", wire) != fmt.Sprintf("%T", imported) {
				t.Fatalf("wire refused with %T (%v), import with %T (%v)", wire, wire, imported, imported)
			}
		})
	}
}

// The wire keeps the order its errors have always come in: a blank source is
// reported before a kind field, and a status on a call before a transport.
func TestTheWireReportsASourceBeforeAKindField(t *testing.T) {
	seconds := 600
	_, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
		Kind: crmcontracts.CreateActivityRequestKind("note"), Source: " ", DurationSeconds: &seconds,
	})
	if _, isKindField := errors.AsType[*KindFieldError](err); err == nil || isKindField {
		t.Fatalf("a blank source was answered with %v, want the source refusal", err)
	}
	held := crmcontracts.CreateActivityRequestMeetingStatus("held")
	transport := "whatsapp"
	_, err = LogActivityInputFrom(crmcontracts.CreateActivityRequest{
		Kind: crmcontracts.CreateActivityRequestKind("note"), Source: "test",
		MeetingStatus: &held, ChannelProvider: &transport,
	})
	if _, ok := errors.AsType[*MeetingStatusKindError](err); !ok {
		t.Fatalf("a status on a note was answered with %v", err)
	}
}
