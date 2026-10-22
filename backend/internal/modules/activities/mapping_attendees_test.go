// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// attendee_user_ids pairs with a meeting or a call, the two kinds with
// attendees. A note naming colleagues would credit them with a conversation
// nobody had.

import (
	"errors"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestLogActivityInputCarriesTheColleaguesWhoAttendedACall(t *testing.T) {
	colleague := openapi_types.UUID(ids.NewV7())
	attendees := []openapi_types.UUID{colleague}
	subject := "Renewal call"

	in, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
		Kind:            crmcontracts.CreateActivityRequestKindCreateActivityRequestKindCall,
		AttendeeUserIds: &attendees,
		Subject:         &subject,
		Source:          "human",
	})
	if err != nil {
		t.Fatalf("mapping a call with a colleague: %v", err)
	}
	if len(in.AttendeeUserIDs) != 1 || in.AttendeeUserIDs[0].UUID != ids.UUID(colleague) {
		t.Fatalf("AttendeeUserIDs = %v, want [%v]", in.AttendeeUserIDs, colleague)
	}
}

func TestLogActivityInputRefusesColleaguesOnANote(t *testing.T) {
	attendees := []openapi_types.UUID{openapi_types.UUID(ids.NewV7())}

	_, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
		Kind:            crmcontracts.CreateActivityRequestKindCreateActivityRequestKindNote,
		AttendeeUserIds: &attendees,
		Source:          "human",
	})

	var fault *KindFieldError
	if !errors.As(err, &fault) {
		t.Fatalf("err = %v, want a KindFieldError naming attendee_user_ids", err)
	}
	if field, _, _ := fault.FieldFault(); field != fieldAttendeeUserIDs {
		t.Fatalf("FieldFault names %q, want %s", field, fieldAttendeeUserIDs)
	}
}

// Presence decides, not length: an empty list on a note is still the wrong
// field for that kind.
func TestLogActivityInputRefusesAnEmptyColleagueListOnANote(t *testing.T) {
	attendees := []openapi_types.UUID{}

	_, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
		Kind:            crmcontracts.CreateActivityRequestKindCreateActivityRequestKindNote,
		AttendeeUserIds: &attendees,
		Source:          "human",
	})

	if _, ok := errors.AsType[*KindFieldError](err); !ok {
		t.Fatalf("err = %v, want a KindFieldError naming attendee_user_ids", err)
	}
}
