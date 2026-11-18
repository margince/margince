// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package accountdraft

import (
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// An account with only an internal note is still a first touch.
// One with a past meeting is not, and silence_days counts from the meeting.
func TestANoteOnlyAccountIsAFirstTouch(t *testing.T) {
	now := time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)
	body := "Met their COO at the trade fair."
	view := func(kind crmcontracts.ActivityKind) crmcontracts.Company360 {
		return crmcontracts.Company360{Activities: &crmcontracts.ActivityListResponse{Data: []crmcontracts.Activity{{
			Id: openapi_types.UUID(ids.NewV7()), Kind: kind,
			Body: &body, OccurredAt: now.Add(-10 * 24 * time.Hour),
		}}}}
	}

	if state := ConversationState(view(crmcontracts.ActivityKindNote), now, time.UTC); state.Band != convstate.BandNone {
		t.Errorf("band = %v after a note and no exchange, want %v", state.Band, convstate.BandNone)
	}
	state := ConversationState(view(crmcontracts.ActivityKindMeeting), now, time.UTC)
	if state.Band == convstate.BandNone || state.SilenceDays != 10 {
		t.Errorf("state = %+v, want ten days since the meeting", state)
	}
}
